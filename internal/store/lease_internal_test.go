package store

import (
	"testing"
	"time"
)

// TestReconcileGraceWindowMemory covers the never-leased grace branches for the
// in-memory store: a fresh run is protected by the grace window; a never-leased
// run older than the window (and an expired-lease run) are listed.
func TestReconcileGraceWindowMemory(t *testing.T) {
	m := NewMemory()

	fresh, err := m.CreateRun(CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}

	stale, err := m.CreateRun(CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.runs[stale.ID].CreatedAt = time.Now().UTC().Add(-reconcileGrace - time.Minute)
	m.mu.Unlock()

	expired, err := m.CreateRun(CreateRunInput{AgentID: "a", Input: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.LeaseRun(expired.ID, -time.Hour); err != nil {
		t.Fatal(err)
	}

	got, err := m.ListRunsForReconcile(50)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r.ID)
	}
	if containsID(ids, fresh.ID) {
		t.Fatalf("fresh never-leased run (within grace) must not be listed, got %v", ids)
	}
	if !containsID(ids, stale.ID) {
		t.Fatalf("stale never-leased run (older than grace) must be listed, got %v", ids)
	}
	if !containsID(ids, expired.ID) {
		t.Fatalf("expired-lease run must be listed, got %v", ids)
	}
}

func containsID(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
