package store_test

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// sqliteTestTimeLayout mirrors the store's fixed-width (9 fractional digits)
// RFC3339 layout used for TEXT/TIMESTAMP columns on SQLite.
const sqliteTestTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

func TestLeaseRunAtomic(t *testing.T) {
	st := newLeaseStore(t)
	run := mustCreateRun(t, st)

	acq1, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || !acq1 {
		t.Fatalf("first lease: acq=%v err=%v", acq1, err)
	}
	acq2, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || acq2 {
		t.Fatalf("second lease while held must fail: acq=%v err=%v", acq2, err)
	}
	if err := st.ClearRunLease(run.ID); err != nil {
		t.Fatal(err)
	}
	acq3, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || !acq3 {
		t.Fatalf("lease after clear must succeed: acq=%v err=%v", acq3, err)
	}
}

func TestLeaseRunExpired(t *testing.T) {
	st := newLeaseStore(t)
	run := mustCreateRun(t, st)
	if _, err := st.LeaseRun(run.ID, -time.Second); err != nil { // 立即过期
		t.Fatal(err)
	}
	acq, err := st.LeaseRun(run.ID, time.Minute)
	if err != nil || !acq {
		t.Fatalf("expired lease must be releasable: acq=%v err=%v", acq, err)
	}
}

