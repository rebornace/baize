package identity

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Store holds per-conversation identities.
type Store interface {
	Upsert(conversationID string, id Identity) (string, error)
	List(conversationID string) []Identity
	Get(conversationID, id string) (Identity, error)
	ListPublic(conversationID string) []PublicView
	Delete(conversationID, id string) error
	SetDefault(conversationID, id string) error
	ClearCaptured(conversationID string)
	Touch(conversationID, id string) error
}

// MemoryStore is an in-process Identity store keyed by conversation ID.
type MemoryStore struct {
	mu     sync.RWMutex
	byConv map[string]map[string]*Identity // conversationID -> identityID -> Identity
}

// NewMemoryStore creates an empty in-memory identity store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byConv: map[string]map[string]*Identity{},
	}
}

// Upsert inserts or updates an identity. Same conversation + scheme + subject reuses the existing id.
func (s *MemoryStore) Upsert(conversationID string, id Identity) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		conv = map[string]*Identity{}
		s.byConv[conversationID] = conv
	}

	var existing *Identity
	for _, item := range conv {
		if item.Scheme == id.Scheme && item.Subject == id.Subject {
			existing = item
			break
		}
	}

	now := time.Now().UTC()
	if existing != nil {
		existing.Label = id.Label
		existing.CredentialHeaders = cloneHeaders(id.CredentialHeaders)
		if id.ClaimsSummary != nil {
			existing.ClaimsSummary = cloneClaims(id.ClaimsSummary)
		}
		if !id.UpdatedAt.IsZero() {
			existing.UpdatedAt = id.UpdatedAt
		} else {
			existing.UpdatedAt = now
		}
		if id.IsDefault {
			s.clearDefaultLocked(conv, id.Scheme, existing.ID)
			existing.IsDefault = true
		}
		return existing.ID, nil
	}

	newID := "idt_" + uuid.NewString()
	if id.CreatedAt.IsZero() {
		id.CreatedAt = now
	}
	if id.UpdatedAt.IsZero() {
		id.UpdatedAt = now
	}
	id.ID = newID
	id.CredentialHeaders = cloneHeaders(id.CredentialHeaders)
	if id.ClaimsSummary != nil {
		id.ClaimsSummary = cloneClaims(id.ClaimsSummary)
	}
	cp := id
	conv[newID] = &cp

	if id.IsDefault {
		s.clearDefaultLocked(conv, id.Scheme, newID)
	}

	return newID, nil
}

func (s *MemoryStore) clearDefaultLocked(conv map[string]*Identity, scheme, keepID string) {
	for _, item := range conv {
		if item.ID != keepID && item.Scheme == scheme {
			item.IsDefault = false
		}
	}
}

func cloneHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}

func cloneClaims(c map[string]any) map[string]any {
	if c == nil {
		return nil
	}
	out := make(map[string]any, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}

// List returns all identities for a conversation.
func (s *MemoryStore) List(conversationID string) []Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		return nil
	}
	out := make([]Identity, 0, len(conv))
	for _, item := range conv {
		out = append(out, cloneIdentity(*item))
	}
	return out
}

// Get returns a single identity by id within a conversation.
func (s *MemoryStore) Get(conversationID, id string) (Identity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		return Identity{}, fmt.Errorf("identity not found")
	}
	item, ok := conv[id]
	if !ok {
		return Identity{}, fmt.Errorf("identity not found")
	}
	return cloneIdentity(*item), nil
}

func cloneIdentity(id Identity) Identity {
	cp := id
	cp.CredentialHeaders = cloneHeaders(id.CredentialHeaders)
	cp.ClaimsSummary = cloneClaims(id.ClaimsSummary)
	return cp
}

// ListPublic returns sanitized public views without credential headers.
func (s *MemoryStore) ListPublic(conversationID string) []PublicView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		return nil
	}
	out := make([]PublicView, 0, len(conv))
	for _, item := range conv {
		out = append(out, toPublicView(*item))
	}
	return out
}

func toPublicView(id Identity) PublicView {
	v := PublicView{
		ID:        id.ID,
		Label:     id.Label,
		Scheme:    id.Scheme,
		Source:    id.Source,
		IsDefault: id.IsDefault,
	}
	if id.ClaimsSummary != nil {
		v.ClaimsSummary = cloneClaims(id.ClaimsSummary)
	}
	if !id.LastUsedAt.IsZero() {
		t := id.LastUsedAt
		v.LastUsedAt = &t
	}
	return v
}

// Delete removes an identity from a conversation.
func (s *MemoryStore) Delete(conversationID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		return fmt.Errorf("identity not found")
	}
	if _, ok := conv[id]; !ok {
		return fmt.Errorf("identity not found")
	}
	delete(conv, id)
	if len(conv) == 0 {
		delete(s.byConv, conversationID)
	}
	return nil
}

// SetDefault marks an identity as the default for its scheme within a conversation.
func (s *MemoryStore) SetDefault(conversationID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		return fmt.Errorf("identity not found")
	}
	item, ok := conv[id]
	if !ok {
		return fmt.Errorf("identity not found")
	}
	s.clearDefaultLocked(conv, item.Scheme, id)
	item.IsDefault = true
	return nil
}

// ClearCaptured removes login_capture and manual identities for a conversation.
func (s *MemoryStore) ClearCaptured(conversationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		return
	}
	for id, item := range conv {
		if item.Source == SourceLoginCapture || item.Source == SourceManual {
			delete(conv, id)
		}
	}
	if len(conv) == 0 {
		delete(s.byConv, conversationID)
	}
}

// Touch updates LastUsedAt for an identity.
func (s *MemoryStore) Touch(conversationID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	conv, ok := s.byConv[conversationID]
	if !ok {
		return fmt.Errorf("identity not found")
	}
	item, ok := conv[id]
	if !ok {
		return fmt.Errorf("identity not found")
	}
	item.LastUsedAt = time.Now().UTC()
	return nil
}
