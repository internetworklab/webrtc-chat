package msgs_store

import (
	"sync"
	"testing"
)

// Index by sender id
type IndexedMsgsCollection struct {
	store map[string][]interface{}
}

func NewIndexedMsgsCollection() *IndexedMsgsCollection {
	return &IndexedMsgsCollection{
		store: make(map[string][]interface{}),
	}
}

type IdentifiableMessage interface {
	GetSenderId() string
}

func (indexColl *IndexedMsgsCollection) DeepClone() MsgsCollection {
	newMap := make(map[string][]interface{})
	for senderId, li := range indexColl.store {
		newList := make([]interface{}, len(li))
		copy(newList, li)
		newMap[senderId] = newList
	}
	newIndexColl := new(IndexedMsgsCollection)
	newIndexColl.store = newMap
	return newIndexColl
}

func (indexColl *IndexedMsgsCollection) Append(msg interface{}) {
	senderId := ""
	if sender, ok := msg.(IdentifiableMessage); ok {
		senderId = sender.GetSenderId()
	}
	indexColl.store[senderId] = append(indexColl.store[senderId], msg)
}

// GetMessagesBySenderId returns messages for a specific sender
func (indexColl *IndexedMsgsCollection) GetMessagesBySenderId(senderId string) []interface{} {
	return indexColl.store[senderId]
}

// MockMessage is a mock message for testing
type MockMessage struct {
	Sender string
	Seq    int
}

type Comparable interface {
	IsEqual(rhs Comparable) bool
}

func (msg *MockMessage) IsEqual(rhs Comparable) bool {
	if rhsMsg, ok := rhs.(*MockMessage); ok && rhsMsg != nil {
		return msg.Sender == rhsMsg.Sender && msg.Seq == rhsMsg.Seq
	}
	return false
}

// GetSenderId implements IdentifiableMessage interface
func (m *MockMessage) GetSenderId() string {
	return m.Sender
}

// Sender wraps SyncMsgsStore and sends messages with a specific sender name
type Sender struct {
	store  *SyncMsgsStore
	sender string
}

// NewSender creates a new Sender
func NewSender(store *SyncMsgsStore, sender string) *Sender {
	return &Sender{
		store:  store,
		sender: sender,
	}
}

// SendMessages sends N fake messages to the store
func (s *Sender) SendMessages(n int) error {
	for i := 0; i < n; i++ {
		msg := &MockMessage{
			Sender: s.sender,
			Seq:    i,
		}
		if err := s.store.Append(msg); err != nil {
			return err
		}
	}
	return nil
}

// CommitMessage loads the store, checks for existing messages from this sender,
// and appends a new message with incremented sequence number
func (s *Sender) CommitMessage() error {
	store := s.store.Load()

	var newSeq int
	if store != nil {
		coll := store.Load().(*IndexedMsgsCollection)
		messages := coll.store[s.sender]
		if len(messages) > 0 {
			lastMsg := messages[len(messages)-1].(*MockMessage)
			newSeq = lastMsg.Seq + 1
		}
	}

	msg := &MockMessage{
		Sender: s.sender,
		Seq:    newSeq,
	}

	return s.store.Append(msg)
}

// CommitMessages calls CommitMessage n times
func (s *Sender) CommitMessages(n int) error {
	for i := 0; i < n; i++ {
		if err := s.CommitMessage(); err != nil {
			return err
		}
	}
	return nil
}

func TestConcurrentSendMessages(t *testing.T) {
	store := NewSyncMsgsStore(func() MsgsCollection {
		return NewIndexedMsgsCollection()
	})

	// Define test cases with different senders and message counts
	testCases := []struct {
		sender string
		count  int
	}{
		{"sender1", 100},
		{"sender2", 150},
		{"sender3", 200},
		{"sender4", 50},
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(testCases))

	for _, tc := range testCases {
		wg.Add(1)
		go func(sender string, count int) {
			defer wg.Done()
			s := NewSender(store, sender)
			if err := s.SendMessages(count); err != nil {
				errChan <- err
			}
		}(tc.sender, tc.count)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			t.Errorf("Error sending messages: %v", err)
		}
	}

	// Verify the store contains all messages
	result := store.Load()
	if result == nil {
		t.Fatal("Store is nil after sending messages")
	}

	// Count total messages expected
	totalExpected := 0
	for _, tc := range testCases {
		totalExpected += tc.count
	}

	// Verify we have the correct number of messages
	coll := result.Load().(*IndexedMsgsCollection)
	totalActual := 0
	for _, messages := range coll.store {
		totalActual += len(messages)
	}

	if totalActual != totalExpected {
		t.Errorf("Expected %d messages, got %d", totalExpected, totalActual)
	}

	// Verify each sender has the correct number of messages
	for _, tc := range testCases {
		messages := coll.store[tc.sender]
		if len(messages) != tc.count {
			t.Errorf("Sender %s: expected %d messages, got %d", tc.sender, tc.count, len(messages))
		}
	}

	// Verify each message content using Comparable interface
	for _, tc := range testCases {
		messages := coll.store[tc.sender]
		for i, msg := range messages {
			actualMsg, ok := msg.(*MockMessage)
			if !ok {
				t.Errorf("Sender %s, message %d: failed to cast to MockMessage", tc.sender, i)
				continue
			}

			expectedMsg := &MockMessage{
				Sender: tc.sender,
				Seq:    i,
			}

			if !actualMsg.IsEqual(expectedMsg) {
				t.Errorf("Sender %s, message %d: expected {Sender: %s, Seq: %d}, got {Sender: %s, Seq: %d}",
					tc.sender, i, expectedMsg.Sender, expectedMsg.Seq, actualMsg.Sender, actualMsg.Seq)
			}
		}
	}
}

