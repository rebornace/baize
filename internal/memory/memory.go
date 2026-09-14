package memory

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-process Store implementation.
type MemoryStore struct {
	mu         sync.RWMutex
	byID       map[string]Entry
	ownerKeyID map[string]string
}

// NewMemoryStore creates an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:       map[string]Entry{},
		ownerKeyID: map[string]string{},
	}
}

var _ Store = (*MemoryStore)(nil)

func ownerKey(ownerID, key string) string {
	return ownerID + "\x00" + key
}

func (s *MemoryStore) Upsert(e Entry) (Entry, error) {
	if e.OwnerID == "" {
		return Entry{}, fmt.Errorf("owner_id required")
	}
	e.Text = truncateText(e.Text)
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	if e.Key != "" {
		if id, ok := s.ownerKeyID[ownerKey(e.OwnerID, e.Key)]; ok {
			prev := s.byID[id]
			prev.Key = e.Key
			prev.Text = e.Text
			if e.Source != "" {
				prev.Source = e.Source
			}
			prev.UpdatedAt = now
			s.byID[id] = prev
			return prev, nil
		}
	}

	if e.ID != "" {
		if prev, ok := s.byID[e.ID]; ok {
			if prev.OwnerID != e.OwnerID {
				return Entry{}, fmt.Errorf("owner mismatch")
			}
			oldKey := prev.Key
			prev.Text = e.Text
			prev.Key = e.Key
			if e.Source != "" {
				prev.Source = e.Source
			}
			prev.UpdatedAt = now
			s.byID[e.ID] = prev
			if oldKey != e.Key {
				if oldKey != "" {
					delete(s.ownerKeyID, ownerKey(e.OwnerID, oldKey))
				}
				if e.Key != "" {
					s.ownerKeyID[ownerKey(e.OwnerID, e.Key)] = e.ID
				}
			}
			return prev, nil
		}
	}

	out := e
	out.ID = "mem_" + uuid.NewString()
	if out.Source == "" {
		out.Source = SourceExplicit
	}
	out.CreatedAt = now
	out.UpdatedAt = now
	s.byID[out.ID] = out
	if out.Key != "" {
		s.ownerKeyID[ownerKey(out.OwnerID, out.Key)] = out.ID
	}
	return out, nil
}

func (s *MemoryStore) Get(id string) (Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.byID[id]
	if !ok {
		return Entry{}, fmt.Errorf("memory entry not found")
	}
	return e, nil
}

func (s *MemoryStore) Delete(ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("memory entry not found")
	}
	if e.OwnerID != ownerID {
		return fmt.Errorf("owner mismatch")
	}
	delete(s.byID, id)
	if e.Key != "" {
		delete(s.ownerKeyID, ownerKey(ownerID, e.Key))
	}
	return nil
}

func (s *MemoryStore) List(ownerID string, limit, offset int) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []Entry
	for _, e := range s.byID {
		if e.OwnerID == ownerID {
			all = append(all, e)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].UpdatedAt.Equal(all[j].UpdatedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].UpdatedAt.After(all[j].UpdatedAt)
	})
	if offset > len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (s *MemoryStore) Search(ownerID, query string, topK int) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type scored struct {
		e     Entry
		score int
	}
	var hits []scored
	for _, e := range s.byID {
		if e.OwnerID != ownerID {
			continue
		}
		if sc := Rank(query, e.Text); sc > 0 {
			hits = append(hits, scored{e: e, score: sc})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		if hits[i].e.UpdatedAt.Equal(hits[j].e.UpdatedAt) {
			return hits[i].e.ID < hits[j].e.ID
		}
		return hits[i].e.UpdatedAt.After(hits[j].e.UpdatedAt)
	})
	if topK <= 0 {
		topK = DefaultTopK
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	out := make([]Entry, len(hits))
	for i, h := range hits {
		out[i] = h.e
	}
	return out, nil
}

func (s *MemoryStore) Forget(ownerID, key, text string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key != "" {
		id, ok := s.ownerKeyID[ownerKey(ownerID, key)]
		if !ok {
			return 0, nil
		}
		e := s.byID[id]
		delete(s.byID, id)
		delete(s.ownerKeyID, ownerKey(ownerID, key))
		_ = e
		return 1, nil
	}
	var n int
	for id, e := range s.byID {
		if e.OwnerID != ownerID {
			continue
		}
		if e.Text == text {
			delete(s.byID, id)
			if e.Key != "" {
				delete(s.ownerKeyID, ownerKey(ownerID, e.Key))
			}
			n++
		}
	}
	return n, nil
}
