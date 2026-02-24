package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	pkgconnreg "example.com/webrtcserver/pkg/connreg"
	pkgframing "example.com/webrtcserver/pkg/framing"
	pkgsafemap "example.com/webrtcserver/pkg/safemap"

	"github.com/pion/webrtc/v4"
)

const (
	// MaxICERestartAttempts is the maximum number of ICE restart attempts before giving up
	MaxICERestartAttempts = 3
)

// PeerConnEntry tracks a peer connection and its associated data
type PeerConnEntry struct {
	PeerConnection     *webrtc.PeerConnection
	RemoteOffers       []webrtc.SessionDescription
	QueuedICEOffers    []webrtc.ICECandidateInit
	ICERestartAttempts int
	CurrentTrackSender *webrtc.RTPSender // Track the current active track sender for this peer
	mu                 sync.RWMutex
}

// NextICERestartAttempt increments the ICE restart attempts counter and returns
// the current attempt number and whether another restart attempt is allowed
// (true if attempts <= MaxICERestartAttempts).
// This method is thread-safe.
func (e *PeerConnEntry) NextICERestartAttempt() (int, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ICERestartAttempts++
	return e.ICERestartAttempts, e.ICERestartAttempts <= MaxICERestartAttempts
}

// InitiateICERestart creates an ICE restart offer, sets it as the local description,
// and sends it via the signalling channel. Returns error if max attempts exceeded.
// This method is thread-safe.
func (e *PeerConnEntry) InitiateICERestart(fromNodeID, toNodeID string, signallingTx chan<- pkgframing.MessagePayload) error {
	attempts, ok := e.NextICERestartAttempt()
	if !ok {
		return fmt.Errorf("max ICE restart attempts (%d) exceeded", MaxICERestartAttempts)
	}

	log.Printf("[webrtc] Initiating ICE restart for peer %s (attempt %d/%d)", toNodeID, attempts, MaxICERestartAttempts)

	// Create offer with ICE restart
	offer, err := e.PeerConnection.CreateOffer(&webrtc.OfferOptions{
		ICERestart: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create ICE restart offer: %w", err)
	}

	// Set local description
	if err := e.PeerConnection.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Marshal the offer to JSON
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return fmt.Errorf("failed to marshal offer: %w", err)
	}

	// Send the offer via signalling channel
	offerMsg := pkgframing.MessagePayload{
		SDPOffer: &pkgconnreg.SDPOfferPayload{
			Type:       pkgconnreg.OfferTypeOffer,
			OfferJSON:  string(offerJSON),
			FromNodeId: fromNodeID,
			ToNodeId:   toNodeID,
		},
	}
	signallingTx <- offerMsg

	log.Printf("[webrtc] Sent ICE restart offer to peer %s", toNodeID)
	return nil
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

// GetPeerConnEntry retrieves a peer connection entry
func (s *PeerConnStore) GetPeerConnEntry(remoteNodeID string) (*PeerConnEntry, bool) {
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
