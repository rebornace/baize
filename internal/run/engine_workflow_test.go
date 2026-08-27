package run

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// loadWorkflowCatalog writes a skill dir with SKILL.md + workflow.yaml and
// loads it as a Catalog, mirroring loadTestCatalog in skills_test.go.
func loadWorkflowCatalog(t *testing.T, id string, wfYAML string) *skill.Catalog {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: " + id + "\ndescription: pipeline\ntools:\n  - ta\n  - tb\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte(wfYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := skill.LoadCatalog([]string{root}, filepath.Join(root, "_user"))
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

const workflowTwoStep = `name: demo
steps:
  - id: a
    tool: ta
    args:
      q: "{{input.text}}"
  - id: b
    tool: tb
    args:
      x: "{{a.result.ok}}"
`

func TestExecuteWorkflowPipelineRunsToCompletion(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var taCalls, tbCalls atomic.Int32
	var sawTaArg any
	reg.Register("ta", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		taCalls.Add(1)
		sawTaArg = args["q"]
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("tb", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		tbCalls.Add(1)
		return map[string]any{}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", workflowTwoStep)

	chatCalls := atomic.Int32{}
	llmMock := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		chatCalls.Add(1)
		if chatCalls.Load() == 1 {
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}},
			}}
		}
		t.Fatalf("LLM must not be called after workflow activation; msgs=%+v", msgs)
		return llm.Message{}
	}}

	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if taCalls.Load() != 1 || tbCalls.Load() != 1 {
		t.Fatalf("tool calls: ta=%d tb=%d want 1 each", taCalls.Load(), tbCalls.Load())
	}
	if sawTaArg != "hi" {
		t.Fatalf("ta arg q=%v want input.text hi", sawTaArg)
	}

	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}

	evs, _ := st.ListEvents(r.ID)
	wantSeq := []string{
		EventRunStarted,
		EventToolResult, // activate_skill result (activate skips llm.tool_call)
		"workflow.started",
		"workflow.step_started", EventLLMToolCall, EventToolResult, "workflow.step_completed",
		"workflow.step_started", EventLLMToolCall, EventToolResult, "workflow.step_completed",
		EventLLMMessage,
	}
	var seq []string
	for _, ev := range evs {
		seq = append(seq, ev.Type)
	}
	if len(seq) < len(wantSeq) {
		t.Fatalf("events too short: %v", seq)
	}
	for i, w := range wantSeq {
		if seq[i] != w {
			t.Fatalf("event[%d]=%s want %s; full=%v", i, seq[i], w, seq)
		}
	}

	// The llm.message terminal marker comes from the workflow completion path.
	last := evs[len(evs)-1]
	if last.Type != EventLLMMessage || last.Data["content"] != "workflow completed" {
		t.Fatalf("terminal event=%+v", last)
	}
	// activate_skill's tool.result must report the activation success.
	actRes := evs[2]
	if isErr, _ := actRes.Data["is_error"].(bool); isErr {
		t.Fatalf("activate_skill result must not be error: %+v", actRes.Data)
	}
}

func TestExecuteWorkflowStepFailureFailsRun(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("ta", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return nil, false, context.DeadlineExceeded
	})
	reg.Register("tb", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		t.Fatal("tb must not run after step failure")
		return nil, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", workflowTwoStep)

	llmMock := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}},
		}}
	}}
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat, ToolTimeout: 10}
	_ = eng.Execute(context.Background(), r.ID, ag, r.Input)

	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusFailed {
		t.Fatalf("status=%s want failed", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	var sawWfErr bool
	for _, ev := range evs {
		if ev.Type == EventLLMError {
			sawWfErr = true
		}
	}
	if !sawWfErr {
		t.Fatalf("missing %s event", EventLLMError)
	}
}

func TestExecuteActivateNonWorkflowSkillKeepsReAct(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("list_tickets", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	cat := loadTestCatalog(t, map[string]struct {
		desc  string
		tools []string
		body  string
	}{
		"demo": {desc: "list", tools: []string{"list_tickets"}, body: "body"},
	})

	chatCalls := 0
	llmMock := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		chatCalls++
		if chatCalls == 1 {
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}},
			}}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "done react"}
	}}
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if chatCalls < 2 {
		t.Fatalf("ReAct path must keep chatting after non-workflow activation")
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded || got.Output != "done react" {
		t.Fatalf("run=%+v", got)
	}
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == "workflow.started" {
			t.Fatal("non-workflow skill must not enter DSL mode")
		}
	}
	st2 := e_getState(eng, r.ID)
	if st2.workflowStarted {
		t.Fatal("workflowStarted flag leaked into ReAct run")
	}
}

func TestContinueFromHITLWorkflowFailFast(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "ticket-agent", System: "helper"})
	reg := tool.NewRegistry()
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "9"}, false, nil
	}, true)

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}

	// Persisted state simulating a process restart mid-workflow:
	// activation done, then step b waiting on approval.
	_ = st.AppendEvent(r.ID, store.Event{Type: EventRunStarted})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: EventLLMToolCall,
		Data: map[string]any{"id": "c0", "name": skill.ActivateToolName, "arguments": map[string]any{"id": "wf-skill"}},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: EventToolResult,
		Data: map[string]any{"tool_call_id": "c0", "name": skill.ActivateToolName, "content": map[string]any{}, "is_error": false},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: "workflow.started",
		Data: map[string]any{"skill": "wf-skill", "steps": []string{"a", "b"}},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: "workflow.step_started",
		Data: map[string]any{"step": "b", "tool": "create_ticket"},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: EventHITLWaiting,
		Data: map[string]any{"prompt": "workflow step \"b\": create_ticket", "tool_name": "create_ticket"},
	})
	_ = st.UpdateRun(r.ID, store.StatusWaitingHuman, "", "")
	_ = st.SetHITL(r.ID, &store.HITLPayload{
		Prompt:    "workflow step \"b\": create_ticket",
		ToolName:  "create_ticket",
		Arguments: map[string]any{"x": "1"},
	})

	eng := &Engine{Store: st, LLM: &scriptLLM{}, Tools: reg, Gate: NewGate()}
	err = eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true})
	if err == nil {
		t.Fatal("expected fail-fast error for workflow cold resume")
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusFailed {
		t.Fatalf("status=%s want failed", got.Status)
	}
	if got.Error != "workflow run interrupted by restart; please re-run" {
		t.Fatalf("error=%q", got.Error)
	}
	evs, _ := st.ListEvents(r.ID)
	var sawRejectWaitInvoke bool
	for _, ev := range evs {
		if ev.Type == EventToolResult && asString(ev.Data["name"]) == "create_ticket" {
			sawRejectWaitInvoke = true
		}
	}
	if sawRejectWaitInvoke {
		t.Fatal("cold resume must not invoke the pending workflow step")
	}
}

// e_getState grants read access to the internal per-run skill state for tests.
func e_getState(e *Engine, runID string) *runSkillState {
	return e.getRunSkillState(runID)
}
