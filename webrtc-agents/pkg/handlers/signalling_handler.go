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

	"github.com/pion/webrtc/v4"
)

// SignallingHandler handles WebRTC peer connections
type SignallingHandler struct {
	peerConnStore     *PeerConnStore
	webrtcAPI         *webrtc.API
	iceServers        []webrtc.ICEServer
	debug             bool
	nodeID            string
	dcHandler         DCHandler
	generalProperties sync.Map
}

func (h *SignallingHandler) GetNodeID() string {
	val, ok := h.generalProperties.Load(GP_Key_NodeId)
	if !ok {
		return ""
	}
	if nodeID, isValid := val.(string); isValid {
		return nodeID
	}
	return ""
}

func (h *SignallingHandler) SetNodeID(nodeID string) {
	h.generalProperties.Store(GP_Key_NodeId, nodeID)
}

// NewSignallingHandler creates a new WebRTC handler with MediaEngine configured for Opus codec only.
func NewSignallingHandler(dcHandler DCHandler, iceServers []string, debug bool, api *webrtc.API) *SignallingHandler {
	// Convert string ICE servers to webrtc.ICEServer
	var servers []webrtc.ICEServer
	servers = append(servers, webrtc.ICEServer{
		URLs: iceServers,
	})

	if api == nil {
		api = webrtc.NewAPI()
	}

	handler := &SignallingHandler{
		peerConnStore: NewPeerConnStore(),
		webrtcAPI:     api,
		iceServers:    servers,
		debug:         debug,
		dcHandler:     dcHandler,
	}

	return handler
}

// Serve starts the WebRTC handler
func (h *SignallingHandler) Serve(ctx context.Context, signallingTx chan<- pkgframing.MessagePayload, signallingRx <-chan pkgframing.MessagePayload) {

	for {
		select {
		case <-ctx.Done():
			h.cleanup()
			return
		case msg, ok := <-signallingRx:
			if !ok {
				return
			}
			h.handleMessage(msg, signallingTx)
		}
	}
}

// handleMessage processes incoming signalling messages
func (h *SignallingHandler) handleMessage(msg pkgframing.MessagePayload, signallingTx chan<- pkgframing.MessagePayload) {
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
func (h *SignallingHandler) handleSDPOffer(sdpOffer *pkgconnreg.SDPOfferPayload, signallingTx chan<- pkgframing.MessagePayload) {

	remoteNodeID := sdpOffer.FromNodeId
	log.Printf("[webrtc] Received SDP offer from peer %s, type: %s", remoteNodeID, sdpOffer.Type)

	// Get or create peer connection entry
	entry, found := h.peerConnStore.GetPeerConnEntry(remoteNodeID)
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
func (h *SignallingHandler) handleICEOffer(iceOffer *pkgconnreg.ICEOfferPayload) {

	remoteNodeID := iceOffer.FromNodeId
	if h.debug {
		log.Printf("[webrtc] Received ICE candidate from peer %s", remoteNodeID)
	}

	entry, found := h.peerConnStore.GetPeerConnEntry(remoteNodeID)
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
func (h *SignallingHandler) createPeerConnection(remoteNodeID string, signallingTx chan<- pkgframing.MessagePayload) (*PeerConnEntry, error) {
	config := webrtc.Configuration{
		ICEServers: h.iceServers,
	}

	// NewPeerConnection uses the MediaEngine from the API to constrain codec negotiation
	pc, err := h.webrtcAPI.NewPeerConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	entry := &PeerConnEntry{
		PeerConnection:     pc,
		RemoteOffers:       make([]webrtc.SessionDescription, 0),
		QueuedICEOffers:    make([]webrtc.ICECandidateInit, 0),
		ICERestartAttempts: 0,
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, DCHandlerCtxRemoteNodeID, remoteNodeID)
	ctx = context.WithValue(ctx, DCHandlerCtxOurNodeID, h.GetNodeID())
	ctx = context.WithValue(ctx, DCHandlerCtxPeerConnStore, h.peerConnStore)

	// Set up data channel handler for incoming data channels
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		h.dcHandler.Serve(ctx, dc, signallingTx)
		log.Printf("[webrtc] Received data channel: %s from peer %s", dc.Label(), remoteNodeID)
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
		case webrtc.PeerConnectionStateDisconnected, webrtc.PeerConnectionStateFailed:
			// Wait briefly for Disconnected state, immediate for Failed
			if state == webrtc.PeerConnectionStateDisconnected {
				<-time.After(3 * time.Second)
			}
			if err := entry.InitiateICERestart(h.GetNodeID(), remoteNodeID, signallingTx); err != nil {
				log.Printf("[webrtc] ICE restart failed for peer %s: %v", remoteNodeID, err)
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

// cleanup closes all peer connections
func (h *SignallingHandler) cleanup() {
	h.peerConnStore.Walk(func(remoteNodeID string, entry *PeerConnEntry) (bool, error) {
		if err := entry.PeerConnection.Close(); err != nil {
			log.Printf("Failed to close peer connection to %s: %v", remoteNodeID, err)
		}
		return true, nil
	})
}
