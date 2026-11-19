package store

import (
	"path/filepath"
	"testing"
)

func runOriginCase(t *testing.T, st Store) {
	t.Helper()
	scheduled, err := st.CreateRun(CreateRunInput{
		AgentID: "a", Input: "scheduled", ConversationID: "c1",
		Origin: RunOriginScheduled,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.GetRun(scheduled.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Origin != RunOriginScheduled {
		t.Fatalf("origin=%q want %q", got.Origin, RunOriginScheduled)
	}

	legacy, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "legacy", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	gotLegacy, err := st.GetRun(legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotLegacy.Origin != "" {
		t.Fatalf("empty origin must stay empty, got %q", gotLegacy.Origin)
	}
}

func TestRunOriginMemory(t *testing.T) {
	runOriginCase(t, NewMemory())
}

func TestRunOriginSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run_origin.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runOriginCase(t, st)
}
