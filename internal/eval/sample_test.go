package eval_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

func TestSelectSampleIDsStable(t *testing.T) {
	ids := []string{"run_c", "run_a", "run_b", "run_d", "run_e"}
	a := eval.SelectSampleIDs(ids, 3, "seed1")
	b := eval.SelectSampleIDs(ids, 3, "seed1")
	if len(a) != 3 {
		t.Fatalf("len=%d", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("unstable: %v vs %v", a, b)
		}
	}
	c := eval.SelectSampleIDs(ids, 3, "other-seed-zzzz")
	if len(c) != 3 {
		t.Fatalf("len c=%d", len(c))
	}
}

func TestNormalizeSampleMaxN(t *testing.T) {
	if eval.NormalizeSampleMaxN(0) != 10 {
		t.Fatal("default")
	}
	if eval.NormalizeSampleMaxN(100) != 20 {
		t.Fatal("cap")
	}
}

func TestSampleBatchDraftSkipAndScore(t *testing.T) {
	st := store.NewMemory()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	mk := func(input string) *store.Run {
		r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: input})
		if err != nil {
			t.Fatal(err)
		}
		ts := time.Now().UTC().Add(-time.Minute)
		_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: ts})
		_ = st.AppendEvent(r.ID, store.Event{
			Type: "llm.tool_call", Timestamp: ts.Add(time.Second),
			Data: map[string]any{"name": "list_tickets", "id": "c1"},
		})
		return r
	}
	r1 := mk("one")
	r2 := mk("two")
	r3 := mk("three")

	// Pre-ingest r1 so sample skips it
	if _, err := eval.IngestFromRun(st, eval.IngestInput{RunID: r1.ID}); err != nil {
		t.Fatal(err)
	}

	draft, err := eval.SampleBatch(st, eval.SampleInput{
		Query: eval.Query{From: from, To: to, Limit: 100},
		MaxN:  10,
		Seed:  "t",
		Mode:  eval.SampleModeDraft,
	})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Candidates < 3 {
		t.Fatalf("candidates=%d", draft.Candidates)
	}
	if draft.Skipped < 1 {
		t.Fatalf("want skip for r1, got %+v", draft)
	}
	if draft.Created < 1 || draft.Scored != 0 {
		t.Fatalf("draft result=%+v", draft)
	}

	// Fresh store for score mode
	st2 := store.NewMemory()
	_ = r2
	_ = r3
	r4, _ := st2.CreateRun(store.CreateRunInput{AgentID: "a", Input: "score me"})
	ts := time.Now().UTC().Add(-time.Minute)
	_ = st2.AppendEvent(r4.ID, store.Event{Type: "run.started", Timestamp: ts})
	_ = st2.AppendEvent(r4.ID, store.Event{
		Type: "llm.tool_call", Timestamp: ts.Add(time.Second),
		Data: map[string]any{"name": "list_tickets", "id": "c1"},
	})
	_ = st2.AppendEvent(r4.ID, store.Event{
		Type: "tool.result", Timestamp: ts.Add(2 * time.Second),
		Data: map[string]any{"tool_call_id": "c1", "name": "list_tickets", "is_error": false},
	})

	_, err = eval.SampleBatch(st2, eval.SampleInput{
		Query: eval.Query{From: from, To: to},
		MaxN:  5,
		Seed:  "s",
		Mode:  eval.SampleModeScore,
		Ingest: eval.IngestInput{
			Protocol:      "V0",
			RequiredReads: []string{"list_tickets"},
			Consequential: []string{"create_ticket"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scored, err := eval.SampleBatch(st2, eval.SampleInput{
		Query: eval.Query{From: from, To: to},
		MaxN:  5,
		Seed:  "s",
		Mode:  eval.SampleModeScore,
		Ingest: eval.IngestInput{
			Protocol:      "V0",
			RequiredReads: []string{"list_tickets"},
			Consequential: []string{"create_ticket"},
		},
	})
	// second call: all skipped
	if err != nil {
		t.Fatal(err)
	}
	if scored.Created != 0 && scored.Skipped == 0 {
		t.Fatalf("second score pass should skip: %+v", scored)
	}

	// First score on fresh
	st3 := store.NewMemory()
	r5, _ := st3.CreateRun(store.CreateRunInput{AgentID: "a", Input: "score"})
	_ = st3.AppendEvent(r5.ID, store.Event{Type: "run.started", Timestamp: ts})
	_ = st3.AppendEvent(r5.ID, store.Event{
		Type: "llm.tool_call", Timestamp: ts.Add(time.Second),
		Data: map[string]any{"name": "list_tickets", "id": "c1"},
	})
	_ = st3.AppendEvent(r5.ID, store.Event{
		Type: "tool.result", Timestamp: ts.Add(2 * time.Second),
		Data: map[string]any{"tool_call_id": "c1", "name": "list_tickets", "is_error": false},
	})
	res, err := eval.SampleBatch(st3, eval.SampleInput{
		Query: eval.Query{From: from, To: to},
		MaxN:  5,
		Seed:  "x",
		Mode:  eval.SampleModeScore,
		Ingest: eval.IngestInput{
			Protocol:      "V0",
			RequiredReads: []string{"list_tickets"},
			Consequential: []string{"create_ticket"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 || res.Scored != 1 || len(res.CaseIDs) != 1 {
		t.Fatalf("score result=%+v", res)
	}
	c, err := st3.GetEvalCase(res.CaseIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != store.EvalCaseConfirmed || c.LastScorePass == nil {
		t.Fatalf("case=%+v", c)
	}

	_, err = eval.SampleBatch(st3, eval.SampleInput{
		Query: eval.Query{From: from, To: to},
		Mode:  eval.SampleModeScore,
		Ingest: eval.IngestInput{Protocol: "V0"}, // incomplete
	})
	if !errors.Is(err, eval.ErrInvalidSample) {
		t.Fatalf("want ErrInvalidSample, got %v", err)
	}
}
