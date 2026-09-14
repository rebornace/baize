package run

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// streamLLM implements Streamer: turn0 thinking T0 + tool_call; turn1 thinking T1 + content 答.
type streamLLM struct{ calls int }

func (s *streamLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{}, fmt.Errorf("Chat must not be used when ChatStream succeeds")
}

func (s *streamLLM) ChatStream(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec, onThink, onContent func(cumulative string)) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		if onThink != nil {
			onThink("T0")
		}
		return llm.Message{
			Role:     llm.RoleAssistant,
			Thinking: "T0",
			ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "create_ticket", Arguments: map[string]any{"title": "x"}},
			},
		}, nil
	}
	if onThink != nil {
		onThink("T1")
	}
	if onContent != nil {
		onContent("答")
	}
	return llm.Message{Role: llm.RoleAssistant, Thinking: "T1", Content: "答"}, nil
}

func (s *streamLLM) SupportsVision() bool { return false }

// chatOnlyThinkingLLM is Chat-only (no Streamer) and returns Thinking on the final message.
type chatOnlyThinkingLLM struct{}

func (c *chatOnlyThinkingLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	return llm.Message{Role: llm.RoleAssistant, Content: "答", Thinking: "想"}, nil
}

func (c *chatOnlyThinkingLLM) SupportsVision() bool { return false }

// streamFailLLM returns a stream error containing "status 400", then Chat succeeds.
type streamFailLLM struct {
	streamCalls int
	chatCalls   int
}

func (s *streamFailLLM) ChatStream(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec, onThink, onContent func(cumulative string)) (llm.Message, error) {
	s.streamCalls++
	return llm.Message{}, fmt.Errorf("openai_compatible: status 400: stream not supported")
}

func (s *streamFailLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.chatCalls++
	return llm.Message{Role: llm.RoleAssistant, Content: "答", Thinking: "回退想"}, nil
}

func (s *streamFailLLM) SupportsVision() bool { return false }

func TestEngineStreamThinkingEventsAndPersist(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	})
	msgStore := conversation.NewMemoryStore()
	ag := agent.Def{ID: "a", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "创建工单", ConversationID: "conv1", ThinkingLevel: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "创建工单", RunID: r.ID})

	eng := &Engine{Store: st, LLM: &streamLLM{}, Tools: reg, Messages: msgStore, MaxSteps: 8}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	joined := llm.JoinThinking([]string{"T0", "T1"})
	if !eventSeqContains(evs,
		eventMatch{Type: EventLLMThinkingDelta, Turn: 0, Text: "T0"},
		eventMatch{Type: EventLLMThinking, Turn: 0, Text: "T0"},
		eventMatch{Type: EventLLMToolCall},
		eventMatch{Type: EventLLMThinkingDelta, Turn: 1, Text: "T1"},
		eventMatch{Type: EventLLMContentDelta, Turn: 1, Text: "答"},
		eventMatch{Type: EventLLMThinking, Turn: 1, Text: "T1"},
		eventMatch{Type: EventLLMMessage, Content: "答", Thinking: joined},
	) {
		t.Fatalf("event sequence mismatch; events=%s", summarizeEvents(evs))
	}

	msgs := msgStore.List("conv1")
	var assistant *conversation.Message
	for i := range msgs {
		if msgs[i].Role == conversation.RoleAssistant {
			assistant = &msgs[i]
		}
	}
	if assistant == nil {
		t.Fatal("missing assistant message")
	}
	if assistant.Thinking != joined {
		t.Fatalf("assistant Thinking=%q want %q", assistant.Thinking, joined)
	}
	if assistant.Content != "答" {
		t.Fatalf("assistant Content=%q", assistant.Content)
	}
}

