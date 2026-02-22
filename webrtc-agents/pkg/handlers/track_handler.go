package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	pkgtracks "webrtc-agents/pkg/tracks"
	pkgwn "webrtc-agents/pkg/tracks/wn"
	pkgwsrunner "webrtc-agents/pkg/ws_runner"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"

	"github.com/google/uuid"
	"github.com/hraban/opus"
	"github.com/pion/webrtc/v4"
)

// TrackHandler handles WebRTC peer connections
type TrackHandler struct {
	peerConnStore *PeerConnStore
	webrtcAPI     *webrtc.API
	iceServers    []webrtc.ICEServer
	debug         bool
	nodeID        string
	signallingRx  <-chan pkgframing.MessagePayload
}

func (h *TrackHandler) GetNodeID() string {
	// todo: protect it with lock/proper synchronization
	return h.nodeID
}

func (h *TrackHandler) SetNodeID(nodeID string) {
	// todo: protect it with lock/proper synchronization
	h.nodeID = nodeID
}

// NewTrackHandler creates a new WebRTC handler
func NewTrackHandler(iceServers []string, debug bool) *TrackHandler {
	// Convert string ICE servers to webrtc.ICEServer
	var servers []webrtc.ICEServer
	for _, server := range iceServers {
		servers = append(servers, webrtc.ICEServer{
			URLs: []string{server},
		})
	}

	return &TrackHandler{
		peerConnStore: NewPeerConnStore(),
		webrtcAPI:     webrtc.NewAPI(),
		iceServers:    servers,
		debug:         debug,
	}
}

// Serve starts the WebRTC handler
func (h *TrackHandler) Serve(ctx context.Context, signallingTx chan<- pkgframing.MessagePayload, signallingRx <-chan pkgframing.MessagePayload) {
	h.signallingRx = signallingRx

	for {
		select {
		case <-ctx.Done():
			h.cleanup()
			return
		case msg, ok := <-h.signallingRx:
			if !ok {
				return
			}
			h.handleMessage(msg, signallingTx)
		}
	}
}

func (h *TrackHandler) Run(ctx context.Context, runner pkgwsrunner.WebSocketSignallingSessionRunner) {
	tx, rx := runner.Run(ctx)
	h.Serve(ctx, tx, rx)
}

// handleMessage processes incoming signalling messages
func (h *TrackHandler) handleMessage(msg pkgframing.MessagePayload, signallingTx chan<- pkgframing.MessagePayload) {
	// Handle node ID from registration response
	if msg.NodeId != "" {
		h.SetNodeID(msg.NodeId)
		log.Printf("Registered with node ID: %s", msg.NodeId)
		return
	}

	// Handle SDP offer
	if sdpOffer := msg.SDPOffer; sdpOffer != nil {
		h.handleSDPOffer(sdpOffer, signallingTx)
		return
	}

	// Handle ICE offer
	if iceOffer := msg.ICEOffer; iceOffer != nil {
		h.handleICEOffer(iceOffer)
		return
	}
}

// handleSDPOffer handles SDP offer/answer messages
func (h *TrackHandler) handleSDPOffer(sdpOffer *pkgconnreg.SDPOfferPayload, signallingTx chan<- pkgframing.MessagePayload) {

	remoteNodeID := sdpOffer.FromNodeId
	log.Printf("[webrtc] Received SDP offer from peer %s, type: %s", remoteNodeID, sdpOffer.Type)

	// Get or create peer connection entry
	entry, found := h.peerConnStore.Get(remoteNodeID)
	if !found {
		var createErr error
		entry, createErr = h.createPeerConnection(remoteNodeID, signallingTx)
		if createErr != nil {
			log.Printf("Failed to create peer connection: %v", createErr)
			return
		}
		h.peerConnStore.Set(remoteNodeID, entry)
	}

	// Parse the SDP offer
	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(sdpOffer.OfferJSON), &offer); err != nil {
		log.Printf("Failed to parse SDP offer: %v", err)
		return
	}

	entry.mu.Lock()
	entry.RemoteOffers = append(entry.RemoteOffers, offer)
	entry.mu.Unlock()

	// Set remote description
	if err := entry.PeerConnection.SetRemoteDescription(offer); err != nil {
		log.Printf("Failed to set remote description: %v", err)
		return
	}

	// If this is an offer, create an answer
	if sdpOffer.Type == pkgconnreg.OfferTypeOffer {
		// Check if this is an ICE restart offer
		isICERestart := containsICERestartOption(offer.SDP)

		if isICERestart {
			log.Printf("[webrtc] Received ICE restart offer from peer %s", remoteNodeID)
			// Clear queued ICE candidates from previous session
			entry.mu.Lock()
			entry.QueuedICEOffers = nil
			entry.mu.Unlock()
		}

		log.Printf("[webrtc] Creating answer for peer %s", remoteNodeID)

		answer, err := entry.PeerConnection.CreateAnswer(nil)
		if err != nil {
			log.Printf("Failed to create answer: %v", err)
			return
		}

		if err := entry.PeerConnection.SetLocalDescription(answer); err != nil {
			log.Printf("Failed to set local description: %v", err)
			return
		}

		answerJSON, err := json.Marshal(answer)
		if err != nil {
			log.Printf("Failed to marshal answer: %v", err)
			return
		}

		answerMsg := pkgframing.MessagePayload{
			SDPOffer: &pkgconnreg.SDPOfferPayload{
				Type:       pkgconnreg.OfferTypeAnswer,
				OfferJSON:  string(answerJSON),
				FromNodeId: h.GetNodeID(),
				ToNodeId:   remoteNodeID,
			},
		}

		signallingTx <- answerMsg

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
}

