package store

import (
	"path/filepath"
	"testing"
	"time"
)

func waitingRunsCase(t *testing.T, st Store) {
	t.Helper()
	a, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "first", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(a.ID, StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)

	b, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "second", ConversationID: "c2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(b.ID, StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "running", ConversationID: "c3"}); err != nil {
		t.Fatal(err)
	}

	got, err := st.ListWaitingHumanRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 waiting runs, got %d", len(got))
	}
	if got[0].ID != a.ID || got[1].ID != b.ID {
		t.Fatalf("want created_at ascending [%s %s], got [%s %s]", a.ID, b.ID, got[0].ID, got[1].ID)
	}

	limited, err := st.ListWaitingHumanRuns(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].ID != a.ID {
		t.Fatalf("limit 1 want oldest %s, got %+v", a.ID, limited)
	}
}

func TestListWaitingHumanRunsMemory(t *testing.T) {
	waitingRunsCase(t, NewMemory())
}

func TestListWaitingHumanRunsSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "waiting_runs.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	waitingRunsCase(t, st)
}

func TestHITLPayloadWaitingSinceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hitl_waiting.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	r, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "x", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	if err := st.SetHITL(r.ID, &HITLPayload{
		Prompt:       "approve",
		ToolName:     "write",
		Kind:         HITLKindApproval,
		WaitingSince: want,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetHITL(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.WaitingSince.Equal(want) {
		t.Fatalf("waiting_since=%v want %v", got, want)
	}
}
