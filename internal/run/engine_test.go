package run

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// captureLLM records every Chat call's input messages and returns a fixed
// assistant reply via onChat. Used to assert history-window injection.
type captureLLM struct {
	onChat func(msgs []llm.Message, tools []llm.ToolSpec) llm.Message
	err    error
}

func (c *captureLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	if c.err != nil {
		return llm.Message{}, c.err
	}
	return c.onChat(messages, tools), nil
}

func (c *captureLLM) SupportsVision() bool { return false }

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

func (s *scriptLLM) SupportsVision() bool { return false }

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

func TestLoginGateBeforeHITL(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterMeta(tool.Meta{
		Spec: llm.ToolSpec{Name: "create_ticket"}, ConnectorID: "c", RequireLogin: true,
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	}, true) // require_approval + require_login
	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "创建工单", ConversationID: "conv_hitl",
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store: st, LLM: &scriptLLM{}, Tools: reg, Gate: NewGate(),
		Identities: identity.NewMemoryStore(),
	}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status == store.StatusWaitingHuman {
		t.Fatal("must not enter waiting_human")
	}
	if calls.Load() != 0 {
		t.Fatalf("invoke=%d", calls.Load())
	}
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventHITLWaiting {
			t.Fatal("hitl.waiting")
		}
		if ev.Type == EventToolResult {
			if ev.Data["is_error"] != true {
				t.Fatalf("%+v", ev.Data)
			}
		}
	}
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

func (s *loginScriptLLM) SupportsVision() bool { return false }

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
		LLM:   &scriptLLM{calls: 1}, // next Chat returns final message
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

// TestExecuteInjectsPassthroughHeaders: injectAuthCtxFromRun is the production
// path that copies Run.PassthroughHeaders into ctx. A tool closure reads
// identity.PassthroughHeadersFrom(ctx) and asserts the run-private headers
// arrived — validating the engine wiring (not just the register closure).
func TestExecuteInjectsPassthroughHeaders(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var sawAuth, sawRun, sawAgent string
	reg.Register("probe", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
			sawAuth = h["Authorization"]
		}
		sawRun = identity.RunIDFrom(ctx)
		sawAgent = identity.AgentIDFrom(ctx)
		return map[string]any{"ok": true}, false, nil
	})

	probeLLM := &probeScriptLLM{}
	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID:            ag.ID,
		Input:              "探测",
		PassthroughHeaders: map[string]string{"Authorization": "Bearer RUN_TOKEN"},
	})
	if err != nil {
		t.Fatal(err)
	}

	eng := &Engine{Store: st, LLM: probeLLM, Tools: reg}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if sawAuth != "Bearer RUN_TOKEN" {
		t.Fatalf("PassthroughHeadersFrom(ctx)=%q want Bearer RUN_TOKEN", sawAuth)
	}
	if sawRun != r.ID {
		t.Fatalf("RunIDFrom(ctx)=%q want %q", sawRun, r.ID)
	}
	if sawAgent != ag.ID {
		t.Fatalf("AgentIDFrom(ctx)=%q want %q", sawAgent, ag.ID)
	}
}

// probeScriptLLM issues a single probe tool call then a final message.
type probeScriptLLM struct{ calls int }

func (s *probeScriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "p1", Name: "probe", Arguments: map[string]any{}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}

func (s *probeScriptLLM) SupportsVision() bool { return false }

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

func TestExecuteInjectsConversationHistory(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "上一轮问题"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "上一轮回答"})

	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "本轮回答"}
	}}
	eng := &Engine{Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4, Messages: msgStore, MaxMessages: 40}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "本轮问题", ConversationID: "conv1"})
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "本轮问题"); err != nil {
		t.Fatal(err)
	}
	// expect: system, 上一轮 user, 上一轮 assistant, 本轮 user
	if len(saw) < 4 || saw[1].Content != "上一轮问题" || saw[3].Content != "本轮问题" {
		t.Fatalf("%+v", saw)
	}
}

