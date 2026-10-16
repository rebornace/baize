package schedule_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/schedule"
	_ "modernc.org/sqlite"
)

func openSQLiteSchedule(t *testing.T) schedule.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sched.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() { _ = db.Close() })
	s, err := schedule.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func openSQLiteScheduleConcurrent(t *testing.T) schedule.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sched-concurrent.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = db.Close() })
	s, err := schedule.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSQLiteResumeFailsAtActiveCap(t *testing.T) {
	s := openSQLiteSchedule(t)
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
	pausedID := paused.ID
	if err := s.Resume("alice", pausedID, next); !errors.Is(err, schedule.ErrActiveCap) {
		t.Fatalf("resume at cap: %v", err)
	}
	list, _ := s.List("alice")
	active := 0
	for _, row := range list {
		if row.ID == pausedID && row.Status != schedule.StatusPaused {
			t.Fatalf("paused row changed: %+v", row)
		}
		if row.Status == schedule.StatusActive {
			active++
		}
	}
	if active != 20 {
		t.Fatalf("active count=%d", active)
	}
}

func TestSQLiteConcurrentInsertActiveCap(t *testing.T) {
	s := openSQLiteScheduleConcurrent(t)
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 19; i++ {
		if _, err := s.Insert(schedule.Item{
			OwnerID: "alice", ConversationID: "c1", Instruction: "fill",
			Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	var okCount atomic.Int32
	var capCount atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Insert(schedule.Item{
				OwnerID: "alice", ConversationID: "c1", Instruction: "race",
				Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
			})
			if err == nil {
				okCount.Add(1)
			} else if errors.Is(err, schedule.ErrActiveCap) {
				capCount.Add(1)
			}
		}()
	}
	wg.Wait()
	list, err := s.List("alice")
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, row := range list {
		if row.Status == schedule.StatusActive {
			active++
		}
	}
	if active != 20 {
		t.Fatalf("active=%d okInserts=%d capErr=%d", active, okCount.Load(), capCount.Load())
	}
	if okCount.Load() != 1 {
		t.Fatalf("expected 1 concurrent insert ok, got %d", okCount.Load())
	}
}

func TestSQLiteClaimConcurrentOneRow(t *testing.T) {
	s := openSQLiteScheduleConcurrent(t)
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	if _, err := s.Insert(schedule.Item{
		OwnerID: "alice", ConversationID: "c1", Instruction: "race",
		Kind: "daily", Status: schedule.StatusActive, NextRunAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
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
