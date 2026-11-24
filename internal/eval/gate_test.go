package eval

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestRunGateEmptyAndFailure(t *testing.T) {
	st := store.NewMemory()
	empty, err := RunGate(st, 0.9, 0)
	if err != nil || !empty.Empty || empty.Pass || empty.Total != 0 {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}

	now := time.Now().UTC()
	if err := st.UpsertEvalCase(store.EvalCase{
		ID: "ec_gate_fail", Status: store.EvalCaseConfirmed, Protocol: "V1",
		RequiredReads: []string{"read_ticket"}, Consequential: []string{"update_ticket"},
		EventsJSON: []byte("[]"), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertEvalCase(store.EvalCase{
		ID: "ec_gate_draft", Status: store.EvalCaseDraft, Protocol: "V1",
		EventsJSON: []byte("[]"), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := RunGate(st, 0.9, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || res.Failed != 1 || res.Pass || len(res.Failures) != 1 {
		t.Fatalf("res=%+v", res)
	}
	if res.Failures[0].ID != "ec_gate_fail" || len(res.Failures[0].Codes) == 0 {
		t.Fatalf("failure=%+v", res.Failures[0])
	}
	if res.RanAt.IsZero() {
		t.Fatal("RanAt not set")
	}

	pass, err := RunGate(st, 0, 0)
	if err != nil || !pass.Pass {
		t.Fatalf("threshold 0 should pass a non-empty suite: %+v err=%v", pass, err)
	}
}