// handleICEOffer handles ICE candidate messages
func (h *TrackHandler) handleICEOffer(iceOffer *pkgconnreg.ICEOfferPayload) {

	remoteNodeID := iceOffer.FromNodeId
	if h.debug {
		log.Printf("[webrtc] Received ICE candidate from peer %s", remoteNodeID)
	}

	entry, found := h.peerConnStore.Get(remoteNodeID)
	if !found {
		log.Printf("[webrtc] No peer connection found for ICE candidate from %s", remoteNodeID)
		return
	}

	// Parse ICE candidate
	var iceCandidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(iceOffer.OfferJSON), &iceCandidate); err != nil {
		log.Printf("Failed to parse ICE candidate: %v", err)
		return
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
		if h.debug {
			log.Printf("[webrtc] Queued ICE candidate from peer %s", remoteNodeID)
		}
		return
	}

	// Add ICE candidate directly
	if err := entry.PeerConnection.AddICECandidate(iceCandidate); err != nil {
		log.Printf("Failed to add ICE candidate: %v", err)
	}
}

// createPeerConnection creates a new peer connection for the given remote node
func (h *TrackHandler) createPeerConnection(remoteNodeID string, signallingTx chan<- pkgframing.MessagePayload) (*PeerConnEntry, error) {
	config := webrtc.Configuration{
		ICEServers: h.iceServers,
	}

	pc, err := h.webrtcAPI.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	entry := &PeerConnEntry{
		PeerConnection:        pc,
		FileDataChannels:      make(map[string]*webrtc.DataChannel),
		RemoteOffers:          make([]webrtc.SessionDescription, 0),
		QueuedICEOffers:       make([]webrtc.ICECandidateInit, 0),
		ICERestartAttempts:    0,
		MaxICERestartAttempts: 3,
	}

	// Set up data channel handler for incoming data channels
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		log.Printf("[webrtc] Received data channel: %s from peer %s", dc.Label(), remoteNodeID)

		switch dc.Label() {
		case PredefinedDCLabelChat:
			entry.mu.Lock()
			entry.DataChannel = dc
			entry.mu.Unlock()
			h.setupChatDataChannel(dc, remoteNodeID, signallingTx)
		case PredefinedDCLabelPing:
			h.setupPingDataChannel(dc, remoteNodeID)
		default:
			log.Printf("[webrtc] Unknown (or unsupported) data channel label: %s", dc.Label())
		}
	})

	// Set up ICE candidate handler
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
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
				FromNodeId: h.GetNodeID(),
				ToNodeId:   remoteNodeID,
			},
		}

		signallingTx <- iceOfferMsg

		if h.debug {
			log.Printf("[webrtc] Sent ICE candidate to peer %s", remoteNodeID)
		}
	})

	// Set up connection state handler
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("[webrtc] Peer connection state changed: %s for peer %s", state, remoteNodeID)

		switch state {
		case webrtc.PeerConnectionStateDisconnected:
			// Wait briefly then attempt ICE restart
			go func() {
				time.Sleep(3 * time.Second)
				entry.mu.RLock()
				currentState := entry.PeerConnection.ConnectionState()
				attempts := entry.ICERestartAttempts
				maxAttempts := entry.MaxICERestartAttempts
				entry.mu.RUnlock()

				if currentState == webrtc.PeerConnectionStateDisconnected && attempts < maxAttempts {
					log.Printf("[webrtc] Attempting ICE restart for peer %s (attempt %d/%d)", remoteNodeID, attempts+1, maxAttempts)
					if err := h.initiateICERestart(entry, remoteNodeID, signallingTx); err != nil {
						log.Printf("[webrtc] ICE restart failed for peer %s: %v", remoteNodeID, err)
					}
				} else if attempts >= maxAttempts {
					log.Printf("[webrtc] Max ICE restart attempts reached for peer %s, closing connection", remoteNodeID)
					h.peerConnStore.Delete(remoteNodeID)
					if err := entry.PeerConnection.Close(); err != nil {
						log.Printf("Failed to close peer connection: %v", err)
					}
				}
			}()

		case webrtc.PeerConnectionStateFailed:
			// Attempt immediate ICE restart
			entry.mu.RLock()
			attempts := entry.ICERestartAttempts
			maxAttempts := entry.MaxICERestartAttempts
			entry.mu.RUnlock()

			if attempts < maxAttempts {
				log.Printf("[webrtc] Connection failed, attempting ICE restart for peer %s (attempt %d/%d)", remoteNodeID, attempts+1, maxAttempts)
				if err := h.initiateICERestart(entry, remoteNodeID, signallingTx); err != nil {
					log.Printf("[webrtc] ICE restart failed for peer %s: %v", remoteNodeID, err)
					h.peerConnStore.Delete(remoteNodeID)
					if err := entry.PeerConnection.Close(); err != nil {
						log.Printf("Failed to close peer connection: %v", err)
					}
				}
			} else {
				log.Printf("[webrtc] Max ICE restart attempts reached for peer %s, closing connection", remoteNodeID)
				h.peerConnStore.Delete(remoteNodeID)
				if err := entry.PeerConnection.Close(); err != nil {
					log.Printf("Failed to close peer connection: %v", err)
				}
			}

		case webrtc.PeerConnectionStateConnected:
			// Reset restart attempts on successful connection
			entry.mu.Lock()
			entry.ICERestartAttempts = 0
			entry.mu.Unlock()

		case webrtc.PeerConnectionStateClosed:
			h.peerConnStore.Delete(remoteNodeID)
		}
	})

	return entry, nil
}

