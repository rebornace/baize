package schedule_test

import (
	"database/sql"
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