func TestSenderSendMessages(t *testing.T) {
	store := NewSyncMsgsStore(func() MsgsCollection {
		return NewIndexedMsgsCollection()
	})
	sender := NewSender(store, "test-sender")

	err := sender.SendMessages(10)
	if err != nil {
		t.Fatalf("SendMessages returned error: %v", err)
	}

	result := store.Load()
	if result == nil {
		t.Fatal("Store is nil after sending messages")
	}

	coll := result.Load().(*IndexedMsgsCollection)
	messages := coll.store["test-sender"]

	if len(messages) != 10 {
		t.Errorf("Expected 10 messages, got %d", len(messages))
	}

	// Verify sequence numbers
	for i, msg := range messages {
		mockMsg := msg.(*MockMessage)
		if mockMsg.Seq != i {
			t.Errorf("Message %d: expected seq %d, got %d", i, i, mockMsg.Seq)
		}
		if mockMsg.Sender != "test-sender" {
			t.Errorf("Message %d: expected sender 'test-sender', got '%s'", i, mockMsg.Sender)
		}
	}
}

func TestConcurrentReadWrites(t *testing.T) {
	store := NewSyncMsgsStore(func() MsgsCollection {
		return NewIndexedMsgsCollection()
	})

	// Define test cases with different senders and message counts
	testCases := []struct {
		sender string
		count  int
	}{
		{"reader1", 50},
		{"reader2", 75},
		{"reader3", 100},
		{"reader4", 25},
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(testCases))

	for _, tc := range testCases {
		wg.Add(1)
		go func(sender string, count int) {
			defer wg.Done()
			s := NewSender(store, sender)
			if err := s.CommitMessages(count); err != nil {
				errChan <- err
			}
		}(tc.sender, tc.count)
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			t.Errorf("Error committing messages: %v", err)
		}
	}

	// Verify the store contains all messages
	result := store.Load()
	if result == nil {
		t.Fatal("Store is nil after committing messages")
	}

	// Count total messages expected
	totalExpected := 0
	for _, tc := range testCases {
		totalExpected += tc.count
	}

	// Verify we have the correct number of messages
	coll := result.Load().(*IndexedMsgsCollection)
	totalActual := 0
	for _, messages := range coll.store {
		totalActual += len(messages)
	}

	if totalActual != totalExpected {
		t.Errorf("Expected %d messages, got %d", totalExpected, totalActual)
	}

	// Verify each sender has the correct number of messages
	for _, tc := range testCases {
		messages := coll.store[tc.sender]
		if len(messages) != tc.count {
			t.Errorf("Sender %s: expected %d messages, got %d", tc.sender, tc.count, len(messages))
		}
	}

	// Verify each message content using Comparable interface
	for _, tc := range testCases {
		messages := coll.store[tc.sender]

		for i, msg := range messages {
			actualMsg, ok := msg.(*MockMessage)
			if !ok {
				t.Errorf("Sender %s, message %d: failed to cast to MockMessage", tc.sender, i)
				continue
			}

			expectedMsg := &MockMessage{
				Sender: tc.sender,
				Seq:    i,
			}

			if !actualMsg.IsEqual(expectedMsg) {
				t.Errorf("Sender %s, message %d: expected {Sender: %s, Seq: %d}, got {Sender: %s, Seq: %d}",
					tc.sender, i, expectedMsg.Sender, expectedMsg.Seq, actualMsg.Sender, actualMsg.Seq)
			}
		}
	}
}
