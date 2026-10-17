package run

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/capfill"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func proposePluginArgs() map[string]any {
	return map[string]any{
		"connector_id": "orders-ro",
		"goal":         "按月列订单",
		"gap":          "现有 list_orders 无 user/month 过滤",
		"read_only":    true,
		"risk":         "订单含 PII",
		"tools": []any{
			map[string]any{"name": "list-orders-by-month", "description": "只读按月"},
		},
	}
}

func registerProposePlugin(reg *tool.Registry, st store.Store) {
	tm := capfill.ProposePluginToolMeta(st)
	reg.RegisterSpecApproved(tm.Spec, tm.Invoker, true)
	_ = reg.RegisterArgsPrecheck(capfill.ProposeName, capfill.ProposePluginArgsPrecheck(st))
}

type proposePluginScriptLLM struct{ calls int }

func (s *proposePluginScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	for _, m := range messages {
		if m.Role == llm.RoleTool {
			return llm.Message{Role: llm.RoleAssistant, Content: "已保存"}, nil
		}
	}
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: capfill.ProposeName, Arguments: proposePluginArgs()},
	}}, nil
}

func (s *proposePluginScriptLLM) SupportsVision() bool { return false }

func setupProposePluginRun(t *testing.T) (*store.Memory, *tool.Registry) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	reg := tool.NewRegistry()
	registerProposePlugin(reg, st)
	return st, reg
}

func TestProposePluginRejectedWritesNothing(t *testing.T) {
	st, reg := setupProposePluginRun(t)
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "补插件", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &proposePluginScriptLLM{}, Tools: reg, Gate: gate,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(ctx, r.ID, ag, r.Input) }()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := gate.Resume(r.ID, Decision{Approve: false}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("want reject error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if _, err := st.GetConnector("orders-ro"); err == nil {
		t.Fatal("connector must not be written on reject")
	}
}

func TestProposePluginAdminApproveWritesDisabled(t *testing.T) {
	st, reg := setupProposePluginRun(t)
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "补插件", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &proposePluginScriptLLM{}, Tools: reg, Gate: gate,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(ctx, r.ID, ag, r.Input) }()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := gate.Resume(r.ID, Decision{
		Approve: true,
		Role:    controlplane.RoleAdmin,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if _, err := st.GetConnector("orders-ro"); err != nil {
		t.Fatal(err)
	}
	tl, err := st.GetTool("list-orders-by-month")
	if err != nil {
		t.Fatal(err)
	}
	if tl.Enabled {
		t.Fatal("tool must be disabled")
	}
}

func TestProposePluginOperatorApproveDenied(t *testing.T) {
	st, reg := setupProposePluginRun(t)
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "补插件", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &proposePluginScriptLLM{}, Tools: reg, Gate: gate,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(ctx, r.ID, ag, r.Input) }()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := gate.Resume(r.ID, Decision{
		Approve: true,
		Role:    controlplane.RoleOperator,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if _, err := st.GetConnector("orders-ro"); err == nil {
		t.Fatal("operator approve must not write connector")
	}
	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundDeny := false
	for _, ev := range evs {
		if ev.Type != EventToolResult {
			continue
		}
		if errMsg, ok := ev.Data["error"].(string); ok && errMsg != "" {
			foundDeny = true
			break
		}
		if content, ok := ev.Data["content"].(map[string]any); ok {
			if errMsg, ok := content["error"].(string); ok && errMsg != "" {
				foundDeny = true
				break
			}
		}
	}
	if !foundDeny {
		t.Fatal("want tool result error for operator approve")
	}
}

func TestProposePluginConflictNoHITL(t *testing.T) {
	st, reg := setupProposePluginRun(t)
	st.UpsertConnector(store.Connector{ID: "orders-ro", Type: "http"})

	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				return llm.Message{Role: llm.RoleAssistant, Content: "冲突"}
			}
		}
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: capfill.ProposeName, Arguments: proposePluginArgs()},
		}}
	}}
	ag := agent.Def{ID: "a", System: "sys"}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "补插件", ConversationID: "conv1"})
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: reg, Gate: NewGate(), MaxSteps: 4,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	if err := eng.Execute(ctx, r.ID, ag, r.Input); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status == store.StatusWaitingHuman {
		t.Fatal("must not enter waiting_human for connector conflict")
	}
}

func TestProposePluginBadArgsNoHITL(t *testing.T) {
	st, reg := setupProposePluginRun(t)
	badArgs := proposePluginArgs()
	badArgs["read_only"] = false

	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				return llm.Message{Role: llm.RoleAssistant, Content: "重试"}
			}
		}
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: capfill.ProposeName, Arguments: badArgs},
		}}
	}}
	ag := agent.Def{ID: "a", System: "sys"}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "补插件", ConversationID: "conv1"})
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: reg, Gate: NewGate(), MaxSteps: 4,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	if err := eng.Execute(ctx, r.ID, ag, r.Input); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status == store.StatusWaitingHuman {
		t.Fatal("must not enter waiting_human for invalid args")
	}
	if _, err := st.GetConnector("orders-ro"); err == nil {
		t.Fatal("connector must not be written")
	}
}
