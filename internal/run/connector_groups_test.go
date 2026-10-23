package run

import (
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/llm"
)

func TestExpandAllowedByConnectorGroups(t *testing.T) {
	idx := BuildConnectorGroupIndex([]config.ConnectorGroup{
		{ID: "finance", Connectors: []string{"expense", "invoice"}},
		{ID: "commerce", Connectors: []string{"mall", "order"}},
	})
	specs := []llm.ToolSpec{
		{Name: "a", Source: "expense"},
		{Name: "b", Source: "invoice"},
		{Name: "c", Source: "mall"},
		{Name: "d", Source: "orphan"},
	}
	allowed := map[string]bool{"expense": true}
	out, expanded := expandAllowedByConnectorGroups(idx, specs, allowed)
	if !out["invoice"] || !out["expense"] {
		t.Fatalf("finance siblings: %+v", out)
	}
	if out["mall"] || out["orphan"] {
		t.Fatalf("must not pull other groups: %+v", out)
	}
	if len(expanded) != 1 || expanded[0] != "invoice" {
		t.Fatalf("expanded=%v", expanded)
	}
	// Fail-open unchanged.
	open, exp := expandAllowedByConnectorGroups(idx, specs, nil)
	if open != nil || exp != nil {
		t.Fatalf("nil allowed must stay nil: %v %v", open, exp)
	}
}

func TestBuildConnectorGroupIndexFirstWins(t *testing.T) {
	idx := BuildConnectorGroupIndex([]config.ConnectorGroup{
		{ID: "g1", Connectors: []string{"a", "b"}},
		{ID: "g2", Connectors: []string{"b", "c"}},
	})
	if idx.bySource["b"] != "g1" {
		t.Fatalf("first group wins: %+v", idx.bySource)
	}
	if _, ok := idx.byGroup["g2"]; !ok {
		t.Fatal("g2 should still exist for c")
	}
}

func TestLastTurnHadToolError(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "q"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "1", Name: "x"}}},
		{Role: llm.RoleTool, ToolCallID: "1", Content: `{"error":"unknown tool"}`},
	}
	if !lastTurnHadToolError(msgs) {
		t.Fatal("expected tool error")
	}
	okMsgs := []llm.Message{
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "1", Name: "x"}}},
		{Role: llm.RoleTool, ToolCallID: "1", Content: `{"ok":true}`},
	}
	if lastTurnHadToolError(okMsgs) {
		t.Fatal("success must not count as error")
	}
}

func TestStickyClearedAfterToolError(t *testing.T) {
	eng := &Engine{}
	eng.SetConnectorGroups(nil)
	catalog := []llm.ToolSpec{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}
	_, _, _ = eng.applyStickyNarrow("c1", []llm.ToolSpec{{Name: "t1"}, {Name: "t2"}}, catalog)
	eng.clearStickyNarrow("c1")
	sent, reused, _ := eng.applyStickyNarrow("c1", []llm.ToolSpec{{Name: "t3"}}, catalog)
	if reused || sent[0].Name != "t3" {
		t.Fatalf("after clear must accept new set: reused=%v sent=%v", reused, toolNames(sent))
	}
}