func TestHeartbeatRunExtends(t *testing.T) {
	st := newLeaseStore(t)
	run := mustCreateRun(t, st)
	if _, err := st.LeaseRun(run.ID, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	ok, err := st.HeartbeatRun(run.ID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("heartbeat: ok=%v err=%v", ok, err)
	}
	if acq, _ := st.LeaseRun(run.ID, time.Minute); acq {
		t.Fatal("after heartbeat the lease must still be held")
	}
}

func TestLeaseRunTerminalRejected(t *testing.T) {
	st := newLeaseStore(t)
	run := mustCreateRun(t, st)
	if err := st.UpdateRun(run.ID, store.StatusSucceeded, "", ""); err != nil {
		t.Fatal(err)
	}
	if acq, err := st.LeaseRun(run.ID, time.Minute); err != nil || acq {
		t.Fatalf("terminal run must not be leasable: acq=%v err=%v", acq, err)
	}
}

func TestListRunsForReconcile(t *testing.T) {
	st := newLeaseStore(t)
	orphan := mustCreateRun(t, st)                                // running, 无约但刚建（grace 内）
	if _, err := st.LeaseRun(orphan.ID, -time.Hour); err != nil { // 租约过期
		t.Fatal(err)
	}
	held := mustCreateRun(t, st)
	if _, err := st.LeaseRun(held.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	done := mustCreateRun(t, st)
	if err := st.UpdateRun(done.ID, store.StatusSucceeded, "", ""); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListRunsForReconcile(50)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if !contains(ids, orphan.ID) {
		t.Fatalf("orphan (expired lease) must be listed, got %v", ids)
	}
	if contains(ids, held.ID) {
		t.Fatalf("held run must not be listed, got %v", ids)
	}
	if contains(ids, done.ID) {
		t.Fatalf("terminal run must not be listed, got %v", ids)
	}
}

// TestReconcileGraceWindowSQL covers the never-leased grace branches: a freshly
// created run is protected by the 30s grace window, while a never-leased run
// older than the window is listed for reconciliation.
func TestReconcileGraceWindowSQL(t *testing.T) {
	st := newLeaseStore(t)
	db := rawDB(t, st)

	fresh := mustCreateRun(t, st) // created_at = now, never leased → inside grace

	staleID := "run_stale_never_leased"
	if _, err := db.Exec(
		`INSERT INTO runs (id, agent_id, input, status, output, error, created_at, lease_until)
		 VALUES (?, 'a', 'x', 'running', '', '', ?, NULL)`,
		staleID, time.Now().Add(-time.Hour).UTC().Format(sqliteTestTimeLayout),
	); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListRunsForReconcile(50)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if !contains(ids, staleID) {
		t.Fatalf("stale never-leased run (older than grace) must be listed, got %v", ids)
	}
	if contains(ids, fresh.ID) {
		t.Fatalf("fresh never-leased run (within grace) must not be listed, got %v", ids)
	}
}

// TestLeaseComparisonFixedWidthSameSecond locks down the SQLite lexicographic
// comparison: lease_until is TEXT-compared byte-wise, so timestamps must use a
// fixed-width fractional layout. Variable-length RFC3339Nano trims trailing
// zeros and inverts order within one whole second (".25Z" < ".2Z"), which
// would treat a live lease as expired and let a second worker steal the run.
func TestLeaseComparisonFixedWidthSameSecond(t *testing.T) {
	st := newLeaseStore(t)
	db := rawDB(t, st)

	// Production must persist lease_until as fixed-width RFC3339 (9 digits).
	run := mustCreateRun(t, st)
	if _, err := st.LeaseRun(run.ID, time.Minute); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := db.QueryRow(
		`SELECT CAST(lease_until AS TEXT) FROM runs WHERE id = ?`, run.ID,
	).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !isFixedWidthTimestamp(stored) {
		t.Fatalf("lease_until must be fixed-width RFC3339 (9 fractional digits), got %q", stored)
	}

	// Two leases in the same whole second, differing only in fraction. At
	// now=.200 the later lease (.250) must still be held (not reclaimable);
	// at now=.250 the earlier lease (.200) must be expired (reclaimable).
	base := "2026-09-03T12:00:00"
	created := base + ".000000000Z"
	liveLease := base + ".250000000Z"
	earlyLease := base + ".200000000Z"
	liveID, earlyID := "run_live_fmt", "run_early_fmt"
	for _, c := range []struct{ id, lease string }{
		{liveID, liveLease},
		{earlyID, earlyLease},
	} {
		if _, err := db.Exec(
			`INSERT INTO runs (id, agent_id, input, status, output, error, created_at, lease_until)
			 VALUES (?, 'a', 'x', 'running', '', '', ?, ?)`,
			c.id, created, c.lease,
		); err != nil {
			t.Fatal(err)
		}
	}
	reclaimable := func(id, nowArg string) int {
		var n int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM runs
			 WHERE id = ? AND status IN ('queued','running')
			   AND (lease_until IS NULL OR lease_until < ?)`,
			id, nowArg,
		).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// now = .200: later lease .250 must NOT be stale (".250..." < ".200..." is false).
	if got := reclaimable(liveID, base+".200000000Z"); got != 0 {
		t.Fatalf("later lease .250 must stay held at now .200 (lexicographic order inverted), reclaimable=%d", got)
	}
	// now = .250: earlier lease .200 is expired and must be reclaimable.
	if got := reclaimable(earlyID, base+".250000000Z"); got != 1 {
		t.Fatalf("earlier lease .200 must be stale at now .250, reclaimable=%d", got)
	}
}

func newLeaseStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.OpenWithOptions("sqlite", store.OpenOptions{SQLitePath: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := st.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	})
	return st
}

// rawDB returns the underlying *sql.DB of a SQLStore for direct-row test setup.
func rawDB(t *testing.T, st store.Store) *sql.DB {
	t.Helper()
	b, ok := st.(interface{ DB() *sql.DB })
	if !ok {
		t.Fatalf("store %T does not expose DB()", st)
	}
	return b.DB()
}

func mustCreateRun(t *testing.T, st store.Store) *store.Run {
	t.Helper()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// created_at = now：无租约的新 run 落在调和宽限期内，不应被立即调和。
	return r
}

func isFixedWidthTimestamp(s string) bool {
	if !strings.HasSuffix(s, "Z") {
		return false
	}
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return false
	}
	frac := s[dot+1 : len(s)-1]
	if len(frac) != 9 {
		return false
	}
	for _, c := range frac {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