// TestExecuteRespectsMaxMessagesWindow: only the most recent MaxMessages
// history entries are injected into the LLM prompt.
func TestExecuteRespectsMaxMessagesWindow(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "旧问题"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "旧回答"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "近问题"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "近回答"})

	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "本轮回答"}
	}}
	eng := &Engine{Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4, Messages: msgStore, MaxMessages: 2}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "本轮问题", ConversationID: "conv1"})
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "本轮问题"); err != nil {
		t.Fatal(err)
	}
	// expect: system, 近问题, 近回答, 本轮问题 — not 旧*
	if len(saw) != 4 {
		t.Fatalf("len(saw)=%d want 4; saw=%+v", len(saw), saw)
	}
	if saw[1].Content != "近问题" || saw[2].Content != "近回答" || saw[3].Content != "本轮问题" {
		t.Fatalf("window not applied: %+v", saw)
	}
	for _, m := range saw {
		if m.Content == "旧问题" || m.Content == "旧回答" {
			t.Fatalf("old messages leaked into prompt: %+v", saw)
		}
	}
}

// TestExecuteDedupCurrentInput: when the API has already appended the current
// user input to the message store before calling Execute, the engine must not
// append it a second time.
func TestExecuteDedupCurrentInput(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "上一轮回答"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "本轮问题"})

	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "本轮回答"}
	}}
	eng := &Engine{Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4, Messages: msgStore, MaxMessages: 40}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "本轮问题", ConversationID: "conv1"})
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "本轮问题"); err != nil {
		t.Fatal(err)
	}
	// expect: system, 上一轮 assistant, 本轮 user (no duplicate)
	var userCount int
	for _, m := range saw {
		if m.Role == llm.RoleUser && m.Content == "本轮问题" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("expected exactly one 本轮问题 user message, got %d; saw=%+v", userCount, saw)
	}
	if len(saw) != 3 || saw[2].Content != "本轮问题" {
		t.Fatalf("unexpected messages=%+v", saw)
	}
}

// TestExecuteWritesAssistantOnSuccess: a succeeded run must append an assistant
// message with the run output to the conversation store.
func TestExecuteWritesAssistantOnSuccess(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()

	llmStub := &captureLLM{onChat: func(_ []llm.Message, _ []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "最终答复"}
	}}
	eng := &Engine{Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4, Messages: msgStore, MaxMessages: 40}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "问", ConversationID: "conv1"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "问", RunID: r.ID})
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "问"); err != nil {
		t.Fatal(err)
	}
	msgs := msgStore.List("conv1")
	// Expect exactly one assistant message with the final output.
	var assistant *conversation.Message
	for i := range msgs {
		if msgs[i].Role == conversation.RoleAssistant {
			assistant = &msgs[i]
			break
		}
	}
	if assistant == nil || assistant.Content != "最终答复" {
		t.Fatalf("missing assistant terminal message; msgs=%+v", msgs)
	}
	if assistant.RunID != r.ID {
		t.Fatalf("assistant.RunID=%q want %q", assistant.RunID, r.ID)
	}
}

// TestExecuteWritesSystemNoteOnFailure: a failed run (LLM error) must append a
// system_note message prefixed with "运行失败：".
func TestExecuteWritesSystemNoteOnFailure(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()

	llmStub := &captureLLM{err: fmt.Errorf("upstream 502")}
	eng := &Engine{Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4, Messages: msgStore, MaxMessages: 40}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "问", ConversationID: "conv1"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "问", RunID: r.ID})
	// Execute returns the LLM error; the engine still records the terminal note.
	_ = eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "问")

	msgs := msgStore.List("conv1")
	var note *conversation.Message
	for i := range msgs {
		if msgs[i].Role == conversation.RoleSystemNote {
			note = &msgs[i]
			break
		}
	}
	if note == nil {
		t.Fatalf("missing system_note terminal message; msgs=%+v", msgs)
	}
	if !strings.HasPrefix(note.Content, "运行失败：") {
		t.Fatalf("note.Content=%q want prefix 运行失败：", note.Content)
	}
	if !strings.Contains(note.Content, "upstream 502") {
		t.Fatalf("note.Content=%q want to contain error", note.Content)
	}
}

