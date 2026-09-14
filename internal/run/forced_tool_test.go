package run

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// fatalLLM fails the test if Chat is ever called.
type fatalLLM struct{ t *testing.T }

func (f *fatalLLM) Chat(context.Context, []llm.Message, []llm.ToolSpec) (llm.Message, error) {
	f.t.Fatal("LLM Chat must not be called for ExecuteForcedTool")
	return llm.Message{}, nil
}

func (f *fatalLLM) SupportsVision() bool { return false }

func TestExecuteForcedToolInvokesWithoutLLM(t *testing.T) {
	st := store.NewMemory()
	ids := identity.NewMemoryStore()
	reg := tool.NewRegistry()
	var invoked atomic.Int32
	reg.Register("login", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		invoked.Add(1)
		conv := identity.ConversationIDFrom(ctx)
		_, err := ids.Upsert(conv, identity.Identity{
			Label:             "u",
			Scheme:            "http",
			CredentialHeaders: map[string]string{"Authorization": "Bearer t"},
			Source:            "capture",
			Subject:           "u",
			IsDefault:         true,
		})
		if err != nil {
			return map[string]any{"error": err.Error()}, true, err
		}
		return map[string]any{"ok": true}, false, nil
	})

	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "login", ConversationID: "conv-forced",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(r.ID, store.Event{Type: EventRunStarted})

	eng := &Engine{Store: st, LLM: &fatalLLM{t: t}, Tools: reg, Identities: ids, Gate: NewGate()}
	if err := eng.ExecuteForcedTool(context.Background(), r.ID, "login", map[string]any{"u": "a"}); err != nil {
		t.Fatalf("ExecuteForcedTool: %v", err)
	}

	if invoked.Load() != 1 {
		t.Fatalf("invoke=%d want 1", invoked.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}
	assertEventTypes(t, st, r.ID, EventLLMToolCall, EventToolResult)
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventLLMMessage {
			t.Fatalf("unexpected llm.message: %+v", ev)
		}
	}
	if len(ids.List("conv-forced")) == 0 {
		t.Fatal("expected identity upserted")
	}
}

func TestExecuteForcedToolSkipsLoginGate(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterMeta(tool.Meta{
		Spec: llm.ToolSpec{Name: "login"}, ConnectorID: "c", RequireLogin: true,
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"ok": true}, false, nil
	}, false)

	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "login", ConversationID: "conv-no-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(r.ID, store.Event{Type: EventRunStarted})

	eng := &Engine{
		Store: st, LLM: &fatalLLM{t: t}, Tools: reg, Gate: NewGate(),
		Identities: identity.NewMemoryStore(),
	}
	if err := eng.ExecuteForcedTool(context.Background(), r.ID, "login", map[string]any{"u": "a"}); err != nil {
		t.Fatalf("ExecuteForcedTool: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("invoke=%d want 1 (login gate must be skipped)", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventToolResult {
			if ev.Data["is_error"] == true {
				t.Fatalf("tool.result should not be login_required error: %+v", ev.Data)
			}
		}
	}
}

func TestExecuteForcedToolHITL(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterSpecApproved(llm.ToolSpec{Name: "login"}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"ok": true}, false, nil
	}, true)

	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "login", ConversationID: "conv-hitl",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(r.ID, store.Event{Type: EventRunStarted})

	gate := NewGate()
	eng := &Engine{Store: st, LLM: &fatalLLM{t: t}, Tools: reg, Gate: gate}

	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.ExecuteForcedTool(context.Background(), r.ID, "login", map[string]any{"u": "a"})
	}()

	waitStatus(t, st, r.ID, store.StatusWaitingHuman)
	if calls.Load() != 0 {
		t.Fatalf("tool called before approve: %d", calls.Load())
	}
	if err := gate.Resume(r.ID, Decision{Approve: true, Comment: "ok"}); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ExecuteForcedTool: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ExecuteForcedTool timed out")
	}
	if calls.Load() != 1 {
		t.Fatalf("invoke=%d want 1", calls.Load())
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
	assertEventTypes(t, st, r.ID, EventHITLWaiting, EventHITLResumed, EventToolResult)
}

func TestExecuteForcedToolRedactsArgsInEvent(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("login", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})

	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "login", ConversationID: "conv-redact",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(r.ID, store.Event{Type: EventRunStarted})

	eng := &Engine{Store: st, LLM: &fatalLLM{t: t}, Tools: reg, Gate: NewGate()}
	args := map[string]any{"u": "a", "password": "secret"}
	if err := eng.ExecuteForcedTool(context.Background(), r.ID, "login", args); err != nil {
		t.Fatalf("ExecuteForcedTool: %v", err)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ev := range evs {
		if ev.Type != EventLLMToolCall {
			continue
		}
		found = true
		argMap, _ := ev.Data["arguments"].(map[string]any)
		if argMap["password"] != "***" {
			t.Fatalf("password=%v want ***", argMap["password"])
		}
		if argMap["u"] != "a" {
			t.Fatalf("u=%v want a", argMap["u"])
		}
	}
	if !found {
		t.Fatal("missing llm.tool_call")
	}
	if args["password"] != "secret" {
		t.Fatal("caller args must not be mutated")
	}
}
