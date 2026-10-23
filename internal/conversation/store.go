package conversation

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store persists conversation messages.
type Store interface {
	Append(conversationID string, msg Message) (Message, error)
	List(conversationID string) []Message
	ListWindow(conversationID string, n int) []Message
	// Search returns messages in conversationID whose content matches query
	// (FTS5 on SQLite; substring on MemoryStore; ILIKE/tsquery on Postgres).
	// topK caps results (default 8, max 20). Empty query returns nil.
	Search(conversationID, query string, topK int) ([]Message, error)
	ListSummaries() []Summary
	Clear(conversationID string)
	TruncateFrom(conversationID, messageID string) (deleted int, err error)
	Fork(srcConversationID, throughMessageID string) (newConversationID string, copied int, err error)
	// SetRunID rebinds a persisted user turn to a new run (regenerate without duplicating the user row).
	SetRunID(messageID, runID string) error
	// Rolling summary (context compaction). Derived from messages; safe to drop.
	GetRollingSummary(conversationID string) (RollingSummary, bool)
	UpsertRollingSummary(s RollingSummary) error
	ClearRollingSummary(conversationID string)
	GetContextProjection(conversationID string) (ContextProjection, bool)
	UpsertContextProjection(p ContextProjection) error
	ClearContextProjection(conversationID string)
	GetGoalStack(conversationID string) (GoalStack, bool)
	UpsertGoalStack(s GoalStack) error
	ClearGoalStack(conversationID string)
}

// MemoryStore is an in-memory Store implementation.
type MemoryStore struct {
	mu          sync.RWMutex
	msgs        map[string][]Message
	meta        map[string]Meta
	summaries   map[string]RollingSummary
	projections map[string]ContextProjection
	goals       map[string]GoalStack
	workspaces  map[string]Workspace
}

// NewMemoryStore creates an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		msgs:        map[string][]Message{},
		meta:        map[string]Meta{},
		summaries:   map[string]RollingSummary{},
		projections: map[string]ContextProjection{},
		goals:       map[string]GoalStack{},
		workspaces:  map[string]Workspace{},
	}
}

var _ MetaStore = (*MemoryStore)(nil)

func (s *MemoryStore) EnsureMeta(m Meta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meta == nil {
		s.meta = map[string]Meta{}
	}
	if _, exists := s.meta[m.ID]; exists {
		return nil
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now().UTC()
	}
	if m.Source == "ui" && strings.TrimSpace(m.WorkspaceID) == "" {
		m.WorkspaceID = DefaultWorkspaceID
	}
	s.meta[m.ID] = m
	return nil
}

func (s *MemoryStore) GetMeta(id string) (Meta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.meta[id]
	if !ok {
		return Meta{}, ErrMetaNotFound
	}
	return m, nil
}

