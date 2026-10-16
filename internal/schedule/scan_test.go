package schedule_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/schedule"
)

type openCall struct {
	ConversationID string
	Instruction    string
	OwnerID        string
}

type fakeOpener struct {
	mu    sync.Mutex
	calls []openCall
	err   error
	runID string
}

func (f *fakeOpener) Open(conversationID, instruction, ownerID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, openCall{conversationID, instruction, ownerID})
	if f.err != nil {
		return "", f.err
	}
	rid := f.runID
	if rid == "" {
		rid = "run_1"
	}
	return rid, nil
}

func (f *fakeOpener) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

type fakeMeta struct {
	owners map[string]string
	err    error
}

func (m *fakeMeta) GetMeta(conversationID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if o, ok := m.owners[conversationID]; ok {
		return o, nil
	}
	return "alice", nil
}

func nonTerminalRunStatus(string) (bool, error) {
	return false, nil
}

func insertDaily(t *testing.T, s schedule.Store, owner, conv, instr string, next time.Time) schedule.Item {
	t.Helper()
	it, err := s.Insert(schedule.Item{
		OwnerID: owner, ConversationID: conv, Instruction: instr,
		Kind: "daily", Timezone: "UTC", TimeOfDay: "09:00",
		Status: schedule.StatusActive, NextRunAt: next,
	})
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func rowByID(t *testing.T, s schedule.Store, owner, id string) schedule.Item {
	t.Helper()
	list, err := s.List(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range list {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("row %q not found", id)
	return schedule.Item{}
}

func TestScanFiresOnceThenCompletes(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	fire := now.Add(-time.Hour)
	it, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "do once",
		Kind: "once", Timezone: "UTC", FireAt: fire, NextRunAt: fire,
		Status: schedule.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := schedule.Scan(s, meta, opener, nonTerminalRunStatus, now); err != nil {
		t.Fatal(err)
	}
	if opener.callCount() != 1 {
		t.Fatalf("Open calls=%d", opener.callCount())
	}
	c := opener.calls[0]
	if c.ConversationID != "c1" || c.Instruction != "do once" || c.OwnerID != "alice" {
		t.Fatalf("Open args: %+v", c)
	}
	got := rowByID(t, s, "alice", it.ID)
	if got.Status != schedule.StatusCompleted || got.RunningRunID != "run_1" {
		t.Fatalf("after scan: %+v", got)
	}
}

func TestScanMissedPeriodsFireOnce(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	threeDaysAgo := now.AddDate(0, 0, -3)
	it := insertDaily(t, s, "alice", "c1", "daily task", threeDaysAgo)
	if err := schedule.Scan(s, meta, opener, nonTerminalRunStatus, now); err != nil {
		t.Fatal(err)
	}
	if opener.callCount() != 1 {
		t.Fatalf("Open calls=%d", opener.callCount())
	}
	got := rowByID(t, s, "alice", it.ID)
	if !got.NextRunAt.After(now) {
		t.Fatalf("next_run_at=%v want after %v", got.NextRunAt, now)
	}
	if got.RunningRunID != "run_1" {
		t.Fatalf("running_run_id=%q", got.RunningRunID)
	}
}

func TestScanSkipsInFlight(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	due := now.Add(-time.Hour)
	it := insertDaily(t, s, "alice", "c1", "in flight", due)
	if err := s.CompleteOpen(it.ID, "run_in_flight", schedule.StatusActive, due); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Scan(s, meta, opener, nonTerminalRunStatus, now); err != nil {
		t.Fatal(err)
	}
	if opener.callCount() != 0 {
		t.Fatalf("Open should not run, calls=%d", opener.callCount())
	}
	got := rowByID(t, s, "alice", it.ID)
	if !got.NextRunAt.After(now) {
		t.Fatalf("next_run_at=%v want after now", got.NextRunAt)
	}
	if got.RunningRunID != "run_in_flight" {
		t.Fatalf("running_run_id=%q", got.RunningRunID)
	}
}

func TestScanPausedDoesNotFire(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	if _, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "paused",
		Kind: "daily", Timezone: "UTC", TimeOfDay: "09:00",
		Status: schedule.StatusPaused, NextRunAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := schedule.Scan(s, meta, opener, nonTerminalRunStatus, now); err != nil {
		t.Fatal(err)
	}
	if opener.callCount() != 0 {
		t.Fatalf("Open calls=%d", opener.callCount())
	}
}

func TestScanOwnerMismatchDoesNotComplete(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "bob"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	it := insertDaily(t, s, "alice", "c1", "secret instruction xyz", now.Add(-time.Minute))
	if err := schedule.Scan(s, meta, opener, nonTerminalRunStatus, now); err != nil {
		t.Fatal(err)
	}
	if opener.callCount() != 0 {
		t.Fatalf("Open calls=%d", opener.callCount())
	}
	got := rowByID(t, s, "alice", it.ID)
	if got.Status != schedule.StatusActive || got.RunningRunID != "" {
		t.Fatalf("row: %+v", got)
	}
	if got.LastError == "" {
		t.Fatal("expected last_error")
	}
	if contains(got.LastError, "secret instruction") {
		t.Fatalf("last_error must not contain instruction: %q", got.LastError)
	}
}

func TestScanOpenFailureKeepsDue(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{err: errors.New("boom")}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	due := now.Add(-time.Minute)
	it := insertDaily(t, s, "alice", "c1", "retry me please", due)
	if err := schedule.Scan(s, meta, opener, nonTerminalRunStatus, now); err != nil {
		t.Fatal(err)
	}
	got := rowByID(t, s, "alice", it.ID)
	if got.Status != schedule.StatusActive || got.RunningRunID != "" {
		t.Fatalf("row: %+v", got)
	}
	if !got.NextRunAt.Equal(due) {
		t.Fatalf("next_run_at changed: %v", got.NextRunAt)
	}
	if got.LastError == "" || contains(got.LastError, "retry me") {
		t.Fatalf("last_error=%q", got.LastError)
	}
}

func TestScanClearsStalePending(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	it := insertDaily(t, s, "alice", "c1", "stale pending", now.Add(-time.Hour))
	if _, ok, err := s.ClaimDue(now); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	got := rowByID(t, s, "alice", it.ID)
	if got.RunningRunID != schedule.ClaimPending {
		t.Fatalf("pending=%q", got.RunningRunID)
	}
	if err := schedule.Scan(s, meta, opener, nonTerminalRunStatus, now); err != nil {
		t.Fatal(err)
	}
	if opener.callCount() != 1 {
		t.Fatalf("Open calls=%d", opener.callCount())
	}
}

func TestScanClearsTerminalRunningRun(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	it := insertDaily(t, s, "alice", "c1", "done run", now.Add(24*time.Hour))
	if err := s.CompleteOpen(it.ID, "run_old", schedule.StatusActive, it.NextRunAt); err != nil {
		t.Fatal(err)
	}
	terminal := func(runID string) (bool, error) {
		return runID == "run_old", nil
	}
	if err := schedule.Scan(s, meta, opener, terminal, now); err != nil {
		t.Fatal(err)
	}
	got := rowByID(t, s, "alice", it.ID)
	if got.RunningRunID != "" {
		t.Fatalf("running_run_id=%q want empty", got.RunningRunID)
	}
}

func TestScanConcurrentClaimOpenOnce(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	insertDaily(t, s, "alice", "c1", "race", now.Add(-time.Hour))
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok, err := s.ClaimDue(now)
			if err != nil || !ok {
				return
			}
			_ = schedule.Scan(s, meta, opener, nonTerminalRunStatus, now)
		}()
	}
	wg.Wait()
	if opener.callCount() != 1 {
		t.Fatalf("Open calls=%d want 1", opener.callCount())
	}
}

func TestScanConcurrentFullOpenOnce(t *testing.T) {
	s := schedule.NewMemoryStore()
	meta := &fakeMeta{owners: map[string]string{"c1": "alice"}}
	opener := &fakeOpener{}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	insertDaily(t, s, "alice", "c1", "race2", now.Add(-time.Hour))
	var wg sync.WaitGroup
	var scans atomic.Int32
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scans.Add(1)
			_ = schedule.Scan(s, meta, opener, nonTerminalRunStatus, now)
		}()
	}
	wg.Wait()
	if opener.callCount() != 1 {
		t.Fatalf("Open calls=%d want 1", opener.callCount())
	}
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && searchSub(s, sub)
}

func searchSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
