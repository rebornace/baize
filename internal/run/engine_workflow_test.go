package run

import (
	"context"
	"errors"
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
	cat, err := skill.LoadCatalog([]string{root}, filepath.Join(root, "_user"), nil)
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

func TestExecutePinnedWorkflowUsesInputArgs(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var got map[string]any
	reg.Register("ta", func(_ context.Context, args map[string]any) (map[string]any, bool, error) {
		got = args
		return map[string]any{"ok": true}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", `name: demo
steps:
  - id: a
    tool: ta
    args:
      project_id: "{{input.project_id}}"
      text: "{{input.text}}"
`)
	ag := agent.Def{ID: "a", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		t.Fatal("workflow must not call the LLM")
		return llm.Message{}
	}}, Tools: reg, Skills: cat}
	err = eng.ExecuteWithOpts(context.Background(), r.ID, ag, r.Input, RunOptions{
		Skills:    []string{"demo"},
		InputArgs: map[string]any{"project_id": "p-42"},
	})
	if err != nil {
		t.Fatalf("ExecuteWithOpts: %v", err)
	}
	if got["project_id"] != "p-42" || got["text"] != "hello" {
		t.Fatalf("args=%+v", got)
	}
}

func TestExecutePinnedWorkflowRunsBeforeLLM(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var taCalls, tbCalls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		taCalls.Add(1)
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("tb", func(context.Context, map[string]any) (map[string]any, bool, error) {
		tbCalls.Add(1)
		return map[string]any{}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", workflowTwoStep)

	llmMock := &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		t.Fatal("pinned workflow must run before the first LLM turn")
		return llm.Message{}
	}}
	ag := agent.Def{ID: "a", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat}
	err = eng.ExecuteWithOpts(context.Background(), r.ID, ag, r.Input, RunOptions{Skills: []string{"demo"}})
	if err != nil {
		t.Fatalf("ExecuteWithOpts: %v", err)
	}
	if taCalls.Load() != 1 || tbCalls.Load() != 1 {
		t.Fatalf("tool calls: ta=%d tb=%d want 1 each", taCalls.Load(), tbCalls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	if !containsType(eventTypes(evs), "workflow.started") {
		t.Fatalf("missing workflow.started: %v", eventTypes(evs))
	}
}

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
		EventRunBinding, // R1 turn-0 dependency closure
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
	actRes := evs[3]
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

	eng := &Engine{Store: st, LLM: &scriptLLM{}, Tools: reg}
	err = eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true})
	if err == nil {
		t.Fatal("expected fail-fast error for workflow cold resume")
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusFailed {
		t.Fatalf("status=%s want failed", got.Status)
	}
	if got.Error != HITLWorkflowRestartNote {
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

func TestExecuteWorkflowApprovalReleasesAndResumes(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	var tbCalls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("tb", func(context.Context, map[string]any) (map[string]any, bool, error) {
		tbCalls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", `name: demo
steps:
  - id: a
    tool: ta
  - id: b
    tool: tb
    approve: true
    args:
      title: "{{input.text}}"
`)
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}},
		}}
	}}, Tools: reg, Skills: cat}

	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("Execute err=%v want ErrWaitingHuman", err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%s", got.Status)
	}
	if tbCalls.Load() != 0 {
		t.Fatalf("tb called before approve")
	}
	payload, _ := st.GetHITL(r.ID)
	if payload == nil || payload.Workflow == nil {
		t.Fatalf("missing workflow payload: %+v", payload)
	}
	if payload.Workflow.Skill != "demo" || payload.Workflow.StepID != "b" || payload.Workflow.StepIndex != 1 {
		t.Fatalf("workflow state=%+v", payload.Workflow)
	}

	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	if tbCalls.Load() != 1 {
		t.Fatalf("tb calls=%d want 1", tbCalls.Load())
	}
	got, _ = st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("after resume status=%s", got.Status)
	}
}

func TestWorkflowApprovalRejectStopsWithoutRunningStep(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	var taCalls, tbCalls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		taCalls.Add(1)
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("tb", func(context.Context, map[string]any) (map[string]any, bool, error) {
		tbCalls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n    approve: true\n  - id: b\n    tool: tb\n")
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}}}}
	}}, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("err=%v", err)
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: false, Comment: "no"}); err != nil {
		t.Fatal(err)
	}
	if taCalls.Load() != 0 || tbCalls.Load() != 0 {
		t.Fatalf("no step should run on reject: ta=%d tb=%d", taCalls.Load(), tbCalls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusRejected {
		t.Fatalf("status=%s want rejected", got.Status)
	}
}

func TestWorkflowTwoApprovalsAndRestartResume(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	var taCalls, tbCalls, tcCalls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		taCalls.Add(1)
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("tb", func(context.Context, map[string]any) (map[string]any, bool, error) {
		tbCalls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	})
	reg.Register("tc", func(context.Context, map[string]any) (map[string]any, bool, error) {
		tcCalls.Add(1)
		return map[string]any{"done": true}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n  - id: b\n    tool: tb\n    approve: true\n  - id: c\n    tool: tc\n    approve: true\n")
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}}}}
	}}, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("first pause: %v", err)
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("second pause: %v", err)
	}
	eng2 := &Engine{Store: st, Tools: reg, Skills: cat}
	if err := eng2.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("cold resume: %v", err)
	}
	if taCalls.Load() != 1 || tbCalls.Load() != 1 || tcCalls.Load() != 1 {
		t.Fatalf("calls ta=%d tb=%d tc=%d want 1 each", taCalls.Load(), tbCalls.Load(), tcCalls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
}

func lastEventOfType(t *testing.T, st store.Store, runID, typ string) *store.Event {
	t.Helper()
	evs, err := st.ListEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Type == typ {
			return &evs[i]
		}
	}
	return nil
}

func countEvents(evs []store.Event, typ, step string) int {
	n := 0
	for _, ev := range evs {
		if ev.Type != typ {
			continue
		}
		if step != "" && ev.Data["step"] != step {
			continue
		}
		n++
	}
	return n
}

func TestWorkflowApprovalEmitsStepEventsAndStructuredWait(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("tb", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n  - id: b\n    tool: tb\n    approve: true\n")
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}}}}
	}}, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("Execute err=%v", err)
	}

	wait := lastEventOfType(t, st, r.ID, EventHITLWaiting)
	if wait == nil {
		t.Fatal("missing hitl.waiting")
	}
	if wait.Data["workflow_step"] != "b" || wait.Data["workflow_skill"] != "demo" {
		t.Fatalf("hitl.waiting data=%+v", wait.Data)
	}
	if wait.Data["workflow_step_index"] != 1 {
		t.Fatalf("workflow_step_index=%v want 1", wait.Data["workflow_step_index"])
	}
	if _, ok := wait.Data["tree"]; ok {
		t.Fatal("hitl.waiting must not expose the workflow tree")
	}

	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	evs, _ := st.ListEvents(r.ID)
	if got := countEvents(evs, "workflow.step_started", "b"); got != 1 {
		t.Fatalf("step_started b=%d want 1; events=%+v", got, evs)
	}
	done := lastEventOfType(t, st, r.ID, "workflow.step_completed")
	if done == nil || done.Data["step"] != "b" || done.Data["is_error"] != false {
		t.Fatalf("step_completed=%+v", done)
	}
	if got := countEvents(evs, "workflow.step_completed", "b"); got != 1 {
		t.Fatalf("step_completed b=%d want 1", got)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestWorkflowLoginPausesAndResumes(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		if calls.Add(1) == 1 {
			return tool.LoginRequiredContentMsg("please login"), true, nil
		}
		return map[string]any{"ok": true}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n")
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}}}}
	}}, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("Execute err=%v", err)
	}
	payload, _ := st.GetHITL(r.ID)
	if payload == nil || payload.Kind != store.HITLKindLogin || payload.Workflow == nil {
		t.Fatalf("payload=%+v", payload)
	}
	if payload.Workflow.StepIndex != 0 || payload.Workflow.StepID != "a" {
		t.Fatalf("workflow=%+v", payload.Workflow)
	}
	wait := lastEventOfType(t, st, r.ID, EventHITLWaiting)
	if wait == nil || wait.Data["kind"] != store.HITLKindLogin || wait.Data["workflow_step"] != "a" {
		t.Fatalf("hitl.waiting=%+v", wait)
	}

	eng2 := &Engine{Store: st, Tools: reg, Skills: cat}
	if err := eng2.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("resume login: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d want 2", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	if got := countEvents(evs, "workflow.step_started", "a"); got != 1 {
		t.Fatalf("step_started a=%d want 1; events=%+v", got, evs)
	}
	if got := countEvents(evs, "workflow.step_completed", "a"); got != 1 {
		t.Fatalf("step_completed a=%d want 1", got)
	}
}

func TestWorkflowApproveStepLoginDoesNotReapprove(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		if calls.Add(1) == 1 {
			return tool.LoginRequiredContentMsg("please login"), true, nil
		}
		return map[string]any{"ok": true}, false, nil
	})
	cat := loadWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n    approve: true\n")
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}}}}
	}}, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("Execute err=%v", err)
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("approve err=%v", err)
	}
	payload, _ := st.GetHITL(r.ID)
	if payload == nil || payload.Kind != store.HITLKindLogin {
		t.Fatalf("after approve payload=%+v", payload)
	}
	evs, _ := st.ListEvents(r.ID)
	approvals := 0
	for _, ev := range evs {
		if ev.Type == EventHITLWaiting && ev.Data["kind"] == store.HITLKindApproval {
			approvals++
		}
	}
	if approvals != 1 {
		t.Fatalf("approval waits=%d want 1", approvals)
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("resume login: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d want 2", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestWorkflowLoginRejectStops(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.Register("ta", func(context.Context, map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return tool.LoginRequiredContentMsg("please login"), true, nil
	})
	cat := loadWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n")
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}}}}
	}}, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("Execute err=%v", err)
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: false, Comment: "abort_login"}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d want 1", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusRejected {
		t.Fatalf("status=%s want rejected", got.Status)
	}
}

func TestWorkflowToolApprovalInsideStepResumes(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "ta"}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"ok": true}, false, nil
	}, true)
	cat := loadWorkflowCatalog(t, "demo", "name: demo\nsteps:\n  - id: a\n    tool: ta\n")
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo"}}}}
	}}, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); !errors.Is(err, ErrWaitingHuman) {
		t.Fatalf("Execute err=%v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("calls before approve=%d", calls.Load())
	}
	payload, _ := st.GetHITL(r.ID)
	if payload == nil || payload.Workflow == nil || payload.Kind == store.HITLKindLogin {
		t.Fatalf("payload=%+v", payload)
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls=%d want 1", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	if got := countEvents(evs, "workflow.step_started", "a"); got != 1 {
		t.Fatalf("step_started a=%d want 1; events=%+v", got, evs)
	}
	if got := countEvents(evs, "workflow.step_completed", "a"); got != 1 {
		t.Fatalf("step_completed a=%d want 1", got)
	}
}

func TestExecuteFailsInterruptedWorkflowStep(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "helper"})
	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	_ = st.UpdateRun(r.ID, store.StatusRunning, "", "")
	_ = st.AppendEvent(r.ID, store.Event{Type: "workflow.started", Data: map[string]any{"skill": "demo", "steps": []string{"a"}}})
	_ = st.AppendEvent(r.ID, store.Event{Type: "workflow.step_started", Data: map[string]any{"step": "a", "tool": "ta"}})
	_ = st.SetHITL(r.ID, &store.HITLPayload{
		Kind:     store.HITLKindApproval,
		Workflow: &store.WorkflowState{Skill: "demo", StepIndex: 0, StepID: "a"},
	})
	var llmCalls atomic.Int32
	eng := &Engine{Store: st, LLM: &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		llmCalls.Add(1)
		return llm.Message{Role: llm.RoleAssistant, Content: "must not run"}
	}}}
	err := eng.Execute(context.Background(), r.ID, ag, r.Input)
	if err == nil || err.Error() != HITLWorkflowStepInterruptedNote {
		t.Fatalf("err=%v want %q", err, HITLWorkflowStepInterruptedNote)
	}
	if llmCalls.Load() != 0 {
		t.Fatalf("LLM must not run, calls=%d", llmCalls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusFailed {
		t.Fatalf("status=%s want failed", got.Status)
	}
	ev := lastEventOfType(t, st, r.ID, EventHITLInterrupted)
	if ev == nil || ev.Data["reason"] != "workflow_step_interrupted" {
		t.Fatalf("hitl.interrupted=%+v", ev)
	}
	if p, _ := st.GetHITL(r.ID); p != nil {
		t.Fatalf("HITL must be cleared, got %+v", p)
	}
}