func (s *MemoryStore) ListMeta(filter MetaFilter) ([]Meta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Meta, 0, len(s.meta))
	for _, m := range s.meta {
		if filter.OwnerID != "" && m.OwnerID != filter.OwnerID {
			continue
		}
		if filter.WorkspaceID != "" && m.WorkspaceID != filter.WorkspaceID {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// DeleteMeta removes the meta row. Deleting a missing id is a no-op.
func (s *MemoryStore) DeleteMeta(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.meta, id)
	return nil
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

func (s *MemoryStore) Search(conversationID, query string, topK int) ([]Message, error) {
	topK = clampSearchTopK(topK)
	q := strings.TrimSpace(query)
	if conversationID == "" || q == "" {
		return nil, nil
	}
	all := s.List(conversationID)
	out := make([]Message, 0)
	// Newest first to match SQL Search ordering.
	for i := len(all) - 1; i >= 0; i-- {
		if !strings.Contains(all[i].Content, q) {
			continue
		}
		out = append(out, all[i])
		if len(out) >= topK {
			break
		}
	}
	return out, nil
}

func (s *MemoryStore) ListSummaries() []Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Summary, 0, len(s.msgs))
	for id, msgs := range s.msgs {
		if len(msgs) == 0 {
			continue
		}
		out = append(out, Summarize(id, msgs))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (s *MemoryStore) Clear(conversationID string) {
	s.mu.Lock()
	delete(s.msgs, conversationID)
	delete(s.summaries, conversationID)
	delete(s.projections, conversationID)
	s.mu.Unlock()
	s.ClearGoalStack(conversationID)
}

func (s *MemoryStore) GetRollingSummary(conversationID string) (RollingSummary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.summaries[conversationID]
	return v, ok
}

func (s *MemoryStore) UpsertRollingSummary(sum RollingSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sum.ConversationID == "" {
		return fmt.Errorf("conversation id required")
	}
	if s.summaries == nil {
		s.summaries = map[string]RollingSummary{}
	}
	if sum.UpdatedAt.IsZero() {
		sum.UpdatedAt = time.Now().UTC()
	}
	s.summaries[sum.ConversationID] = sum
	return nil
}

func (s *MemoryStore) ClearRollingSummary(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.summaries, conversationID)
}

func (s *MemoryStore) GetContextProjection(conversationID string) (ContextProjection, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.projections[conversationID]
	return v, ok
}

func (s *MemoryStore) UpsertContextProjection(p ContextProjection) error {
	if err := validateProjection(p); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.projections == nil {
		s.projections = map[string]ContextProjection{}
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	s.projections[p.ConversationID] = p
	return nil
}

func (s *MemoryStore) ClearContextProjection(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.projections, conversationID)
}

func (s *MemoryStore) GetGoalStack(conversationID string) (GoalStack, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.goals[conversationID]
	return v, ok
}

func (s *MemoryStore) UpsertGoalStack(stack GoalStack) error {
	if err := validateGoalStack(stack); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.goals == nil {
		s.goals = map[string]GoalStack{}
	}
	if stack.UpdatedAt.IsZero() {
		stack.UpdatedAt = time.Now().UTC()
	}
	s.goals[stack.ConversationID] = stack
	return nil
}

func (s *MemoryStore) ClearGoalStack(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearGoalStackLocked(conversationID)
}

func (s *MemoryStore) TruncateFrom(conversationID, messageID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgs := s.msgs[conversationID]
	anchorIdx := -1
	for i, m := range msgs {
		if m.ID == messageID {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		return 0, ErrMessageNotFound
	}
	deleted := len(msgs) - anchorIdx
	s.msgs[conversationID] = msgs[:anchorIdx]
	delete(s.summaries, conversationID)
	delete(s.projections, conversationID)
	s.clearGoalStackLocked(conversationID)
	return deleted, nil
}

func (s *MemoryStore) clearGoalStackLocked(conversationID string) {
	delete(s.goals, conversationID)
}

func (s *MemoryStore) Fork(srcConversationID, throughMessageID string) (string, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	src := s.msgs[srcConversationID]
	anchorIdx := -1
	for i, m := range src {
		if m.ID == throughMessageID {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		return "", 0, ErrMessageNotFound
	}
	newID := "conv_" + uuid.NewString()
	prefix := src[:anchorIdx+1]
	copied := make([]Message, 0, len(prefix))
	for _, m := range prefix {
		cp := m
		cp.ID = "msg_" + uuid.NewString()
		cp.ConversationID = newID
		copied = append(copied, cp)
	}
	s.msgs[newID] = copied
	if srcStack, ok := s.goals[srcConversationID]; ok && len(srcStack.Frames) > 0 {
		if s.goals == nil {
			s.goals = map[string]GoalStack{}
		}
		s.goals[newID] = cloneGoalStackForFork(srcStack, newID)
	}
	return newID, len(copied), nil
}

func (s *MemoryStore) SetRunID(messageID, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for convID, msgs := range s.msgs {
		for i, m := range msgs {
			if m.ID == messageID {
				msgs[i].RunID = runID
				s.msgs[convID] = msgs
				return nil
			}
		}
	}
	return ErrMessageNotFound
}
