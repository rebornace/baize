package conversation

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store persists conversation messages.
type Store interface {
	Append(conversationID string, msg Message) (Message, error)
	List(conversationID string) []Message
	ListWindow(conversationID string, n int) []Message
	Clear(conversationID string)
}

// MemoryStore is an in-memory Store implementation.
type MemoryStore struct {
	mu    sync.RWMutex
	msgs  map[string][]Message
}

// NewMemoryStore creates an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		msgs: map[string][]Message{},
	}
}

func (s *MemoryStore) Append(conversationID string, msg Message) (Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg.ID = "msg_" + uuid.NewString()
	msg.ConversationID = conversationID
	msg.CreatedAt = time.Now().UTC()

	s.msgs[conversationID] = append(s.msgs[conversationID], msg)
	return msg, nil
}

func (s *MemoryStore) List(conversationID string) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	msgs := s.msgs[conversationID]
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out
}

func (s *MemoryStore) ListWindow(conversationID string, n int) []Message {
	all := s.List(conversationID)
	if n <= 0 || n >= len(all) {
		return all
	}
	return all[len(all)-n:]
}

func (s *MemoryStore) Clear(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.msgs, conversationID)
}
