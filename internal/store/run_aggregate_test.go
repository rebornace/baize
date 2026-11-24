package store

import (
	"path/filepath"
	"testing"
	"time"
)

func runAggregateCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC()
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		r, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "x"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	if err := st.UpdateRun(ids[0], StatusSucceeded, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(ids[1], StatusFailed, "", "boom"); err != nil {
		t.Fatal(err)
	}
	start := now.Add(-time.Minute)
	end := now.Add(time.Minute)
	page1, err := st.ListRunsCreatedBetweenPage(start, end, "", 2, 0)
	if err != nil || len(page1) != 2 {
		t.Fatalf("page1=%d err=%v", len(page1), err)
	}
	page2, err := st.ListRunsCreatedBetweenPage(start, end, "", 2, 2)
	if err != nil || len(page2) != 1 {
		t.Fatalf("page2=%d err=%v", len(page2), err)
	}
	if page1[0].ID == page2[0].ID {
		t.Fatal("pages overlap")
	}
	counts, err := st.CountRunsByStatusBetween(start, end)
	if err != nil {
		t.Fatal(err)
	}
	if counts[StatusSucceeded] != 1 || counts[StatusFailed] != 1 || counts[StatusRunning] != 1 {
		t.Fatalf("counts=%+v", counts)
	}
}

func TestRunAggregateMemory(t *testing.T) { runAggregateCase(t, NewMemory()) }

func TestRunAggregateSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "run_aggregate.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runAggregateCase(t, st)
}
