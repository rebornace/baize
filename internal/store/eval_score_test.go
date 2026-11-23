package store

import (
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
