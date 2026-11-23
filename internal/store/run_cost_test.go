package store

import (
	"os"
	"path/filepath"
	"testing"
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
