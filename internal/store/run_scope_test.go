package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func runScopeCase(t *testing.T, st Store) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	agent1 := "scope-a1-" + suffix
	agent2 := "scope-a2-" + suffix
	conv1 := "scope-c1-" + suffix
	conv2 := "scope-c2-" + suffix

	create := func(agentID, convID string) *Run {
		r, err := st.CreateRun(CreateRunInput{AgentID: agentID, Input: "x", ConversationID: convID})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := create(agent1, conv1)
	time.Sleep(5 * time.Millisecond)
	second := create(agent1, conv2)
	third := create(agent2, conv1)
	_ = third

	start := time.Now().UTC().Add(-time.Minute)
	end := time.Now().UTC().Add(time.Minute)
	byAgent, err := st.ListRunsCreatedBetweenForScope(start, end, agent1, "", 10)
	if err != nil || len(byAgent) != 2 {
		t.Fatalf("byAgent=%d err=%v", len(byAgent), err)
	}
	if byAgent[0].ID != second.ID || byAgent[1].ID != first.ID {
		t.Fatalf("agent order=%v", []string{byAgent[0].ID, byAgent[1].ID})
	}
	byConv, err := st.ListRunsCreatedBetweenForScope(start, end, "", conv1, 10)
	if err != nil || len(byConv) != 2 {
		t.Fatalf("byConv=%d err=%v", len(byConv), err)
	}
	limited, err := st.ListRunsCreatedBetweenForScope(start, end, agent1, "", 1)
	if err != nil || len(limited) != 1 || limited[0].ID != second.ID {
		t.Fatalf("limited=%+v err=%v", limited, err)
	}
	all, err := st.ListRunsCreatedBetweenPage(start, end, "", 10, 0)
	if err != nil || len(all) < 3 {
		t.Fatalf("all=%d err=%v", len(all), err)
	}
	counts, err := st.CountRunsByStatusBetween(start, end)
	if err != nil || counts[StatusRunning] < 3 {
		t.Fatalf("counts=%+v err=%v", counts, err)
	}

	prefix := "scope_test_" + suffix + "_"
	if err := st.UpsertSetting(prefix+"a", []byte(`1`)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(prefix+"b", []byte(`2`)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting("other_"+suffix, []byte(`3`)); err != nil {
		t.Fatal(err)
	}
	settings, err := st.ListSettingsByPrefix(prefix)
	if err != nil || len(settings) != 2 {
		t.Fatalf("settings=%v err=%v", settings, err)
	}
	if string(settings[prefix+"a"]) != "1" || string(settings[prefix+"b"]) != "2" {
		t.Fatalf("settings=%v", settings)
	}
}

func TestRunScopeMemory(t *testing.T) { runScopeCase(t, NewMemory()) }

func TestRunScopeSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "run_scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runScopeCase(t, st)
}

func TestRunScopePostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runScopeCase(t, st)
}
