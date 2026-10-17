package run

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func proposeSkillArgs() map[string]any {
	return map[string]any{
		"name":        "demo",
		"description": "Demo skill",
		"body":        "Do the thing",
		"tools":       []any{"read_file", "write_file"},
	}
}

func proposeTestCatalog(t *testing.T) *skill.Catalog {
	t.Helper()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	if err := os.MkdirAll(builtin, 0o755); err != nil {
		t.Fatal(err)
	}
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := skill.LoadCatalog([]string{builtin}, filepath.Join(root, "user"), blobs)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func registerProposeSkill(reg *tool.Registry, cat *skill.Catalog) {
	tm := skill.ProposeToolMeta(cat)
	reg.RegisterSpecApproved(tm.Spec, tm.Invoker, true)
	_ = reg.RegisterArgsPrecheck(skill.ProposeName, skill.ValidateProposeArgs)
}

type proposeScriptLLM struct{ calls int }

func (s *proposeScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	for _, m := range messages {
		if m.Role == llm.RoleTool {
			return llm.Message{Role: llm.RoleAssistant, Content: "已保存"}, nil
		}
	}
	return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
		{ID: "c1", Name: skill.ProposeName, Arguments: proposeSkillArgs()},
	}}, nil
}

func (s *proposeScriptLLM) SupportsVision() bool { return false }

func setupProposeRun(t *testing.T) (*store.Memory, *skill.Catalog, *tool.Registry) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	cat := proposeTestCatalog(t)
	reg := tool.NewRegistry()
	registerProposeSkill(reg, cat)
	return st, cat, reg
}

func TestProposeSkillRejectedInstallsNothing(t *testing.T) {
	st, cat, reg := setupProposeRun(t)
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "写 skill", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &proposeScriptLLM{}, Tools: reg, Gate: gate,
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
	if _, ok := cat.Get("demo"); ok {
		t.Fatal("skill must not be installed on reject")
	}
}

func TestProposeSkillApprovedInstalls(t *testing.T) {
	st, cat, reg := setupProposeRun(t)
	ag := agent.Def{ID: "a", System: "sys"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "写 skill", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	gate := NewGate()
	eng := &Engine{
		Store: st, LLM: &proposeScriptLLM{}, Tools: reg, Gate: gate,
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(ctx, r.ID, ag, r.Input) }()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := gate.Resume(r.ID, Decision{Approve: true}); err != nil {
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
	pkg, ok := cat.Get("demo")
	if !ok {
		t.Fatal("skill not installed")
	}
	if pkg.Description != "Demo skill" || pkg.ID != "demo" {
		t.Fatalf("pkg=%+v", pkg)
	}
	if pkg.Body != "Do the thing" {
		t.Fatalf("body=%q", pkg.Body)
	}
	if len(pkg.Tools) != 2 || pkg.Tools[0] != "read_file" || pkg.Tools[1] != "write_file" {
		t.Fatalf("tools=%v", pkg.Tools)
	}
}

func TestProposeSkillCancelledInstallsNothing(t *testing.T) {
	st, cat, _ := setupProposeRun(t)
	var invokes atomic.Int32
	tm := skill.ProposeToolMeta(cat)
	inner := tm.Invoker
	reg2 := tool.NewRegistry()
	reg2.RegisterSpecApproved(tm.Spec, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		invokes.Add(1)
		return inner(ctx, args)
	}, true)
	_ = reg2.RegisterArgsPrecheck(skill.ProposeName, skill.ValidateProposeArgs)

	ag := agent.Def{ID: "a", System: "sys"}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "写 skill", ConversationID: "conv1"})
	eng := &Engine{
		Store: st, LLM: &proposeScriptLLM{}, Tools: reg2, Gate: NewGate(),
		Now: func() time.Time { return scheduleTestNow() },
	}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	done := make(chan error, 1)
	go func() { done <- eng.Execute(ctx, r.ID, ag, r.Input) }()
	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := eng.Cancel(r.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	if invokes.Load() != 0 {
		t.Fatalf("invoke count=%d want 0", invokes.Load())
	}
	if _, ok := cat.Get("demo"); ok {
		t.Fatal("skill must not be installed on cancel")
	}
}

func TestProposeSkillBadArgsNoHITL(t *testing.T) {
	st, cat, reg := setupProposeRun(t)
	badArgs := map[string]any{
		"name": "demo", "description": "d", "body": "",
	}
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				return llm.Message{Role: llm.RoleAssistant, Content: "重试"}
			}
		}
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: skill.ProposeName, Arguments: badArgs},
		}}
	}}
	ag := agent.Def{ID: "a", System: "sys"}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "写 skill", ConversationID: "conv1"})
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
	if _, ok := cat.Get("demo"); ok {
		t.Fatal("skill must not be installed")
	}
}
