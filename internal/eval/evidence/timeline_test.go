package evidence_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/eval/evidence"
)

func TestToolTimelineArgsText(t *testing.T) {
	evs := []evidence.Event{
		{
			Type: "llm.tool_call",
			Data: json.RawMessage(`{"id":"call_1","name":"get_ticket","arguments":{"id":"demo-1"}}`),
		},
		{
			Type: "tool.result",
			Data: json.RawMessage(`{"tool_call_id":"call_1","is_error":false}`),
		},
	}
	tl := evidence.ToolTimeline(evs)
	if len(tl) != 1 {
		t.Fatalf("timeline len=%d", len(tl))
	}
	if !strings.Contains(tl[0].ArgsText, "demo-1") || !strings.Contains(tl[0].ArgsText, "id") {
		t.Fatalf("ArgsText=%q", tl[0].ArgsText)
	}
}

func TestToolTimelineEngineToolCallID(t *testing.T) {
	evs := []evidence.Event{
		{
			Type: "llm.tool_call",
			Data: json.RawMessage(`{"id":"call_1","name":"list_tickets","arguments":{}}`),
		},
		{
			Type: "tool.result",
			Data: json.RawMessage(`{"tool_call_id":"call_1","name":"list_tickets","is_error":false}`),
		},
	}
	tl := evidence.ToolTimeline(evs)
	if len(tl) != 1 {
		t.Fatalf("timeline len=%d", len(tl))
	}
	if !tl[0].HasResult || tl[0].IsError {
		t.Fatalf("want successful result, got HasResult=%v IsError=%v", tl[0].HasResult, tl[0].IsError)
	}

	c := evidence.Case{
		Protocol:      "V0",
		RequiredReads: []string{"list_tickets"},
		Consequential: []string{"create_ticket"},
	}
	res := evidence.Score(c, tl, "")
	if !res.Pass {
		t.Fatalf("V0 required read should pass, codes=%v", res.Codes)
	}
}
