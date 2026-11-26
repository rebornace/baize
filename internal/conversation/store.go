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
	// SearchAll searches messages across conversations (newest first).
	// limit caps results (default 20, max 50). Empty query returns nil.
	SearchAll(query string, limit int) ([]Message, error)
	// SearchAllFiltered is SearchAll with optional conversation, role and time
	// filters. Zero fields mean no restriction; empty query returns nil.
	SearchAllFiltered(filter SearchFilter) ([]Message, error)
	ListSummaries() []Summary
	// ListConversationIDs returns conversations that currently have messages.
	ListConversationIDs() []string
	// DeleteMessagesBefore removes messages older than cutoff for one
	// conversation, always keeping the newest `keep` messages.
	DeleteMessagesBefore(conversationID string, cutoff time.Time, keep int) (int, error)
	// CountMessagesBefore counts what DeleteMessagesBefore would remove.
	CountMessagesBefore(conversationID string, cutoff time.Time, keep int) (int, error)
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

// SearchFilter narrows a cross-conversation message search.
type SearchFilter struct {
	Query          string
	ConversationID string
	Roles          []string
	Since          time.Time // inclusive
	Until          time.Time // exclusive
	Limit          int
}

// normalized trims and deduplicates inputs and clamps the result limit.
func (f SearchFilter) normalized() SearchFilter {
	f.Query = strings.TrimSpace(f.Query)
	f.ConversationID = strings.TrimSpace(f.ConversationID)
	if len(f.Roles) > 0 {
		seen := make(map[string]bool, len(f.Roles))
		roles := make([]string, 0, len(f.Roles))
		for _, role := range f.Roles {
			role = strings.TrimSpace(role)
			if role == "" || seen[role] {
				continue
			}
			seen[role] = true
			roles = append(roles, role)
		}
		f.Roles = roles
	}
	f.Limit = clampSearchAllLimit(f.Limit)
	return f
}

// matches applies role and time filters; the query and conversation are
// handled by each store's native search path.
func (f SearchFilter) matches(m Message) bool {
	if len(f.Roles) > 0 {
		found := false
		for _, role := range f.Roles {
			if m.Role == role {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if !f.Since.IsZero() && m.CreatedAt.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && !m.CreatedAt.Before(f.Until) {
		return false
	}
	return true
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

func (s *MemoryStore) UpdateMetaWorkspace(id, workspaceID string) error {
	if id == "" {
		return fmt.Errorf("meta id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.meta[id]
	if !ok {
		return ErrMetaNotFound
	}
	m.WorkspaceID = NormalizeWorkspaceID(workspaceID)
	m.UpdatedAt = time.Now().UTC()
	s.meta[id] = m
	return nil
}

func (s *MemoryStore) UpdateMetaTitle(id, title string) error {
	if id == "" {
		return fmt.Errorf("meta id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.meta[id]
	if !ok {
		return ErrMetaNotFound
	}
	m.Title = title
	m.UpdatedAt = time.Now().UTC()
	s.meta[id] = m
	return nil
}

func (s *MemoryStore) UpdateMetaArchived(id string, archived bool) error {
	if id == "" {
		return fmt.Errorf("meta id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.meta[id]
	if !ok {
		return ErrMetaNotFound
	}
	if archived {
		m.ArchivedAt = time.Now().UTC()
	} else {
		m.ArchivedAt = time.Time{}
	}
	m.UpdatedAt = time.Now().UTC()
	s.meta[id] = m
	return nil
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
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}

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

func (s *MemoryStore) SearchAll(query string, limit int) ([]Message, error) {
	return s.SearchAllFiltered(SearchFilter{Query: query, Limit: limit})
}

func (s *MemoryStore) SearchAllFiltered(filter SearchFilter) ([]Message, error) {
	filter = filter.normalized()
	if filter.Query == "" {
		return nil, nil
	}
	s.mu.RLock()
	var out []Message
	for convID, msgs := range s.msgs {
		if filter.ConversationID != "" && convID != filter.ConversationID {
			continue
		}
		for i := len(msgs) - 1; i >= 0; i-- {
			if !strings.Contains(msgs[i].Content, filter.Query) || !filter.matches(msgs[i]) {
				continue
			}
			out = append(out, msgs[i])
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

func (s *MemoryStore) ListConversationIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.msgs))
	for id, msgs := range s.msgs {
		if len(msgs) > 0 {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (s *MemoryStore) DeleteMessagesBefore(conversationID string, cutoff time.Time, keep int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := s.msgs[conversationID]
	if len(msgs) == 0 {
		return 0, nil
	}
	if keep < 0 {
		keep = 0
	}
	protected := make(map[string]bool, keep)
	if keep > 0 {
		start := len(msgs) - keep
		if start < 0 {
			start = 0
		}
		for _, m := range msgs[start:] {
			protected[m.ID] = true
		}
	}
	kept := msgs[:0]
	deleted := 0
	for _, m := range msgs {
		if !protected[m.ID] && m.CreatedAt.Before(cutoff) {
			deleted++
			continue
		}
		kept = append(kept, m)
	}
	if len(kept) == 0 {
		delete(s.msgs, conversationID)
	} else {
		s.msgs[conversationID] = kept
	}
	return deleted, nil
}

func (s *MemoryStore) CountMessagesBefore(conversationID string, cutoff time.Time, keep int) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs := s.msgs[conversationID]
	if len(msgs) == 0 {
		return 0, nil
	}
	if keep < 0 {
		keep = 0
	}
	protected := make(map[string]bool, keep)
	if keep > 0 {
		start := len(msgs) - keep
		if start < 0 {
			start = 0
		}
		for _, m := range msgs[start:] {
			protected[m.ID] = true
		}
	}
	count := 0
	for _, m := range msgs {
		if !protected[m.ID] && m.CreatedAt.Before(cutoff) {
			count++
		}
	}
	return count, nil
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
