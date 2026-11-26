package store

import (
	"testing"
	"time"
)

func seedCostBenchmark(b *testing.B, st Store, runs int) {
	b.Helper()
	base := time.Now().UTC().AddDate(0, 0, -3)
	for i := 0; i < runs; i++ {
		r, err := st.CreateRun(CreateRunInput{
			AgentID:        "a",
			Input:          "bench",
			ConversationID: "conv-" + string(rune('a'+i%26)),
			CreatedAt:      base,
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := st.PutRunCost(RunCost{
			RunID: r.ID, Day: base.Format("2006-01-02"), CostUSD: 0.01, Priced: true,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSumRunCostsPerDayGroupedMemory(b *testing.B) {
	st := NewMemory()
	seedCostBenchmark(b, st, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.SumRunCostsPerDayGrouped("conversation",
			time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02"),
			time.Now().UTC().Format("2006-01-02")); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSumRunCostsPerDayGroupedSQLite(b *testing.B) {
	st, err := Open("sqlite", b.TempDir()+"/bench.db")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if c, ok := st.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()
	seedCostBenchmark(b, st, 2000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := st.SumRunCostsPerDayGrouped("conversation",
			time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02"),
			time.Now().UTC().Format("2006-01-02")); err != nil {
			b.Fatal(err)
		}
	}
}