// TestExecuteNoMessageOnWaitingHuman: a run that ends in waiting_human must
// NOT write an assistant or system_note terminal message.
func TestExecuteNoMessageOnWaitingHuman(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	reg := tool.NewRegistry()
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	}, true)

	// First Chat call issues a tool call requiring approval → enters waiting_human.
	llmStub := &captureLLM{onChat: func(_ []llm.Message, _ []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "create_ticket", Arguments: map[string]any{"title": "x"}},
		}}
	}}
	eng := &Engine{Store: st, LLM: llmStub, Tools: reg, Gate: NewGate(), MaxSteps: 4, Messages: msgStore, MaxMessages: 40}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "创建", ConversationID: "conv1"})

	errCh := make(chan error, 1)
	go func() { errCh <- eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "创建") }()
	waitStatus(t, st, r.ID, store.StatusWaitingHuman)

	// While waiting, no terminal message should have been written.
	msgs := msgStore.List("conv1")
	for _, m := range msgs {
		if m.Role == conversation.RoleAssistant || m.Role == conversation.RoleSystemNote {
			t.Fatalf("waiting_human should not write terminal message; got %+v", m)
		}
	}

	// Unblock the goroutine so it can exit; reject → failed (writes system_note).
	_ = eng.Gate.Resume(r.ID, Decision{Approve: false, Comment: "no"})
	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		t.Fatal("Execute did not return after reject")
	}
}

// TestContinueFromHITLColdInjectsHistory: on a cold restart (no Gate waiter),
// ContinueFromHITL must inject the conversation window history (system + prior
// turns + current user input via buildMessages) before replaying this run's
// tool-call / tool-result events, so cross-restart HITL does not drop prior
// context. The current user input must not be duplicated.
func TestContinueFromHITLColdInjectsHistory(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "ticket-agent", System: "helper"})
	msgStore := conversation.NewMemoryStore()
	// Prior conversation turns from earlier runs.
	_, _ = msgStore.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "上一轮问题"})
	_, _ = msgStore.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "上一轮回答"})
	// The API appended the current run's user input before Execute kicked off.
	_, _ = msgStore.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "创建工单"})

	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "create_ticket"}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"id": "9"}, false, nil
	}, true)

	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "创建工单", ConversationID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate persisted waiting_human after a process restart.
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

	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "已创建"}
	}}
	eng := &Engine{
		Store:       st,
		LLM:         llmStub,
		Tools:       reg,
		Gate:        NewGate(), // empty — no in-process waiter, forces cold path
		Messages:    msgStore,
		MaxMessages: 40,
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("invoke count=%d want 1", calls.Load())
	}

	// Expect: system, 上一轮 user, 上一轮 assistant, 本轮 user, assistant(tool_call), tool(result).
	if len(saw) != 6 {
		t.Fatalf("expected 6 messages, got %d: %+v", len(saw), saw)
	}
	if saw[0].Role != llm.RoleSystem || saw[0].Content != "helper" {
		t.Fatalf("saw[0]=%+v", saw[0])
	}
	if saw[1].Role != llm.RoleUser || saw[1].Content != "上一轮问题" {
		t.Fatalf("saw[1]=%+v", saw[1])
	}
	if saw[2].Role != llm.RoleAssistant || saw[2].Content != "上一轮回答" {
		t.Fatalf("saw[2]=%+v", saw[2])
	}
	if saw[3].Role != llm.RoleUser || saw[3].Content != "创建工单" {
		t.Fatalf("saw[3]=%+v", saw[3])
	}
	if saw[4].Role != llm.RoleAssistant || len(saw[4].ToolCalls) != 1 || saw[4].ToolCalls[0].Name != "create_ticket" {
		t.Fatalf("saw[4]=%+v", saw[4])
	}
	if saw[5].Role != llm.RoleTool {
		t.Fatalf("saw[5]=%+v", saw[5])
	}

	// The current user input must appear exactly once.
	var userCount int
	for _, m := range saw {
		if m.Role == llm.RoleUser && m.Content == "创建工单" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("expected exactly one 创建工单 user message, got %d", userCount)
	}

	// The cold resume should also have written the terminal assistant message.
	msgs := msgStore.List("c1")
	var terminal *conversation.Message
	for i := range msgs {
		if msgs[i].Role == conversation.RoleAssistant && msgs[i].RunID == r.ID {
			terminal = &msgs[i]
			break
		}
	}
	if terminal == nil || terminal.Content != "已创建" {
		t.Fatalf("missing terminal assistant message; msgs=%+v", msgs)
	}

	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}
}