func TestEngineChatOnlyThinkingNoDeltas(t *testing.T) {
	st := store.NewMemory()
	msgStore := conversation.NewMemoryStore()
	ag := agent.Def{ID: "a", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "问", ConversationID: "conv1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "问", RunID: r.ID})

	eng := &Engine{Store: st, LLM: &chatOnlyThinkingLLM{}, Tools: tool.NewRegistry(), Messages: msgStore, MaxSteps: 4}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.Type == EventLLMThinkingDelta || ev.Type == EventLLMContentDelta {
			t.Fatalf("Chat-only path must not emit deltas; got %s", ev.Type)
		}
	}
	var msgEv *store.Event
	for i := range evs {
		if evs[i].Type == EventLLMMessage {
			msgEv = &evs[i]
		}
	}
	if msgEv == nil {
		t.Fatal("missing llm.message")
	}
	if asString(msgEv.Data["content"]) != "答" {
		t.Fatalf("content=%q", msgEv.Data["content"])
	}
	if asString(msgEv.Data["thinking"]) != "想" {
		t.Fatalf("thinking=%q want 想", msgEv.Data["thinking"])
	}
}

func TestEngineStreamFallbackToChat(t *testing.T) {
	st := store.NewMemory()
	msgStore := conversation.NewMemoryStore()
	ag := agent.Def{ID: "a", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "问", ConversationID: "conv1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "问", RunID: r.ID})

	stub := &streamFailLLM{}
	eng := &Engine{Store: st, LLM: stub, Tools: tool.NewRegistry(), Messages: msgStore, MaxSteps: 4}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stub.streamCalls != 1 || stub.chatCalls != 1 {
		t.Fatalf("streamCalls=%d chatCalls=%d want 1,1", stub.streamCalls, stub.chatCalls)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range evs {
		if ev.Type == EventLLMThinkingDelta || ev.Type == EventLLMContentDelta {
			t.Fatalf("fallback must not emit deltas; got %s", ev.Type)
		}
	}
	var msgEv *store.Event
	for i := range evs {
		if evs[i].Type == EventLLMMessage {
			msgEv = &evs[i]
		}
	}
	if msgEv == nil {
		t.Fatal("missing llm.message")
	}
	if asString(msgEv.Data["content"]) != "答" || asString(msgEv.Data["thinking"]) != "回退想" {
		t.Fatalf("llm.message data=%v", msgEv.Data)
	}
}

type eventMatch struct {
	Type      string
	Turn      int
	Text      string
	Content   string
	Thinking  string
	checkTurn bool
	checkText bool
	checkMsg  bool
}

func (m eventMatch) withChecks() eventMatch {
	if m.Type == EventLLMMessage {
		m.checkMsg = true
	}
	if m.Text != "" || m.Type == EventLLMThinking || m.Type == EventLLMThinkingDelta || m.Type == EventLLMContentDelta {
		m.checkText = m.Text != ""
		m.checkTurn = true
	}
	if m.Type == EventLLMToolCall {
		m.checkTurn = false
		m.checkText = false
	}
	return m
}

func eventSeqContains(evs []store.Event, want ...eventMatch) bool {
	i := 0
	for _, w := range want {
		w = w.withChecks()
		found := false
		for ; i < len(evs); i++ {
			ev := evs[i]
			if ev.Type != w.Type {
				continue
			}
			if w.checkTurn {
				turn, ok := ev.Data["turn"].(int)
				if !ok {
					// JSON / store may use float64
					if f, okf := ev.Data["turn"].(float64); okf {
						turn = int(f)
						ok = true
					}
				}
				if !ok || turn != w.Turn {
					continue
				}
			}
			if w.checkText && asString(ev.Data["text"]) != w.Text {
				continue
			}
			if w.checkMsg {
				if asString(ev.Data["content"]) != w.Content {
					continue
				}
				if asString(ev.Data["thinking"]) != w.Thinking {
					continue
				}
			}
			found = true
			i++
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func summarizeEvents(evs []store.Event) string {
	var b strings.Builder
	for _, ev := range evs {
		fmt.Fprintf(&b, "%s%v; ", ev.Type, ev.Data)
	}
	return b.String()
}
