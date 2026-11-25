package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func runRetentionCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	recent := now.AddDate(0, 0, -1)

	oldRun, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "old", CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(oldRun.ID, StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(oldRun.ID, Event{Type: "llm.message", Data: map[string]any{"content": "old"}})
	_ = st.PutRunCost(RunCost{RunID: oldRun.ID, Day: old.Format("2006-01-02"), CostUSD: 1})

	recentRun, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "recent", CreatedAt: recent})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(recentRun.ID, StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	waiting, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "waiting", CreatedAt: old})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(waiting.ID, StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}

	n, err := st.DeleteTerminalRunsBefore(now.AddDate(0, 0, -30), 10)
	if err != nil || n < 1 {
		t.Fatalf("deleted=%d err=%v", n, err)
	}
	if _, err := st.GetRun(oldRun.ID); err == nil {
		t.Fatal("old terminal run still present")
	}
	if evs, _ := st.ListEvents(oldRun.ID); len(evs) != 0 {
		t.Fatalf("old events left: %+v", evs)
	}
	if _, ok, _ := st.GetRunCost(oldRun.ID); ok {
		t.Fatal("old run cost still present")
	}
	if _, err := st.GetRun(recentRun.ID); err != nil {
		t.Fatalf("recent run removed: %v", err)
	}
	if got, err := st.GetRun(waiting.ID); err != nil || got.Status != StatusWaitingHuman {
		t.Fatalf("waiting run damaged: %+v err=%v", got, err)
	}
}

func TestRunRetentionMemory(t *testing.T) { runRetentionCase(t, NewMemory()) }

func TestRunRetentionSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "run_retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runRetentionCase(t, st)
}

func TestRunRetentionPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runRetentionCase(t, st)
}

func runRetentionScopedCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	suffix := now.Format("150405.000000000")
	convA, convB, convC := "rt-a-"+suffix, "rt-b-"+suffix, "rt-c-"+suffix
	mk := func(conv string) *Run {
		r, err := st.CreateRun(CreateRunInput{
			AgentID: "a", Input: conv, ConversationID: conv, CreatedAt: old,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		return r
	}
	a := mk(convA)
	b := mk(convB)
	c := mk(convC)

	convIDs, err := st.ListRunConversationIDs()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, id := range convIDs {
		seen[id] = true
	}
	for _, want := range []string{convA, convB, convC} {
		if !seen[want] {
			t.Fatalf("conversation %s missing from %v", want, convIDs)
		}
	}

	if n, err := st.DeleteTerminalRunsBeforeForConversation(convA, now.AddDate(0, 0, -30), 10); err != nil || n != 1 {
		t.Fatalf("scoped n=%d err=%v", n, err)
	}
	if _, err := st.GetRun(a.ID); err == nil {
		t.Fatal("scoped delete left c-a")
	}
	if _, err := st.GetRun(b.ID); err != nil {
		t.Fatalf("scoped delete removed c-b: %v", err)
	}

	if n, err := st.DeleteTerminalRunsBeforeExcluding(now.AddDate(0, 0, -30), 10, []string{convB}); err != nil || n == 0 {
		t.Fatalf("excluding n=%d err=%v", n, err)
	}
	if _, err := st.GetRun(b.ID); err != nil {
		t.Fatalf("excluded conversation removed: %v", err)
	}
	if _, err := st.GetRun(c.ID); err == nil {
		t.Fatal("excluding delete left c-c")
	}
}

func TestRunRetentionScopedMemory(t *testing.T) { runRetentionScopedCase(t, NewMemory()) }

func TestRunRetentionScopedSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "run_retention_scoped.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runRetentionScopedCase(t, st)
}

func TestRunRetentionScopedPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runRetentionScopedCase(t, st)
}
