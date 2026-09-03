package store_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

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

func mustCreateRun(t *testing.T, st store.Store) *store.Run {
	t.Helper()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// 让 created_at 落在调和宽限期之外：无租约的新 run 默认为不应被立即调和。
	return r
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
