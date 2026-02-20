package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"
	pkgsafemap "example.com/webrtcserver/pkg/safemap"

	"github.com/pion/webrtc/v4"
)

// SignallingServerProxy defines the interface for signalling communication
type SignallingServerProxy interface {
	// Send sends a message to the signalling server
	Send(ctx context.Context, msg pkgframing.MessagePayload) error
	// Receive returns a channel for receiving messages from the signalling server
	Receive() <-chan pkgframing.MessagePayload
	// GetNodeID returns the current node ID
	GetNodeID() string
	// SetNodeID sets the node ID after registration
	SetNodeID(nodeID string)
}

// PeerConnEntry tracks a peer connection and its associated data
type PeerConnEntry struct {
	PeerConnection        *webrtc.PeerConnection
	DataChannel           *webrtc.DataChannel
	FileDataChannels      map[string]*webrtc.DataChannel
	RemoteOffers          []webrtc.SessionDescription
	QueuedICEOffers       []webrtc.ICECandidateInit
	ICERestartAttempts    int
	MaxICERestartAttempts int
	mu                    sync.RWMutex
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

// Predefined data channel labels
const (
	PredefinedDCLabelChat = "chat"
	PredefinedDCLabelFile = "file"
	PredefinedDCLabelPing = "ping"
)

// WebRTCHandler handles WebRTC peer connections
type WebRTCHandler struct {
	signalling    SignallingServerProxy
	peerConnStore *PeerConnStore
	webrtcAPI     *webrtc.API
	iceServers    []webrtc.ICEServer
	debug         bool
}

// NewWebRTCHandler creates a new WebRTC handler
func NewWebRTCHandler(iceServers []string, debug bool) *WebRTCHandler {
	// Convert string ICE servers to webrtc.ICEServer
	var servers []webrtc.ICEServer
	for _, server := range iceServers {
		servers = append(servers, webrtc.ICEServer{
			URLs: []string{server},
		})
	}

	return &WebRTCHandler{
		peerConnStore: NewPeerConnStore(),
		webrtcAPI:     webrtc.NewAPI(),
		iceServers:    servers,
		debug:         debug,
	}
}

// Run starts the WebRTC handler
func (h *WebRTCHandler) Run(ctx context.Context, signalling SignallingServerProxy) {
	h.signalling = signalling

	for {
		select {
		case <-ctx.Done():
			h.cleanup()
			return
		case msg, ok := <-signalling.Receive():
			if !ok {
				return
			}
			h.handleMessage(ctx, msg)
		}
	}
}

// handleMessage processes incoming signalling messages
func (h *WebRTCHandler) handleMessage(ctx context.Context, msg pkgframing.MessagePayload) {
	// Handle node ID from registration response
	if msg.NodeId != "" {
		h.signalling.SetNodeID(msg.NodeId)
		log.Printf("Registered with node ID: %s", msg.NodeId)
		return
	}

	// Handle SDP offer
	if sdpOffer := msg.SDPOffer; sdpOffer != nil {
		h.handleSDPOffer(ctx, sdpOffer)
		return
	}

	// Handle ICE offer
	if iceOffer := msg.ICEOffer; iceOffer != nil {
		h.handleICEOffer(ctx, iceOffer)
		return
	}
}

// handleSDPOffer handles SDP offer/answer messages
func (h *WebRTCHandler) handleSDPOffer(ctx context.Context, sdpOffer *pkgconnreg.SDPOfferPayload) {
	myNodeID := h.signalling.GetNodeID()
	if sdpOffer.ToNodeId != myNodeID {
		return
	}

	remoteNodeID := sdpOffer.FromNodeId
	log.Printf("[webrtc] Received SDP offer from peer %s, type: %s", remoteNodeID, sdpOffer.Type)

	// Get or create peer connection entry
	entry, found := h.peerConnStore.Get(remoteNodeID)
	if !found {
		var createErr error
		entry, createErr = h.createPeerConnection(remoteNodeID)
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
				FromNodeId: myNodeID,
				ToNodeId:   remoteNodeID,
			},
		}

		if err := h.signalling.Send(ctx, answerMsg); err != nil {
			log.Printf("Failed to send SDP answer: %v", err)
			return
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
}

// handleICEOffer handles ICE candidate messages
func (h *WebRTCHandler) handleICEOffer(ctx context.Context, iceOffer *pkgconnreg.ICEOfferPayload) {
	myNodeID := h.signalling.GetNodeID()
	if iceOffer.ToNodeId != myNodeID {
		return
	}

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
func (h *WebRTCHandler) createPeerConnection(remoteNodeID string) (*PeerConnEntry, error) {
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
			h.setupChatDataChannel(dc, remoteNodeID)
		case PredefinedDCLabelFile:
			dcID := dc.ID()
			if dcID != nil {
				entry.mu.Lock()
				entry.FileDataChannels[fmt.Sprintf("%d", *dcID)] = dc
				entry.mu.Unlock()
			}
			h.setupFileDataChannel(dc, remoteNodeID)
		case PredefinedDCLabelPing:
			h.setupPingDataChannel(dc, remoteNodeID)
		default:
			log.Printf("[webrtc] Unknown data channel label: %s", dc.Label())
		}
	})

	// Set up ICE candidate handler
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		myNodeID := h.signalling.GetNodeID()
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

		if err := h.signalling.Send(context.Background(), iceOfferMsg); err != nil {
			log.Printf("Failed to send ICE offer: %v", err)
			return
		}

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
					if err := h.initiateICERestart(entry, remoteNodeID); err != nil {
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
				if err := h.initiateICERestart(entry, remoteNodeID); err != nil {
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
func (h *WebRTCHandler) initiateICERestart(entry *PeerConnEntry, remoteNodeID string) error {
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

	myNodeID := h.signalling.GetNodeID()

	offerMsg := pkgframing.MessagePayload{
		SDPOffer: &pkgconnreg.SDPOfferPayload{
			Type:       pkgconnreg.OfferTypeOffer,
			OfferJSON:  string(offerJSON),
			FromNodeId: myNodeID,
			ToNodeId:   remoteNodeID,
		},
	}

	if err := h.signalling.Send(context.Background(), offerMsg); err != nil {
		return fmt.Errorf("failed to send ICE restart offer: %w", err)
	}

	log.Printf("[webrtc] Sent ICE restart offer to peer %s", remoteNodeID)
	return nil
}

// cleanup closes all peer connections
func (h *WebRTCHandler) cleanup() {
	h.peerConnStore.Walk(func(remoteNodeID string, entry *PeerConnEntry) (bool, error) {
		if err := entry.PeerConnection.Close(); err != nil {
			log.Printf("Failed to close peer connection to %s: %v", remoteNodeID, err)
		}
		return true, nil
	})
}

// containsICERestartOption checks if SDP contains ICE restart option
func containsICERestartOption(sdp string) bool {
	// Look for "a=ice-options:restart" in the SDP
	return len(sdp) > 0 && (contains(sdp, "a=ice-options:restart") || contains(sdp, "ice-options:restart"))
}

// contains checks if s contains substr (case-sensitive)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// setupChatDataChannel sets up event handlers for chat data channel
func (h *WebRTCHandler) setupChatDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
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
func (h *WebRTCHandler) setupFileDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
	dc.OnOpen(func() {
		log.Printf("[webrtc] File data channel opened with peer %s", remoteNodeID)
	})

	dc.OnClose(func() {
		log.Printf("[webrtc] File data channel closed with peer %s", remoteNodeID)
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if h.debug {
			log.Printf("[webrtc] Received file data from peer %s, size: %d bytes", remoteNodeID, len(msg.Data))
		}
	})

	dc.OnError(func(err error) {
		log.Printf("[webrtc] File data channel error with peer %s: %v", remoteNodeID, err)
	})
}

// setupPingDataChannel sets up event handlers for ping data channel
func (h *WebRTCHandler) setupPingDataChannel(dc *webrtc.DataChannel, remoteNodeID string) {
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
