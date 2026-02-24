package handlers

import (
	"sync"

	pkgsafemap "example.com/webrtcserver/pkg/safemap"

	"github.com/pion/webrtc/v4"
)

// PeerConnEntry tracks a peer connection and its associated data
type PeerConnEntry struct {
	PeerConnection        *webrtc.PeerConnection
	DataChannel           *webrtc.DataChannel
	FileDataChannels      map[string]*webrtc.DataChannel
	RemoteOffers          []webrtc.SessionDescription
	QueuedICEOffers       []webrtc.ICECandidateInit
	ICERestartAttempts    int
	MaxICERestartAttempts int
	CurrentTrackSender    *webrtc.RTPSender // Track the current active track sender for this peer
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
