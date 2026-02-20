package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"
	pkgsafemap "example.com/webrtcserver/pkg/safemap"
	"github.com/alecthomas/kong"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
)

var cli struct {
	WsServer          string `name:"ws-server" help:"WebSocket server URL" default:"ws://localhost:3001/ws"`
	NodeName          string `name:"node-name" help:"Node name for registration" default:"webrtc-agent-1"`
	PingPeriodSeconds int    `name:"ping-period-seconds" help:"Ping period in seconds" default:"5"`
	Debug             bool   `name:"debug" help:"Show ping/pong messages in logs for debugging purposes"`
}

// PeerConnEntry tracks a peer connection and its associated data
type PeerConnEntry struct {
	PeerConnection   *webrtc.PeerConnection
	DataChannel      *webrtc.DataChannel
	FileDataChannels map[string]*webrtc.DataChannel
	RemoteOffers     []webrtc.SessionDescription
	QueuedICEOffers  []webrtc.ICECandidateInit
	mu               sync.RWMutex
}

// PeerConnStore is a thread-safe store for peer connections
type PeerConnStore struct {
	store *pkgsafemap.SafeMap
}

// NewPeerConnStore creates a new peer connection store
func NewPeerConnStore() *PeerConnStore {
	return &PeerConnStore{
		store: pkgsafemap.NewSafeMap(),
	}
}

// Get retrieves a peer connection entry
func (s *PeerConnStore) Get(remoteNodeID string) (*PeerConnEntry, bool) {
	var entry *PeerConnEntry
	_, found := s.store.Get(remoteNodeID, func(value interface{}) error {
		entry = value.(*PeerConnEntry)
		return nil
	})
	return entry, found
}

// Set stores a peer connection entry
func (s *PeerConnStore) Set(remoteNodeID string, entry *PeerConnEntry) {
	s.store.Set(remoteNodeID, entry)
}

// Delete removes a peer connection entry
func (s *PeerConnStore) Delete(remoteNodeID string) {
	s.store.Delete(remoteNodeID)
}

// Walk iterates over all peer connections
func (s *PeerConnStore) Walk(walkFunc func(remoteNodeID string, entry *PeerConnEntry) (bool, error)) error {
	return s.store.Walk(func(key string, value interface{}) (bool, error) {
		entry := value.(*PeerConnEntry)
		return walkFunc(key, entry)
	})
}

const (
	googleStunServer = "stun:stun.l.google.com:19302"
)

// Predefined data channel labels
const (
	PredefinedDCLabelChat = "chat"
	PredefinedDCLabelFile = "file"
	PredefinedDCLabelPing = "ping"
)

