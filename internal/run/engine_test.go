package run

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type scriptLLM struct{ calls int }

func (s *scriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "create_ticket", Arguments: map[string]any{"title": "x"}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "已创建"}, nil
}

func TestEngineReActToolThenMessage(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	})

	ag := agent.Def{ID: "ticket-agent", System: "you are a ticket helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}

	eng := &Engine{Store: st, LLM: &scriptLLM{}, Tools: reg}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, err := st.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}
	if got.Output != "已创建" {
		t.Fatalf("output=%q", got.Output)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	types := make(map[string]bool)
	for _, ev := range evs {
		types[ev.Type] = true
	}
	for _, want := range []string{"run.started", "llm.tool_call", "tool.result", "llm.message"} {
		if !types[want] {
			t.Fatalf("missing event %s; events=%+v", want, evs)
		}
	}
}

func TestEngineHITLApproveInvokesOnce(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "ticket-agent", System: "helper"})
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	}, true)

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}

	gate := NewGate()
	eng := &Engine{Store: st, LLM: &scriptLLM{}, Tools: reg, Gate: gate}

	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Execute(context.Background(), r.ID, ag, r.Input)
	}()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if calls.Load() != 0 {
		t.Fatalf("tool called before approve: %d", calls.Load())
	}
	if err := gate.Resume(r.ID, Decision{Approve: true, Comment: "lgtm"}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Execute timed out")
	}
	if calls.Load() != 1 {
		t.Fatalf("invoke count=%d want 1", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
	assertEventTypes(t, st, r.ID, EventHITLWaiting, EventHITLResumed)
}

func TestEngineHITLRejectNoInvoke(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	}, true)

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}

	gate := NewGate()
	eng := &Engine{Store: st, LLM: &scriptLLM{}, Tools: reg, Gate: gate}

	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Execute(context.Background(), r.ID, ag, r.Input)
	}()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if err := gate.Resume(r.ID, Decision{Approve: false, Comment: "nope"}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected reject error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Execute timed out")
	}
	if calls.Load() != 0 {
		t.Fatalf("invoke count=%d want 0", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusFailed {
		t.Fatalf("status=%s want failed", got.Status)
	}
	assertEventTypes(t, st, r.ID, EventHITLWaiting, EventHITLRejected)
}

func TestExecuteInjectsConversationID(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var sawConvID string
	reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		sawConvID = identity.ConversationIDFrom(ctx)
		return map[string]any{"id": "1"}, false, nil
	})

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "创建工单", ConversationID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}

	eng := &Engine{Store: st, LLM: &scriptLLM{}, Tools: reg}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if sawConvID != "c1" {
		t.Fatalf("ConversationIDFrom=%q want c1", sawConvID)
	}
}

func TestToolResultEventRedactsAccessToken(t *testing.T) {
	const jwt = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig"
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("login", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{
			"accessToken": jwt,
			"email":       "admin@x.com",
			"data":        map[string]any{"token": jwt, "role": "admin"},
		}, false, nil
	})

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "登录"})
	if err != nil {
		t.Fatal(err)
	}

	eng := &Engine{Store: st, LLM: &loginScriptLLM{}, Tools: reg}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range evs {
		if ev.Type != EventToolResult {
			continue
		}
		found = true
		content, _ := ev.Data["content"].(map[string]any)
		if content["accessToken"] != "[redacted]" {
			t.Fatalf("accessToken=%v want [redacted]", content["accessToken"])
		}
		if content["email"] != "admin@x.com" {
			t.Fatalf("email=%v", content["email"])
		}
		nested, _ := content["data"].(map[string]any)
		if nested["token"] != "[redacted]" {
			t.Fatalf("data.token=%v want [redacted]", nested["token"])
		}
		raw, _ := json.Marshal(ev.Data)
		if strings.Contains(string(raw), jwt) {
			t.Fatalf("tool.result still contains JWT: %s", raw)
		}
	}
	if !found {
		t.Fatal("missing tool.result event")
	}
}

type loginScriptLLM struct{ calls int }

func (s *loginScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "login", Arguments: map[string]any{}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "ok"}, nil
}

func TestContinueFromHITLInjectsConversationID(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "ticket-agent", System: "helper"})
	reg := tool.NewRegistry()
	var sawConvID string
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		sawConvID = identity.ConversationIDFrom(ctx)
		return map[string]any{"id": "9"}, false, nil
	}, true)

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "创建工单", ConversationID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}

	_ = st.AppendEvent(r.ID, store.Event{Type: EventRunStarted})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: EventLLMToolCall,
		Data: map[string]any{"id": "c1", "name": "create_ticket", "arguments": map[string]any{"title": "x"}},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: EventHITLWaiting,
		Data: map[string]any{"prompt": "Approve tool create_ticket?", "tool_name": "create_ticket"},
	})
	_ = st.UpdateRun(r.ID, store.StatusWaitingHuman, "", "")
	_ = st.SetHITL(r.ID, &store.HITLPayload{
		Prompt:    "Approve tool create_ticket?",
		ToolName:  "create_ticket",
		Arguments: map[string]any{"title": "x"},
	})

	eng := &Engine{
		Store: st,
		LLM:   &scriptLLM{calls: 1},
		Tools: reg,
		Gate:  NewGate(),
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	if sawConvID != "c1" {
		t.Fatalf("ConversationIDFrom=%q want c1", sawConvID)
	}
}

func TestContinueFromHITLColdApprove(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "ticket-agent", System: "helper"})
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"id": "9"}, false, nil
	}, true)

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate persisted waiting_human after process restart (no Gate waiter).
	_ = st.AppendEvent(r.ID, store.Event{Type: EventRunStarted})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: EventLLMToolCall,
		Data: map[string]any{"id": "c1", "name": "create_ticket", "arguments": map[string]any{"title": "x"}},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: EventHITLWaiting,
		Data: map[string]any{"prompt": "Approve tool create_ticket?", "tool_name": "create_ticket"},
	})
	_ = st.UpdateRun(r.ID, store.StatusWaitingHuman, "", "")
	_ = st.SetHITL(r.ID, &store.HITLPayload{
		Prompt:    "Approve tool create_ticket?",
		ToolName:  "create_ticket",
		Arguments: map[string]any{"title": "x"},
	})

	// LLM after cold resume: only the follow-up message (tool already done).
	eng := &Engine{
		Store: st,
		LLM: &scriptLLM{calls: 1}, // next Chat returns final message
		Tools: reg,
		Gate:  NewGate(), // empty — no waiter
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("invoke count=%d want 1", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
}

func waitStatus(t *testing.T, st store.Store, id string, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, err := st.GetRun(id)
		if err == nil && got.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, _ := st.GetRun(id)
	t.Fatalf("status=%v want %s", got, want)
}

func assertEventTypes(t *testing.T, st store.Store, runID string, want ...string) {
	t.Helper()
	evs, err := st.ListEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, ev := range evs {
		types[ev.Type] = true
	}
	for _, w := range want {
		if !types[w] {
			t.Fatalf("missing event %s; events=%+v", w, evs)
		}
	}
}