// TestContinueFromHITLColdNoMessageStore: when Messages is nil, the cold resume
// path falls back to the legacy behavior (system + user input + event replay)
// without injecting any conversation history.
func TestContinueFromHITLColdNoMessageStore(t *testing.T) {
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

	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "已创建"}
	}}
	eng := &Engine{
		Store: st,
		LLM:   llmStub,
		Tools: reg,
		Gate:  NewGate(),
		// Messages intentionally nil.
	}
	if err := eng.ContinueFromHITL(context.Background(), r.ID, Decision{Approve: true}); err != nil {
		t.Fatalf("ContinueFromHITL: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("invoke count=%d want 1", calls.Load())
	}
	// Expect: system, user input, assistant(tool_call), tool(result).
	if len(saw) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(saw), saw)
	}
	if saw[0].Role != llm.RoleSystem || saw[1].Role != llm.RoleUser || saw[1].Content != "创建工单" {
		t.Fatalf("legacy header mismatch: saw[0]=%+v saw[1]=%+v", saw[0], saw[1])
	}
	if saw[2].Role != llm.RoleAssistant || len(saw[2].ToolCalls) != 1 {
		t.Fatalf("saw[2]=%+v", saw[2])
	}
	if saw[3].Role != llm.RoleTool {
		t.Fatalf("saw[3]=%+v", saw[3])
	}
}

func TestBuildMessagesAfterTruncateAndResend(t *testing.T) {
	msgStore := conversation.NewMemoryStore()
	u1, _ := msgStore.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "u1"})
	_, _ = msgStore.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "a1"})
	u2, _ := msgStore.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "u2"})
	_, _ = msgStore.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "a2"})

	if _, err := msgStore.TruncateFrom("c1", u2.ID); err != nil {
		t.Fatal(err)
	}
	_, _ = msgStore.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "u2-edited", RunID: "run_new"})

	eng := &Engine{Messages: msgStore, MaxMessages: 40}
	got := eng.buildMessages("sys", "c1", "u2-edited", nil)
	if len(got) != 4 {
		t.Fatalf("len=%d want 4; got=%+v", len(got), got)
	}
	if got[1].Content != "u1" || got[2].Content != "a1" || got[3].Content != "u2-edited" {
		t.Fatalf("unexpected window: %+v", got)
	}
	for _, m := range got {
		if m.Content == "u2" || m.Content == "a2" {
			t.Fatalf("rolled-back content leaked: %+v", got)
		}
	}
	_ = u1
}

func TestRecordTerminalMessageSkipsSupersededRun(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	u1, _ := msgStore.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "u1"})
	run, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "u1", ConversationID: "c1"})
	if err := msgStore.SetRunID(u1.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := msgStore.TruncateFrom("c1", u1.ID); err != nil {
		t.Fatal(err)
	}
	_ = st.UpdateRun(run.ID, store.StatusSucceeded, "ghost assistant", "")

	eng := &Engine{Store: st, Messages: msgStore}
	eng.recordTerminalMessage(run.ID)

	if len(msgStore.List("c1")) != 0 {
		t.Fatalf("expected no assistant append after rollback; got %+v", msgStore.List("c1"))
	}
}