// initiateICERestart initiates an ICE restart for a peer connection
func (h *TrackHandler) initiateICERestart(entry *PeerConnEntry, remoteNodeID string, tx chan<- pkgframing.MessagePayload) error {
	entry.mu.Lock()
	entry.ICERestartAttempts++
	attempts := entry.ICERestartAttempts
	entry.mu.Unlock()

	log.Printf("[webrtc] Initiating ICE restart for peer %s (attempt %d)", remoteNodeID, attempts)

	// Create offer with ICE restart
	offer, err := entry.PeerConnection.CreateOffer(&webrtc.OfferOptions{
		ICERestart: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create ICE restart offer: %w", err)
	}

	// Set local description
	if err := entry.PeerConnection.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Send the offer via signalling server
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return fmt.Errorf("failed to marshal offer: %w", err)
	}

	offerMsg := pkgframing.MessagePayload{
		SDPOffer: &pkgconnreg.SDPOfferPayload{
			Type:       pkgconnreg.OfferTypeOffer,
			OfferJSON:  string(offerJSON),
			FromNodeId: h.GetNodeID(),
			ToNodeId:   remoteNodeID,
		},
	}

	tx <- offerMsg

	log.Printf("[webrtc] Sent ICE restart offer to peer %s", remoteNodeID)
	return nil
}

// cleanup closes all peer connections
func (h *TrackHandler) cleanup() {
	h.peerConnStore.Walk(func(remoteNodeID string, entry *PeerConnEntry) (bool, error) {
		if err := entry.PeerConnection.Close(); err != nil {
			log.Printf("Failed to close peer connection to %s: %v", remoteNodeID, err)
		}
		return true, nil
	})
}

