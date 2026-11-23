package skill

import (
	"fmt"
	"sync"
	"time"
)

// Usage tracks activate_skill counts for curator archival decisions.
type Usage struct {
	SkillID         string
	ActivateCount   int
	LastActivatedAt time.Time
	InstalledAt     time.Time
}

// UsageStore persists per-skill usage. Implementations must be safe for concurrent use.
type UsageStore interface {
	RecordActivate(skillID string, at time.Time) error
	EnsureInstalled(skillID string, at time.Time) error
	Get(skillID string) (Usage, bool)
	List() []Usage
}

// MemoryUsageStore is an in-memory UsageStore.
type MemoryUsageStore struct {
	mu   sync.Mutex
	byID map[string]Usage
}

func NewMemoryUsageStore() *MemoryUsageStore {
	return &MemoryUsageStore{byID: map[string]Usage{}}
}

func (s *MemoryUsageStore) RecordActivate(skillID string, at time.Time) error {
	if skillID == "" {
		return fmt.Errorf("skill id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = map[string]Usage{}
	}
	u := s.byID[skillID]
	u.SkillID = skillID
	if u.InstalledAt.IsZero() {
		u.InstalledAt = at
	}
	u.ActivateCount++
	u.LastActivatedAt = at
	s.byID[skillID] = u
	return nil
}

func (s *MemoryUsageStore) EnsureInstalled(skillID string, at time.Time) error {
	if skillID == "" {
		return fmt.Errorf("skill id required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byID == nil {
		s.byID = map[string]Usage{}
	}
	if _, ok := s.byID[skillID]; ok {
		return nil
	}
	s.byID[skillID] = Usage{SkillID: skillID, InstalledAt: at}
	return nil
}

func (s *MemoryUsageStore) Get(skillID string) (Usage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[skillID]
	return u, ok
}

func (s *MemoryUsageStore) List() []Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Usage, 0, len(s.byID))
	for _, u := range s.byID {
		out = append(out, u)
	}
	return out
}
