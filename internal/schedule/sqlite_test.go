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

func TestSQLiteMigratesClaimExpiresColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sched-old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE conversation_schedules (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  instruction TEXT NOT NULL,
  kind TEXT NOT NULL,
  timezone TEXT NOT NULL DEFAULT '',
  fire_at TEXT NOT NULL DEFAULT '',
  time_of_day TEXT NOT NULL DEFAULT '',
  weekday INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  next_run_at TEXT NOT NULL,
  running_run_id TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 0
);
INSERT INTO conversation_schedules (
  id, owner_id, conversation_id, instruction, kind, timezone, fire_at, time_of_day, weekday,
  status, next_run_at, running_run_id, last_error, revision
) VALUES ('sched_old', 'alice', 'c1', 'old row', 'once', 'UTC', '', '', 0,
  'active', '2026-10-16T12:00:00Z', '', '', 0);
`); err != nil {
		t.Fatal(err)
	}
	s, err := schedule.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	it, ok, err := s.ClaimDue(now)
	if err != nil || !ok {
		t.Fatalf("claim after migration: ok=%v err=%v", ok, err)
	}
	if it.ID != "sched_old" || it.ClaimExpiresAt.IsZero() {
		t.Fatalf("claimed=%+v", it)
	}
	if got := rowByID(t, s, "alice", "sched_old"); got.RunningRunID != schedule.ClaimPending ||
		!got.ClaimExpiresAt.Equal(now.Add(schedule.ClaimLease)) {
		t.Fatalf("row=%+v", got)
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