// setupChatDataChannel sets up event handlers for chat data channel
func (h *TrackHandler) setupChatDataChannel(dc *webrtc.DataChannel, remoteNodeID string, signallingTx chan<- pkgframing.MessagePayload) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] Chat data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] Chat data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		// Parse the message as ChatMessage
		var chatMsg ChatMessage
		if err := json.Unmarshal(msg.Data, &chatMsg); err != nil {
			log.Printf("[webrtc] Failed to parse chat message: %v", err)
			return
		}

		// Check for "/start" command to create a track
		if chatMsg.Message != nil && *chatMsg.Message == "/start" {
			log.Printf("[webrtc] Received /start command from peer %s", remoteNodeID)
			entry, found := h.peerConnStore.Get(remoteNodeID)
			if !found {
				log.Printf("[webrtc] No peer connection found for peer %s", remoteNodeID)
				return
			}
			if err := h.createAndAddTrack(entry, remoteNodeID, signallingTx); err != nil {
				log.Printf("[webrtc] Failed to create track for peer %s: %v", remoteNodeID, err)
			}
			return
		}

		// Skip control messages that shouldn't be echoed
		// - ack: acknowledgment messages
		// - delete: delete request messages
		// - amend: amend request messages
		// - ping: ping/pong messages
		// - file: file transfer metadata
		if chatMsg.ACK != nil || chatMsg.Delete != nil || chatMsg.Amend != nil || chatMsg.Ping != nil || chatMsg.File != nil {
			log.Printf("[webrtc] Ignored un-echoable message from peer %s: %+v", remoteNodeID, string(msg.Data))
			return
		}

		// Only echo messages that have actual text content
		if chatMsg.Message == nil && chatMsg.RichText == nil {
			log.Printf("[webrtc] Skipping non-text message from peer %s", remoteNodeID)
			return
		}

		// Send ACK for the received message first
		ackMsg := ChatMessage{
			MessageID:  uuid.New().String(),
			FromNodeID: chatMsg.ToNodeID,
			ToNodeID:   chatMsg.FromNodeID,
			Timestamp:  time.Now().UnixMilli(),
			ACK: &ChatMessageACK{
				MessageID: chatMsg.MessageID,
			},
		}
		ackData, err := json.Marshal(ackMsg)
		if err != nil {
			log.Printf("[webrtc] Failed to marshal ACK message: %v", err)
		} else if err := dc.SendText(string(ackData)); err != nil {
			log.Printf("[webrtc] Failed to send ACK: %v", err)
		}

		// Swap sender and receiver for echo
		chatMsg.FromNodeID, chatMsg.ToNodeID = chatMsg.ToNodeID, chatMsg.FromNodeID
		chatMsg.ACK = nil
		chatMsg.ACKed = nil
		chatMsg.MessageID = uuid.New().String()

		// Marshal the modified message
		responseData, err := json.Marshal(chatMsg)
		if err != nil {
			log.Printf("[webrtc] Failed to marshal chat message: %v", err)
			return
		}

		// Send the modified message back
		if err := dc.SendText(string(responseData)); err != nil {
			log.Printf("[webrtc] Failed to send chat response: %v", err)
		}
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] Chat data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// setupPingDataChannel sets up event handlers for ping data channel
func (h *TrackHandler) setupPingDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] Ping data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] Ping data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if h.debug {
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

// createAndAddTrack creates a new audio track and adds it to the peer connection
func (h *TrackHandler) createAndAddTrack(entry *PeerConnEntry, remoteNodeID string, signallingTx chan<- pkgframing.MessagePayload) error {

	frameIntv := pkgtracks.DefaultFrameIntv
	sampleRate := pkgtracks.DefaultSampleRate
	channels := pkgtracks.DefaultChannelsCount
	samplesPerPacket := int(float64(frameIntv) / float64(1000) * float64(sampleRate))

	enc, err := opus.NewEncoder(
		sampleRate,
		channels,
		opus.AppAudio,
	)
	if err != nil {
		return fmt.Errorf("failed to create encoder: %w", err)
	}

	packetGen, err := pkgwn.NewOpusWhiteNoiseGenerator(
		enc, channels, samplesPerPacket, pkgtracks.DefaultMaxPayloadSize,
	)
	if err != nil {
		return fmt.Errorf("failed to create white noise RTP packet generator: %w", err)
	}

	// Create a new audio track
	track, err := pkgtracks.NewTrackHandle(fmt.Sprintf("stream-%s", remoteNodeID), frameIntv, packetGen)
	if err != nil {
		return fmt.Errorf("failed to create track: %w", err)
	}

	// Add the track to the peer connection
	sender, err := entry.PeerConnection.AddTrack(track)
	if err != nil {
		return fmt.Errorf("failed to add track to peer connection: %w", err)
	}

	log.Printf("[webrtc] Created and added track %s for peer %s", track.ID(), remoteNodeID)

	// Create a new offer since we added a track
	offer, err := entry.PeerConnection.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("failed to create offer: %w", err)
	}

	// Set local description
	if err := entry.PeerConnection.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Marshal the offer to JSON
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return fmt.Errorf("failed to marshal offer: %w", err)
	}

	// Send the offer via signalling server
	offerMsg := pkgframing.MessagePayload{
		SDPOffer: &pkgconnreg.SDPOfferPayload{
			Type:       pkgconnreg.OfferTypeOffer,
			OfferJSON:  string(offerJSON),
			FromNodeId: h.GetNodeID(),
			ToNodeId:   remoteNodeID,
		},
	}

	signallingTx <- offerMsg

	log.Printf("[webrtc] Sent offer with new track to peer %s", remoteNodeID)

	// Remove the sender if we need to stop the track later
	_ = sender

	return nil
}
