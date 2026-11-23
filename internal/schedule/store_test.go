package schedule_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/schedule"
)

func TestActiveCapAndClaim(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		if _, err := s.Insert(schedule.Item{
			OwnerID: "alice", ConversationID: "c1", Instruction: "喝水",
			Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "第二十一条",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	}); err == nil {
		t.Fatal("21st active must fail")
	} else if !errors.Is(err, schedule.ErrActiveCap) {
		t.Fatalf("want ErrActiveCap, got %v", err)
	}
	got, ok, err := s.ClaimDue(now)
	if err != nil || !ok || got.RunningRunID != schedule.ClaimPending {
		t.Fatalf("claim %+v ok %v err %v", got, ok, err)
	}
	got2, ok2, err := s.ClaimDue(now)
	if err != nil || !ok2 || got2.RunningRunID != schedule.ClaimPending {
		t.Fatalf("second claim other row: %+v ok %v err %v", got2, ok2, err)
	}
	if got2.ID == got.ID {
		t.Fatal("second claim must be a different row")
	}
}

func TestPausedNotCountedInActiveCap(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		if _, err := s.Insert(schedule.Item{
			OwnerID: "alice", ConversationID: "c1", Instruction: "x",
			Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "paused one",
		Kind: "daily", Status: schedule.StatusPaused, NextRunAt: now,
	}); err != nil {
		t.Fatalf("paused should not count toward cap: %v", err)
	}
}

func TestPausePreventsClaim(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	it, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "喝水",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Pause("alice", it.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ClaimDue(now); ok {
		t.Fatal("paused row must not be claimed")
	}
}

func TestResumeSetsNextAndActive(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	next := now.Add(2 * time.Hour)
	it, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "喝水",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Pause("alice", it.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume("alice", it.ID, next); err != nil {
		t.Fatal(err)
	}
	list, err := s.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].Status != schedule.StatusActive || !list[0].NextRunAt.Equal(next) {
		t.Fatalf("after resume: %+v", list[0])
	}
}

func TestDeleteRemovesFromList(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	it, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "喝水",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("alice", it.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range list {
		if row.ID == it.ID {
			t.Fatal("deleted row still listed")
		}
	}
}

func TestClaimSameRowOnlyOnce(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	if _, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "only",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.ClaimDue(now); !ok {
		t.Fatal("first claim")
	}
	if _, ok, _ := s.ClaimDue(now); ok {
		t.Fatal("same row must not be claimed twice")
	}
}

func TestMemoryConcurrentClaimOneRow(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	if _, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "race",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok, err := s.ClaimDue(now); err == nil && ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("expected exactly one claim, got %d", wins.Load())
	}
}

func TestResumeFailsAtActiveCap(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	for i := 0; i < 20; i++ {
		if _, err := s.Insert(schedule.Item{
			OwnerID: "alice", ConversationID: "c1", Instruction: "active",
			Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	paused, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "paused extra",
		Kind: "daily", Status: schedule.StatusPaused, NextRunAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Resume("alice", paused.ID, next); !errors.Is(err, schedule.ErrActiveCap) {
		t.Fatalf("resume at cap: %v", err)
	}
	pausedID := paused.ID
	list, err := s.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range list {
		if row.ID == pausedID && row.Status != schedule.StatusPaused {
			t.Fatalf("paused row changed: %+v", row)
		}
	}
	active := 0
	for _, row := range list {
		if row.Status == schedule.StatusActive {
			active++
		}
	}
	if active != 20 {
		t.Fatalf("active count=%d", active)
	}
}

func TestRevisionIncrementsOnUpdate(t *testing.T) {
	s := schedule.NewMemoryStore()
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	it, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "rev",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if it.Revision != 0 {
		t.Fatalf("insert revision=%d", it.Revision)
	}
	if err := s.Pause("alice", it.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List("alice")
	if list[0].Revision != 1 {
		t.Fatalf("after pause revision=%d", list[0].Revision)
	}
}
