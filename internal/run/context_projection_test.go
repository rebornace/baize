package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/tool"
)

func TestBuildMessagesInjectsProjectionAfterHistory(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{
		Messages:    ms,
		MaxMessages: 40,
		Settings:    fakeKnobs{k: runtimecfg.Knobs{ContextProjectionEnabled: true}},
	}
	if _, err := ms.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "上一问"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "上一答"}); err != nil {
		t.Fatal(err)
	}
	if err := ms.UpsertContextProjection(conversation.ContextProjection{
		ConversationID: "c1",
		Pins:           []string{"订单 ORD-1"},
		Summary:        "正在查物流",
	}); err != nil {
		t.Fatal(err)
	}
	msgs := e.buildMessages("系统", "c1", "现在的问题", nil)
	if msgs[0].Content != "系统" {
		t.Fatalf("system first: %+v", msgs[0])
	}
	var histIdx, projIdx, userIdx = -1, -1, -1
	for i, m := range msgs {
		switch {
		case m.Role == llm.RoleUser && m.Content == "上一问":
			histIdx = i
		case m.Role == llm.RoleSystem && strings.Contains(m.Content, "订单 ORD-1"):
			projIdx = i
		case m.Role == llm.RoleUser && m.Content == "现在的问题":
			userIdx = i
		}
	}
	if histIdx < 0 || projIdx < 0 || userIdx < 0 {
		t.Fatalf("missing parts: hist=%d proj=%d user=%d msgs=%+v", histIdx, projIdx, userIdx, msgs)
	}
	if histIdx >= projIdx || projIdx >= userIdx {
		t.Fatalf("projection must sit after history and before current user: %d %d %d", histIdx, projIdx, userIdx)
	}
	raw := ms.List("c1")
	if len(raw) != 2 || raw[0].Content != "上一问" {
		t.Fatalf("persisted history must be unchanged: %+v", raw)
	}
}

func TestBuildMessagesSkipsProjectionWhenSwitchOff(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms, MaxMessages: 40}
	if err := ms.UpsertContextProjection(conversation.ContextProjection{
		ConversationID: "c1", Pins: []string{"secret"},
	}); err != nil {
		t.Fatal(err)
	}
	msgs := e.buildMessages("系统", "c1", "问", nil)
	for _, m := range msgs {
		if strings.Contains(m.Content, "secret") {
			t.Fatalf("switch off must not inject projection: %+v", msgs)
		}
	}
}

func TestSpecsForRunOmitsContextToolsUntilArmed(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register("echo", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	e := &Engine{Tools: reg}
	for _, tm := range e.ContextTools() {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	e.beginRunSkills("r1", nil, "sys")
	specs := e.specsForRun("r1")
	for _, s := range specs {
		if contextToolNames[s.Name] {
			t.Fatalf("unarmed must hide %s", s.Name)
		}
	}
	e.setContextArmed("r1", true)
	specs = e.specsForRun("r1")
	seen := map[string]bool{}
	for _, s := range specs {
		seen[s.Name] = true
	}
	for name := range contextToolNames {
		if !seen[name] {
			t.Fatalf("armed must include %s", name)
		}
	}
}

func TestApplyDroppedStubsLivePromptOnly(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms}
	if _, err := ms.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "keep"}); err != nil {
		t.Fatal(err)
	}
	if err := ms.UpsertContextProjection(conversation.ContextProjection{
		ConversationID: "c1", Dropped: []string{"tc1"},
	}); err != nil {
		t.Fatal(err)
	}
	msgs := []llm.Message{
		{Role: llm.RoleTool, ToolCallID: "tc1", Content: strings.Repeat("huge", 50)},
		{Role: llm.RoleTool, ToolCallID: "tc2", Content: "keep-me"},
	}
	e.applyDroppedToolStubs(msgs, "c1")
	if msgs[0].Content != droppedToolPlaceholder {
		t.Fatalf("tc1 should be stubbed: %q", msgs[0].Content)
	}
	if msgs[1].Content != "keep-me" {
		t.Fatalf("tc2 must stay: %q", msgs[1].Content)
	}
	if got := ms.List("c1"); len(got) != 1 || got[0].Content != "keep" {
		t.Fatalf("store must stay: %+v", got)
	}
}

func TestContextPinDoesNotRewriteMessages(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms}
	if _, err := ms.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "orig"}); err != nil {
		t.Fatal(err)
	}
	ctx := identity.WithConversationID(context.Background(), "c1")
	out, isErr, err := e.contextPin(ctx, map[string]any{"text": "钉住审批结论"})
	if err != nil || isErr {
		t.Fatalf("pin: %v isErr=%v out=%v", err, isErr, out)
	}
	p, ok := ms.GetContextProjection("c1")
	if !ok || len(p.Pins) != 1 || p.Pins[0] != "钉住审批结论" {
		t.Fatalf("projection: %+v ok=%v", p, ok)
	}
	if got := ms.List("c1"); len(got) != 1 || got[0].Content != "orig" {
		t.Fatalf("messages rewritten: %+v", got)
	}
}
