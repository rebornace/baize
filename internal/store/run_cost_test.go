package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func runCostCase(t *testing.T, st Store) {
	t.Helper()
	if _, ok, err := st.GetRunCost("r1"); err != nil || ok {
		t.Fatalf("initial get ok=%v err=%v", ok, err)
	}
	if err := st.PutRunCost(RunCost{RunID: "r1", Day: "2026-11-23", CostUSD: 0.5, Priced: true}); err != nil {
		t.Fatal(err)
	}
	c, ok, err := st.GetRunCost("r1")
	if err != nil || !ok || c.CostUSD != 0.5 || !c.Priced || c.Day != "2026-11-23" {
		t.Fatalf("get=%+v ok=%v err=%v", c, ok, err)
	}
	if err := st.PutRunCost(RunCost{RunID: "r1", Day: "2026-11-23", CostUSD: 0.7}); err != nil {
		t.Fatal(err)
	}
	c, _, _ = st.GetRunCost("r1")
	if c.CostUSD != 0.7 || c.Priced {
		t.Fatalf("overwrite=%+v", c)
	}
}

func TestRunCostMemory(t *testing.T) { runCostCase(t, NewMemory()) }

func runCostScopeCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	agent1 := "scope-a1-" + suffix
	agent2 := "scope-a2-" + suffix
	conv1 := "scope-c1-" + suffix
	conv2 := "scope-c2-" + suffix
	model1 := "scope-m1-" + suffix
	model2 := "scope-m2-" + suffix
	mk := func(agentID, convID, modelID string) *Run {
		r, err := st.CreateRun(CreateRunInput{
			AgentID: agentID, Input: "x", ConversationID: convID, ModelProfileID: modelID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		return r
	}
	r1 := mk(agent1, conv1, model1)
	r2 := mk(agent1, conv2, model1)
	r3 := mk(agent2, conv1, model2)
	if err := st.PutRunCost(RunCost{RunID: r1.ID, Day: now.Format("2006-01-02"), CostUSD: 1.5, Priced: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutRunCost(RunCost{RunID: r3.ID, Day: now.Format("2006-01-02"), CostUSD: 2.5, Priced: true}); err != nil {
		t.Fatal(err)
	}
	missing, err := st.ListRunIDsMissingCostsSince(now.Add(-time.Hour), 5000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range missing {
		if id == r2.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing costs=%v, want %s", missing, r2.ID)
	}
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	if sum, err := st.SumRunCostsByScope(agent1, "", start, end); err != nil || sum != 1.5 {
		t.Fatalf("agent sum=%v err=%v", sum, err)
	}
	if sum, err := st.SumRunCostsByScope("", conv1, start, end); err != nil || sum != 4.0 {
		t.Fatalf("conv sum=%v err=%v", sum, err)
	}
	byModel, err := st.SumRunCostsGrouped("model", start, end)
	if err != nil || byModel[model1] != 1.5 || byModel[model2] != 2.5 {
		t.Fatalf("byModel=%v err=%v", byModel, err)
	}
	byAgent, err := st.SumRunCostsGrouped("agent", start, end)
	if err != nil || byAgent[agent1] != 1.5 || byAgent[agent2] != 2.5 {
		t.Fatalf("byAgent=%v err=%v", byAgent, err)
	}
	if _, err := st.SumRunCostsGrouped("bad", start, end); err == nil {
		t.Fatal("unknown dimension must error")
	}
}

func TestRunCostScopeMemory(t *testing.T) { runCostScopeCase(t, NewMemory()) }

func TestRunCostScopeSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "run_cost_scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runCostScopeCase(t, st)
}

func TestRunCostScopePostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runCostScopeCase(t, st)
}

func costDayCase(t *testing.T, st Store) {
	t.Helper()
	days, err := st.ListCostDays("2026-11-01", "2026-11-30")
	if err != nil || len(days) != 0 {
		t.Fatalf("initial=%v err=%v", days, err)
	}
	if err := st.PutCostDay(CostDay{Day: "2026-11-22", CostUSD: 1.5, Runs: 3, Finalized: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutCostDay(CostDay{Day: "2026-11-23", CostUSD: 0.25, Runs: 1}); err != nil {
		t.Fatal(err)
	}
	days, err = st.ListCostDays("2026-11-20", "2026-11-23")
	if err != nil || len(days) != 2 {
		t.Fatalf("list=%v err=%v", days, err)
	}
	if days[0].Day != "2026-11-22" || days[0].CostUSD != 1.5 || days[0].Runs != 3 || !days[0].Finalized {
		t.Fatalf("day0=%+v", days[0])
	}
	if days[1].Day != "2026-11-23" || days[1].Finalized || days[1].Runs != 1 {
		t.Fatalf("day1=%+v", days[1])
	}
	if err := st.PutCostDay(CostDay{Day: "2026-11-22", CostUSD: 2, Runs: 4, Finalized: true}); err != nil {
		t.Fatal(err)
	}
	days, _ = st.ListCostDays("2026-11-22", "2026-11-22")
	if len(days) != 1 || days[0].CostUSD != 2 || days[0].Runs != 4 {
		t.Fatalf("overwrite=%v", days)
	}
	days, _ = st.ListCostDays("2026-12-01", "2026-12-31")
	if len(days) != 0 {
		t.Fatalf("out of range=%v", days)
	}
}

func TestCostDayMemory(t *testing.T) { costDayCase(t, NewMemory()) }

func TestCostDaySQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "cost_day.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	costDayCase(t, st)
}

func clearCostCacheCase(t *testing.T, st Store) {
	t.Helper()
	if err := st.PutRunCost(RunCost{RunID: "r1", Day: "2026-11-23", CostUSD: 1, Priced: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.PutCostDay(CostDay{Day: "2026-11-23", CostUSD: 1, Runs: 1, Finalized: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.ClearCostCache(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := st.GetRunCost("r1"); ok {
		t.Fatal("run cost survived ClearCostCache")
	}
	if days, _ := st.ListCostDays("2026-01-01", "2026-12-31"); len(days) != 0 {
		t.Fatalf("day rollups survived ClearCostCache: %v", days)
	}
}

func TestClearCostCacheMemory(t *testing.T) { clearCostCacheCase(t, NewMemory()) }

func TestClearCostCacheSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "clear_cost.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	clearCostCacheCase(t, st)
}

func TestRunCostSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "run_cost.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	runCostCase(t, st)
}

func withPostgresCostStore(t *testing.T, fn func(Store)) {
	t.Helper()
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.ClearCostCache(); err != nil {
		t.Fatal(err)
	}
	fn(st)
}

func TestRunCostPostgres(t *testing.T) {
	withPostgresCostStore(t, func(st Store) { runCostCase(t, st) })
}

func TestCostDayPostgres(t *testing.T) {
	withPostgresCostStore(t, func(st Store) { costDayCase(t, st) })
}

func TestClearCostCachePostgres(t *testing.T) {
	withPostgresCostStore(t, func(st Store) { clearCostCacheCase(t, st) })
}
