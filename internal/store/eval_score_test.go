package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func evalScoreHistoryCase(t *testing.T, st Store) {
	t.Helper()
	id := "ec_" + uuid.NewString()
	now := time.Now().UTC()
	if err := st.UpsertEvalCase(EvalCase{
		ID: id, Status: EvalCaseConfirmed, Protocol: "V1",
		EventsJSON: []byte("[]"), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	older := now.Add(-time.Hour)
	if err := st.AppendEvalScore(EvalScoreRecord{
		CaseID: id, Pass: false, Codes: []string{"BSR"}, Notes: "missing_reads:a", ScoredAt: older,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEvalScore(EvalScoreRecord{
		CaseID: id, Pass: true, ScoredAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	recs, err := st.ListEvalScores(id, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || !recs[0].Pass || recs[1].Pass {
		t.Fatalf("records=%+v", recs)
	}
	if recs[1].Notes != "missing_reads:a" || len(recs[1].Codes) != 1 {
		t.Fatalf("older=%+v", recs[1])
	}
	one, err := st.ListEvalScores(id, 1)
	if err != nil || len(one) != 1 || !one[0].Pass {
		t.Fatalf("limit one=%+v err=%v", one, err)
	}
	if err := st.DeleteEvalCase(id); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.ListEvalScores(id, 10); len(left) != 0 {
		t.Fatalf("scores left after delete: %+v", left)
	}
}

func TestEvalScoreHistoryMemory(t *testing.T) {
	evalScoreHistoryCase(t, NewMemory())
}

func TestEvalScoreHistorySQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "eval_scores.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	evalScoreHistoryCase(t, st)
}

func TestEvalScoreHistoryPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	evalScoreHistoryCase(t, st)
}

func evalScorePruneCase(t *testing.T, st Store) {
	t.Helper()
	if err := st.UpsertSetting(SettingKeyEvalScoreHistoryCap, []byte("500")); err != nil {
		t.Fatal(err)
	}
	id := "ec_" + uuid.NewString()
	now := time.Now().UTC()
	if err := st.UpsertEvalCase(EvalCase{
		ID: id, Status: EvalCaseConfirmed, Protocol: "V1",
		EventsJSON: []byte("[]"), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteEvalCase(id) })
	base := now.Add(-time.Duration(EvalScoreHistoryCap+10) * time.Minute)
	total := EvalScoreHistoryCap + 5
	for i := 0; i < total; i++ {
		if err := st.AppendEvalScore(EvalScoreRecord{
			CaseID: id, Pass: i%2 == 0, Notes: fmt.Sprintf("n%d", i),
			ScoredAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// An old backfill must be dropped immediately, not pushed to the front.
	if err := st.AppendEvalScore(EvalScoreRecord{
		CaseID: id, Notes: "stale", ScoredAt: base.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	recs, err := st.ListEvalScores(id, EvalScoreHistoryCap+10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != EvalScoreHistoryCap {
		t.Fatalf("kept %d records, want %d", len(recs), EvalScoreHistoryCap)
	}
	if recs[0].Notes != fmt.Sprintf("n%d", total-1) {
		t.Fatalf("newest=%q", recs[0].Notes)
	}
	if recs[len(recs)-1].Notes != fmt.Sprintf("n%d", total-EvalScoreHistoryCap) {
		t.Fatalf("oldest=%q", recs[len(recs)-1].Notes)
	}
	for _, rec := range recs {
		if rec.Notes == "stale" {
			t.Fatal("stale backfill survived pruning")
		}
	}
}

func TestEvalScorePruneMemory(t *testing.T) { evalScorePruneCase(t, NewMemory()) }

func TestEvalScorePruneSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "eval_prune.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	evalScorePruneCase(t, st)
}

func TestEvalScorePrunePostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	evalScorePruneCase(t, st)
}

func evalScoreCapCase(t *testing.T, st Store) {
	t.Helper()
	t.Cleanup(func() { _ = st.UpsertSetting(SettingKeyEvalScoreHistoryCap, []byte("500")) })
	if err := st.UpsertSetting(SettingKeyEvalScoreHistoryCap, []byte("10")); err != nil {
		t.Fatal(err)
	}
	caseID := "cap-case-" + fmt.Sprintf("%d", time.Now().UnixNano())
	base := time.Now().UTC()
	for i := 0; i < 12; i++ {
		if err := st.AppendEvalScore(EvalScoreRecord{
			CaseID: caseID, Pass: true, Notes: fmt.Sprintf("n%d", i),
			ScoredAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := st.ListEvalScores(caseID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 10 {
		t.Fatalf("kept=%d want 10", len(recs))
	}
	if recs[0].Notes != "n11" || recs[9].Notes != "n2" {
		t.Fatalf("newest=%q oldest=%q", recs[0].Notes, recs[9].Notes)
	}
}

func TestEvalScoreCapMemory(t *testing.T) { evalScoreCapCase(t, NewMemory()) }

func TestEvalScoreCapSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "eval_cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	evalScoreCapCase(t, st)
}

func TestEvalScoreCapPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	evalScoreCapCase(t, st)
}
