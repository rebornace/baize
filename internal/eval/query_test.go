package eval_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

func TestQueryCandidatesFilters(t *testing.T) {
	st := store.NewMemory()
	now := time.Now().UTC()

	rA, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "run-a-input"})
	_ = st.UpdateRun(rA.ID, store.StatusFailed, "", "boom")
	tA0 := rA.CreatedAt.Add(time.Second)
	_ = st.AppendEvent(rA.ID, store.Event{Type: "run.started", Timestamp: tA0})
	_ = st.AppendEvent(rA.ID, store.Event{Type: "tool.result", Timestamp: tA0.Add(time.Minute), Data: map[string]any{"id": "c1"}})

	rB, _ := st.CreateRun(store.CreateRunInput{AgentID: "b", Input: "run-b-input"})
	_ = st.UpdateRun(rB.ID, store.StatusSucceeded, "ok", "")
	_ = st.AppendEvent(rB.ID, store.Event{Type: "run.started", Timestamp: rB.CreatedAt.Add(time.Second)})

	rC, _ := st.CreateRun(store.CreateRunInput{AgentID: "c", Input: "run-c-input"})
	_ = st.UpdateRun(rC.ID, store.StatusFailed, "", "no events")

	out, err := eval.QueryCandidates(st, eval.Query{
		From:          now.Add(-time.Hour),
		To:            now.Add(time.Hour),
		Status:        store.StatusFailed,
		MinDurationMs: 0,
		EventType:     "tool.result",
		Limit:         50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != rA.ID {
		t.Fatalf("want only run A, got %+v", out)
	}
	if out[0].DurationMs == nil || out[0].EndedAt == nil {
		t.Fatalf("duration: %+v", out[0])
	}
}

func TestQueryMinDurationExcludesNoEvents(t *testing.T) {
	st := store.NewMemory()
	now := time.Now().UTC()

	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x"})
	_ = st.UpdateRun(r.ID, store.StatusFailed, "", "boom")

	out, err := eval.QueryCandidates(st, eval.Query{
		From:          now.Add(-time.Hour),
		To:            now.Add(time.Hour),
		Status:        store.StatusFailed,
		MinDurationMs: 1,
		Limit:         50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("no-event run must be excluded when min_duration_ms>0: %+v", out)
	}
}
