package schedule

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// ClaimPending is written to running_run_id while a due row is being opened.
	ClaimPending = "pending"
	// MaxActivePerOwner is the cap on active schedules per owner_id.
	MaxActivePerOwner = 20
)

// ErrActiveCap is returned when Insert or Resume would exceed MaxActivePerOwner active rows.
var ErrActiveCap = errors.New("schedule: active schedule cap reached")

const (
	StatusActive    = "active"
	StatusPaused    = "paused"
	StatusCompleted = "completed"
)

// Item is a persisted conversation schedule row.
type Item struct {
	ID             string
	OwnerID        string
	ConversationID string
	Instruction    string
	Kind           string
	Timezone       string
	FireAt         time.Time
	TimeOfDay      string
	Weekday        int
	Status         string
	NextRunAt      time.Time
	RunningRunID   string
	LastError      string
	Revision       int
}

// Store persists schedules and atomically claims due rows.
type Store interface {
	Insert(it Item) (Item, error)
	ClaimDue(now time.Time) (Item, bool, error)
	List(ownerID string) ([]Item, error)
	Pause(ownerID, id string) error
	Resume(ownerID, id string, next time.Time) error
	Delete(ownerID, id string) error
	ClearStalePending() error
	ListWithRunningRun() ([]Item, error)
	ReleaseRunningRun(id string) error
	FailOpen(id, lastError string) error
	CompleteOpen(id, runID, status string, nextRunAt time.Time) error
	AdvanceNextRunAt(id string, next time.Time) error
	SetRunningRunID(id, runID string) error
}

// MemoryStore is an in-process Store for tests.
type MemoryStore struct {
	mu         sync.Mutex
	scanSerial sync.Mutex
	byID       map[string]Item
}

func (s *MemoryStore) scanLock()   { s.scanSerial.Lock() }
func (s *MemoryStore) scanUnlock() { s.scanSerial.Unlock() }

// NewMemoryStore creates an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byID: map[string]Item{}}
}

var _ Store = (*MemoryStore)(nil)

func (s *MemoryStore) Insert(it Item) (Item, error) {
	if it.OwnerID == "" {
		return Item{}, fmt.Errorf("schedule: owner_id required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status := it.Status
	if status == "" {
		status = StatusActive
	}
	if status == StatusActive {
		if countActiveLocked(s.byID, it.OwnerID) >= MaxActivePerOwner {
			return Item{}, ErrActiveCap
		}
	}
	out := it
	out.ID = "sched_" + uuid.NewString()
	out.Status = status
	out.Revision = 0
	s.byID[out.ID] = out
	return out, nil
}

func (s *MemoryStore) ClaimDue(now time.Time) (Item, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var candidates []Item
	for _, it := range s.byID {
		if it.Status != StatusActive || it.RunningRunID != "" {
			continue
		}
		if it.NextRunAt.After(now) {
			continue
		}
		candidates = append(candidates, it)
	}
	if len(candidates) == 0 {
		return Item{}, false, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].NextRunAt.Equal(candidates[j].NextRunAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].NextRunAt.Before(candidates[j].NextRunAt)
	})
	pick := candidates[0]
	if pick.RunningRunID != "" {
		return Item{}, false, nil
	}
	pick.RunningRunID = ClaimPending
	pick.Revision++
	s.byID[pick.ID] = pick
	return pick, true, nil
}

func (s *MemoryStore) List(ownerID string) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Item
	for _, it := range s.byID {
		if it.OwnerID == ownerID {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemoryStore) Pause(ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	if it.OwnerID != ownerID {
		return fmt.Errorf("schedule: owner mismatch")
	}
	if it.Status != StatusActive {
		return fmt.Errorf("schedule: not active")
	}
	it.Status = StatusPaused
	it.Revision++
	s.byID[id] = it
	return nil
}

func (s *MemoryStore) Resume(ownerID, id string, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	if it.OwnerID != ownerID {
		return fmt.Errorf("schedule: owner mismatch")
	}
	if it.Status != StatusPaused {
		return fmt.Errorf("schedule: not paused")
	}
	if countActiveLocked(s.byID, ownerID) >= MaxActivePerOwner {
		return ErrActiveCap
	}
	it.Status = StatusActive
	it.NextRunAt = next.UTC()
	it.Revision++
	s.byID[id] = it
	return nil
}

func (s *MemoryStore) ClearStalePending() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, it := range s.byID {
		if it.RunningRunID == ClaimPending {
			it.RunningRunID = ""
			it.Revision++
			s.byID[id] = it
		}
	}
	return nil
}

func (s *MemoryStore) ListWithRunningRun() ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Item
	for _, it := range s.byID {
		if it.RunningRunID != "" && it.RunningRunID != ClaimPending {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *MemoryStore) ReleaseRunningRun(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	it.RunningRunID = ""
	it.Revision++
	s.byID[id] = it
	return nil
}

func (s *MemoryStore) FailOpen(id, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	it.RunningRunID = ""
	it.LastError = lastError
	it.Revision++
	s.byID[id] = it
	return nil
}

func (s *MemoryStore) CompleteOpen(id, runID, status string, nextRunAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	it.RunningRunID = runID
	it.Status = status
	it.NextRunAt = nextRunAt.UTC()
	it.LastError = ""
	it.Revision++
	s.byID[id] = it
	return nil
}

func (s *MemoryStore) SetRunningRunID(id, runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	it.RunningRunID = runID
	it.Revision++
	s.byID[id] = it
	return nil
}

func (s *MemoryStore) AdvanceNextRunAt(id string, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	it.NextRunAt = next.UTC()
	it.Revision++
	s.byID[id] = it
	return nil
}

func (s *MemoryStore) Delete(ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	if !ok {
		return fmt.Errorf("schedule: not found")
	}
	if it.OwnerID != ownerID {
		return fmt.Errorf("schedule: owner mismatch")
	}
	delete(s.byID, id)
	return nil
}

func countActiveLocked(byID map[string]Item, ownerID string) int {
	n := 0
	for _, it := range byID {
		if it.OwnerID == ownerID && it.Status == StatusActive {
			n++
		}
	}
	return n
}

func formatScheduleTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseScheduleTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339, s)
}
