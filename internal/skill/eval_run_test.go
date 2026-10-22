package skill_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
)

func TestScoreEvalCaseFromRunSkipsExpectState(t *testing.T) {
	c := skill.EvalCase{
		ID:            "c1",
		Protocol:      "V0",
		Input:         "x",
		RequiredReads: []string{"list_tickets"},
		Consequential: []string{"create_ticket"},
		ExpectState:   []skill.EvalExpectState{{ID: "t1", Status: "open"}},
	}
	evs := []store.Event{
		{Type: "llm.tool_call", Data: map[string]any{"id": "c1", "name": "list_tickets", "arguments": map[string]any{}}},
		{Type: "tool.result", Data: map[string]any{"id": "c1", "is_error": false}},
	}
	res := skill.ScoreEvalCaseFromRun(c, evs, string(store.StatusSucceeded))
	if !res.Pass {
		t.Fatalf("codes=%v", res.Codes)
	}
	if !strings.Contains(res.Notes, "state_check=skipped") {
		t.Fatalf("notes=%q", res.Notes)
	}
}