func main() {
	kong.Parse(&cli)

	pingPeriod := time.Duration(cli.PingPeriodSeconds) * time.Second

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u, err := url.Parse(cli.WsServer)
	if err != nil {
		log.Fatal("Failed to parse WebSocket URL:", err)
	}

	log.Printf("Connecting to %s", u.String())

	wsConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("Failed to dial:", err)
	}
	defer wsConn.Close()

	// Track our node ID after registration
	nodeID := ""
	nodeIDMu := sync.RWMutex{}

	// Peer connection store
	peerConnStore := NewPeerConnStore()

	// Create WebRTC API with default settings
	webrtcAPI := webrtc.NewAPI()

	// Channel to signal when connection is closed
	done := make(chan struct{})

	// Helper function to create new peer connection
	createPeerConnection := func(remoteNodeID string) (*PeerConnEntry, error) {
		config := webrtc.Configuration{
			ICEServers: []webrtc.ICEServer{
				{
					URLs: []string{googleStunServer},
				},
			},
		}

		pc, err := webrtcAPI.NewPeerConnection(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create peer connection: %w", err)
		}

		entry := &PeerConnEntry{
			PeerConnection:   pc,
			FileDataChannels: make(map[string]*webrtc.DataChannel),
			RemoteOffers:     make([]webrtc.SessionDescription, 0),
			QueuedICEOffers:  make([]webrtc.ICECandidateInit, 0),
		}

		// Set up data channel handler for incoming data channels
		pc.OnDataChannel(func(dc *webrtc.DataChannel) {
			log.Printf("[webrtc] Received data channel: %s from peer %s", dc.Label(), remoteNodeID)

			switch dc.Label() {
			case PredefinedDCLabelChat:
				entry.mu.Lock()
				entry.DataChannel = dc
				entry.mu.Unlock()
				setupChatDataChannel(dc, remoteNodeID)
			case PredefinedDCLabelFile:
				dcID := dc.ID()
				if dcID != nil {
					entry.mu.Lock()
					entry.FileDataChannels[fmt.Sprintf("%d", *dcID)] = dc
					entry.mu.Unlock()
				}
				setupFileDataChannel(dc, remoteNodeID)
			case PredefinedDCLabelPing:
				setupPingDataChannel(dc, remoteNodeID)
			default:
				log.Printf("[webrtc] Unknown data channel label: %s", dc.Label())
			}
		})

		// Set up ICE candidate handler
		pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
			if candidate == nil {
				return
			}

			nodeIDMu.RLock()
			myNodeID := nodeID
			nodeIDMu.RUnlock()

			if myNodeID == "" {
				return
			}

			candidateJSON, err := json.Marshal(candidate.ToJSON())
			if err != nil {
				log.Printf("Failed to marshal ICE candidate: %v", err)
				return
			}

			iceOfferMsg := pkgframing.MessagePayload{
				ICEOffer: &pkgconnreg.ICEOfferPayload{
					OfferJSON:  string(candidateJSON),
					FromNodeId: myNodeID,
					ToNodeId:   remoteNodeID,
				},
			}

			data, err := json.Marshal(iceOfferMsg)
			if err != nil {
				log.Printf("Failed to marshal ICE offer message: %v", err)
				return
			}

			if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("Failed to send ICE offer: %v", err)
			}

			if cli.Debug {
				log.Printf("[webrtc] Sent ICE candidate to peer %s", remoteNodeID)
			}
		})

		// Set up connection state handler
		pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
			log.Printf("[webrtc] Peer connection state changed: %s for peer %s", state, remoteNodeID)

			switch state {
			case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
				peerConnStore.Delete(remoteNodeID)
				if err := pc.Close(); err != nil {
					log.Printf("Failed to close peer connection: %v", err)
				}
			}
		})

		return entry, nil
	}

	// Start goroutine to read messages from server
	go func() {
		defer close(done)
		for {
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				log.Println("Read error:", err)
				return
			}
			if cli.Debug {
				log.Printf("Received: %s", message)
			}

			var payload pkgframing.MessagePayload
			if err := json.Unmarshal(message, &payload); err != nil {
				log.Printf("Failed to parse message: %v", err)
				continue
			}

			// Handle pong messages
			if payload.Echo != nil && payload.Echo.Direction == pkgconnreg.EchoDirectionS2C {
				if cli.Debug {
					rtt := time.Since(time.UnixMilli(int64(payload.Echo.Timestamp)))
					log.Printf("Pong received - RTT: %v, CorrelationID: %s, SeqID: %d",
						rtt, payload.Echo.CorrelationID, payload.Echo.SeqID)
				}
				continue
			}

			// Handle node ID from registration response
			if payload.NodeId != "" {
				nodeIDMu.Lock()
				nodeID = payload.NodeId
				nodeIDMu.Unlock()
				log.Printf("Registered with node ID: %s", payload.NodeId)
				continue
			}

			// Handle SDP offer
			if sdpOffer := payload.SDPOffer; sdpOffer != nil {
				nodeIDMu.RLock()
				myNodeID := nodeID
				nodeIDMu.RUnlock()

				if sdpOffer.ToNodeId != myNodeID {
					continue
				}

				remoteNodeID := sdpOffer.FromNodeId
				log.Printf("[webrtc] Received SDP offer from peer %s, type: %s", remoteNodeID, sdpOffer.Type)

				// Get or create peer connection entry
				entry, found := peerConnStore.Get(remoteNodeID)
				if !found {
					var createErr error
					entry, createErr = createPeerConnection(remoteNodeID)
					if createErr != nil {
						log.Printf("Failed to create peer connection: %v", createErr)
						continue
					}
					peerConnStore.Set(remoteNodeID, entry)
				}

				// Parse the SDP offer
				var offer webrtc.SessionDescription
				if err := json.Unmarshal([]byte(sdpOffer.OfferJSON), &offer); err != nil {
					log.Printf("Failed to parse SDP offer: %v", err)
					continue
				}

				entry.mu.Lock()
				entry.RemoteOffers = append(entry.RemoteOffers, offer)
				entry.mu.Unlock()

				// Set remote description
				if err := entry.PeerConnection.SetRemoteDescription(offer); err != nil {
					log.Printf("Failed to set remote description: %v", err)
					continue
				}

				// If this is an offer, create an answer
				if sdpOffer.Type == pkgconnreg.OfferTypeOffer {
					log.Printf("[webrtc] Creating answer for peer %s", remoteNodeID)

					answer, err := entry.PeerConnection.CreateAnswer(nil)
					if err != nil {
						log.Printf("Failed to create answer: %v", err)
						continue
					}

					if err := entry.PeerConnection.SetLocalDescription(answer); err != nil {
						log.Printf("Failed to set local description: %v", err)
						continue
					}

					answerJSON, err := json.Marshal(answer)
					if err != nil {
						log.Printf("Failed to marshal answer: %v", err)
						continue
					}

					answerMsg := pkgframing.MessagePayload{
						SDPOffer: &pkgconnreg.SDPOfferPayload{
							Type:       pkgconnreg.OfferTypeAnswer,
							OfferJSON:  string(answerJSON),
							FromNodeId: myNodeID,
							ToNodeId:   remoteNodeID,
						},
					}

					data, err := json.Marshal(answerMsg)
					if err != nil {
						log.Printf("Failed to marshal answer message: %v", err)
						continue
					}

					if err := wsConn.WriteMessage(websocket.TextMessage, data); err != nil {
						log.Printf("Failed to send SDP answer: %v", err)
						continue
					}

					log.Printf("[webrtc] Sent SDP answer to peer %s", remoteNodeID)
				}

				// Process any queued ICE candidates
				entry.mu.RLock()
				queuedICE := make([]webrtc.ICECandidateInit, len(entry.QueuedICEOffers))
				copy(queuedICE, entry.QueuedICEOffers)
				entry.mu.RUnlock()

				for _, iceCandidate := range queuedICE {
					if err := entry.PeerConnection.AddICECandidate(iceCandidate); err != nil {
						log.Printf("Failed to add queued ICE candidate: %v", err)
					}
				}

				entry.mu.Lock()
				entry.QueuedICEOffers = nil
				entry.mu.Unlock()

				continue
			}

			// Handle ICE offer
			if iceOffer := payload.ICEOffer; iceOffer != nil {
				nodeIDMu.RLock()
				myNodeID := nodeID
				nodeIDMu.RUnlock()

				if iceOffer.ToNodeId != myNodeID {
					continue
				}

				remoteNodeID := iceOffer.FromNodeId
				if cli.Debug {
					log.Printf("[webrtc] Received ICE candidate from peer %s", remoteNodeID)
				}

				entry, found := peerConnStore.Get(remoteNodeID)
				if !found {
					log.Printf("[webrtc] No peer connection found for ICE candidate from %s", remoteNodeID)
					continue
				}

				// Parse ICE candidate
				var iceCandidate webrtc.ICECandidateInit
				if err := json.Unmarshal([]byte(iceOffer.OfferJSON), &iceCandidate); err != nil {
					log.Printf("Failed to parse ICE candidate: %v", err)
					continue
				}

				// Check if remote description is set
				entry.mu.RLock()
				hasRemoteOffer := len(entry.RemoteOffers) > 0
				entry.mu.RUnlock()

				if !hasRemoteOffer {
					// Queue the ICE candidate
					entry.mu.Lock()
					entry.QueuedICEOffers = append(entry.QueuedICEOffers, iceCandidate)
					entry.mu.Unlock()
					if cli.Debug {
						log.Printf("[webrtc] Queued ICE candidate from peer %s", remoteNodeID)
					}
					continue
				}

				// Add ICE candidate directly
				if err := entry.PeerConnection.AddICECandidate(iceCandidate); err != nil {
					log.Printf("Failed to add ICE candidate: %v", err)
				}
			}
		}
	}()

	// Send registration message
	registerMsg := pkgframing.MessagePayload{
		Register: &pkgconnreg.RegisterPayload{
			NodeName: cli.NodeName,
		},
	}
	registerData, err := json.Marshal(registerMsg)
	if err != nil {
		log.Fatal("Failed to marshal registration message:", err)
	}

	err = wsConn.WriteMessage(websocket.TextMessage, registerData)
	if err != nil {
		log.Fatal("Failed to send registration:", err)
	}
	log.Printf("Sent registration message for node: %s", cli.NodeName)

	// Ticker for sending ping messages
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	seqID := uint64(0)

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			seqID++
			now := uint64(time.Now().UnixMilli())
			pingMsg := pkgframing.MessagePayload{
				Echo: &pkgconnreg.EchoPayload{
					Direction:     pkgconnreg.EchoDirectionC2S,
					CorrelationID: fmt.Sprintf("ping-%d", seqID),
					Timestamp:     now,
					SeqID:         seqID,
				},
			}
			pingData, err := json.Marshal(pingMsg)
			if err != nil {
				log.Println("Failed to marshal ping message:", err)
				continue
			}

			if err := wsConn.WriteMessage(websocket.TextMessage, pingData); err != nil {
				log.Println("Failed to send ping:", err)
				return
			}
			if cli.Debug {
				log.Printf("Sent ping - SeqID: %d, CorrelationID: ping-%d", seqID, seqID)
			}

		case <-interrupt:
			log.Println("Interrupt received, closing connection...")

			// Close all peer connections
			peerConnStore.Walk(func(remoteNodeID string, entry *PeerConnEntry) (bool, error) {
				if err := entry.PeerConnection.Close(); err != nil {
					log.Printf("Failed to close peer connection to %s: %v", remoteNodeID, err)
				}
				return true, nil
			})

			// Cleanly close the WebSocket connection
			err := wsConn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("Failed to write close message:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

// setupChatDataChannel sets up event handlers for chat data channel
func setupChatDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] Chat data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] Chat data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		log.Printf("[webrtc] Received chat message from peer %s: %s", remoteNodeID, string(msg.Data))
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] Chat data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// setupFileDataChannel sets up event handlers for file data channel
func setupFileDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] File data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] File data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if cli.Debug {
			log.Printf("[webrtc] Received file data from peer %s, size: %d bytes", remoteNodeID, len(msg.Data))
		}
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] File data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// setupPingDataChannel sets up event handlers for ping data channel
func setupPingDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] Ping data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] Ping data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if cli.Debug {
			log.Printf("[webrtc] Received ping message from peer %s", remoteNodeID)
		}
		// Echo back the ping
		if err := dc.Send(msg.Data); err != nil {
			log.Printf("[webrtc] Failed to send ping response: %v", err)
		}
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] Ping data channel error with peer %s: %v", remoteNodeID, err)
	})
}
