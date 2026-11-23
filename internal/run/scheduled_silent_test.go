package run

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type countingChannel struct{ sent int }

func (c *countingChannel) Name() string { return "test" }
func (c *countingChannel) Start(context.Context) error {
	return nil
}
func (c *countingChannel) Stop(context.Context) error { return nil }
func (c *countingChannel) SendText(context.Context, string, string, map[string]string) error {
	c.sent++
	return nil
}
func (c *countingChannel) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}

var _ channel.Channel = (*countingChannel)(nil)

func scheduledTestEngine(t *testing.T, llmStub llm.Provider, origin string) (*Engine, store.Store, *conversation.MemoryStore, *store.Run, *countingChannel) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	ms := conversation.NewMemoryStore()
	if err := ms.EnsureMeta(conversation.Meta{
		ID: "conv-sched", OwnerID: "u", Source: "weixin", ChannelPeer: "peer-1",
	}); err != nil {
		t.Fatal(err)
	}
	ch := &countingChannel{}
	eng := &Engine{Store: st, LLM: llmStub, Messages: ms, Meta: ms, Outbound: ch}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "check", ConversationID: "conv-sched", Origin: origin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Append("conv-sched", conversation.Message{
		Role: conversation.RoleUser, Content: "check", RunID: r.ID,
	}); err != nil {
		t.Fatal(err)
	}
	return eng, st, ms, r, ch
}

func assistantMessageCount(msgs []conversation.Message) int {
	n := 0
	for _, m := range msgs {
		if m.Role == conversation.RoleAssistant {
			n++
		}
	}
	return n
}

func TestScheduledNoUpdateIsSilent(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	_ = now
	llmStub := &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "NO_UPDATE"}
	}}
	eng, st, ms, r, ch := scheduledTestEngine(t, llmStub, store.RunOriginScheduled)
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, r.Input); err != nil {
		t.Fatal(err)
	}

	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded || got.Output != "" {
		t.Fatalf("status=%s output=%q", got.Status, got.Output)
	}
	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventRunSilent) == nil {
		t.Fatal("missing run.silent event")
	}
	if findEvent(evs, EventLLMMessage) != nil {
		t.Fatal("silent run must not persist llm.message")
	}
	if findEvent(evs, EventUnverifiedWrite) != nil {
		t.Fatal("silent run must not trigger write honesty")
	}
	msgs := ms.List("conv-sched")
	if assistantMessageCount(msgs) != 0 {
		t.Fatalf("silent run must not write an assistant message: %+v", msgs)
	}
	if countSystemNotes(msgs, SilentNote) != 1 {
		t.Fatalf("want silent note: %+v", msgs)
	}
	if ch.sent != 0 {
		t.Fatalf("silent run must not deliver outbound, sent=%d", ch.sent)
	}
}

func TestScheduledNoUpdateWithPunctuationIsSilent(t *testing.T) {
	llmStub := &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "  NO_UPDATE。 "}
	}}
	eng, st, _, r, _ := scheduledTestEngine(t, llmStub, store.RunOriginScheduled)
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, r.Input); err != nil {
		t.Fatal(err)
	}
	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventRunSilent) == nil {
		t.Fatal("NO_UPDATE with trailing punctuation must be silent")
	}
}

func TestManualNoUpdateIsNormalReply(t *testing.T) {
	llmStub := &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "NO_UPDATE"}
	}}
	eng, st, ms, r, ch := scheduledTestEngine(t, llmStub, store.RunOriginManual)
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, r.Input); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded || got.Output != "NO_UPDATE" {
		t.Fatalf("status=%s output=%q", got.Status, got.Output)
	}
	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventRunSilent) != nil {
		t.Fatal("manual NO_UPDATE must not be silent")
	}
	if assistantMessageCount(ms.List("conv-sched")) != 1 {
		t.Fatal("manual NO_UPDATE must write a normal assistant message")
	}
	if ch.sent != 1 {
		t.Fatalf("manual reply must be delivered, sent=%d", ch.sent)
	}
}

func TestScheduledNormalReplyIsDelivered(t *testing.T) {
	llmStub := &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "有新增：问卷 11 新增 2 份。"}
	}}
	eng, st, ms, r, ch := scheduledTestEngine(t, llmStub, store.RunOriginScheduled)
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, r.Input); err != nil {
		t.Fatal(err)
	}
	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventRunSilent) != nil {
		t.Fatal("normal scheduled reply must not be silent")
	}
	if assistantMessageCount(ms.List("conv-sched")) != 1 {
		t.Fatal("normal scheduled reply must write an assistant message")
	}
	if ch.sent != 1 {
		t.Fatalf("normal scheduled reply must be delivered, sent=%d", ch.sent)
	}
}

type silentAfterToolLLM struct{ calls int }

func (s *silentAfterToolLLM) Chat(context.Context, []llm.Message, []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "check", Arguments: map[string]any{}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "NO_UPDATE"}, nil
}
func (s *silentAfterToolLLM) SupportsVision() bool { return false }

func TestScheduledSilentAfterToolCall(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register("check", func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	eng, st, ms, r, ch := scheduledTestEngine(t, &silentAfterToolLLM{}, store.RunOriginScheduled)
	eng.Tools = reg
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, r.Input); err != nil {
		t.Fatal(err)
	}
	evs, _ := st.ListEvents(r.ID)
	if findEvent(evs, EventRunSilent) == nil {
		t.Fatal("tool call followed by NO_UPDATE must still be silent")
	}
	if findEvent(evs, EventToolResult) == nil {
		t.Fatal("tool result should be preserved")
	}
	if assistantMessageCount(ms.List("conv-sched")) != 0 {
		t.Fatal("silent run must not write an assistant message")
	}
	if ch.sent != 0 {
		t.Fatalf("silent run must not deliver outbound, sent=%d", ch.sent)
	}
}

func TestScheduledSilentSkipsMemoryExtraction(t *testing.T) {
	calls := 0
	llmStub := &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		calls++
		return llm.Message{Role: llm.RoleAssistant, Content: "NO_UPDATE"}
	}}
	eng, _, _, r, _ := scheduledTestEngine(t, llmStub, store.RunOriginScheduled)
	eng.Memory = memory.NewMemoryStore()
	eng.Settings = fakeKnobs{k: runtimecfg.Knobs{MemoryEnabled: true, MemoryAutoExtract: true}}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, r.Input); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("silent run must not run memory extraction, llm calls=%d", calls)
	}
}

func TestScheduledHintOnlyForScheduledRuns(t *testing.T) {
	eng := &Engine{}
	scheduled := eng.appendScheduleHint("base", &store.Run{Origin: store.RunOriginScheduled}, "zh-CN")
	if !strings.Contains(scheduled, "NO_UPDATE") || !strings.Contains(scheduled, "定时日程") {
		t.Fatalf("missing scheduled hint: %q", scheduled)
	}
	manual := eng.appendScheduleHint("base", &store.Run{Origin: store.RunOriginManual}, "zh-CN")
	if manual != "base" {
		t.Fatalf("manual run must not get scheduled hint: %q", manual)
	}
}
