package schedule_test

import (
	"database/sql"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rebornace/baize/internal/schedule"
)

func openPostgresSchedule(t *testing.T) schedule.Store {
	t.Helper()
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s, err := schedule.OpenPostgres(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func deleteScheduleOwnerRows(t *testing.T, s schedule.Store, owner string) {
	t.Helper()
	items, err := s.List(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if err := s.Delete(owner, it.ID); err != nil {
			t.Fatalf("delete %s: %v", it.ID, err)
		}
	}
}

func TestPostgresClaimConcurrentOneRow(t *testing.T) {
	s := openPostgresSchedule(t)
	deleteScheduleOwnerRows(t, s, "alice")
	defer deleteScheduleOwnerRows(t, s, "alice")
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

func TestPostgresRenewClaimKeepsLease(t *testing.T) {
	s := openPostgresSchedule(t)
	deleteScheduleOwnerRows(t, s, "alice")
	deleteScheduleOwnerRows(t, s, "renew-owner")
	defer deleteScheduleOwnerRows(t, s, "renew-owner")
	now := time.Now().UTC()
	it, err := s.Insert(schedule.Item{
		OwnerID: "renew-owner", ConversationID: "renew-conv", Instruction: "renew",
		Kind: "once", Timezone: "UTC", FireAt: now.Add(-time.Second), NextRunAt: now.Add(-time.Second),
		Status: schedule.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := s.ClaimDue(now)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if claimed.ID != it.ID {
		// Another leftover row was claimed first; release it and claim again.
		_ = s.ReleaseRunningRun(claimed.ID)
		claimed, ok, err = s.ClaimDue(now)
		if err != nil || !ok || claimed.ID != it.ID {
			t.Fatalf("claim own: %+v ok=%v err=%v", claimed, ok, err)
		}
	}
	if ok, err := s.RenewClaim(it.ID, now, 5*time.Minute); err != nil || !ok {
		t.Fatalf("renew: ok=%v err=%v", ok, err)
	}
	if err := s.ClearStalePending(now.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	items, err := s.List("renew-owner")
	if err != nil {
		t.Fatal(err)
	}
	var got schedule.Item
	for _, row := range items {
		if row.ID == it.ID {
			got = row
		}
	}
	if got.RunningRunID != schedule.ClaimPending {
		t.Fatalf("claim released early: %+v", got)
	}
	if !got.ClaimExpiresAt.After(now.Add(3 * time.Minute)) {
		t.Fatalf("claim not renewed: %+v", got)
	}
	_ = s.ReleaseRunningRun(it.ID)
}
