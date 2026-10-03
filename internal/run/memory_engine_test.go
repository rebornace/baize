package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestMemoryInjectSameOwnerAcrossConversations(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_ = msgStore.EnsureMeta(conversation.Meta{ID: "conv-a", OwnerID: "alice", Source: "ui"})
	_ = msgStore.EnsureMeta(conversation.Meta{ID: "conv-b", OwnerID: "alice", Source: "ui"})
	mem := memory.NewMemoryStore()
	if _, err := mem.Upsert(memory.Entry{OwnerID: "alice", Text: "喜欢绿茶", Source: memory.SourceExplicit}); err != nil {
		t.Fatal(err)
	}

	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "推荐龙井"}
	}}
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4,
		Messages: msgStore, Meta: msgStore, Memory: mem,
		Settings: fakeKnobs{k: runtimecfg.Knobs{MemoryEnabled: true, MemoryAutoExtract: false}},
	}
	// Rank requires query as substring of entry text (no Chinese tokenization).
	input := "绿茶"
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: input, ConversationID: "conv-b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, input); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, m := range saw {
		if m.Role == llm.RoleSystem &&
			strings.Contains(m.Content, "长期记忆") &&
			strings.Contains(m.Content, "喜欢绿茶") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected memory block with 喜欢绿茶 in LLM messages; got %+v", saw)
	}
	if len(saw) < 2 || saw[0].Content != "sys" || saw[1].Role != llm.RoleSystem || !strings.Contains(saw[1].Content, "长期记忆") {
		t.Fatalf("memory block must follow system prompt; saw=%+v", saw)
	}
}

func TestMemoryInjectSkipsDifferentOwner(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_ = msgStore.EnsureMeta(conversation.Meta{ID: "conv-alice", OwnerID: "alice", Source: "ui"})
	_ = msgStore.EnsureMeta(conversation.Meta{ID: "conv-bob", OwnerID: "bob", Source: "ui"})
	mem := memory.NewMemoryStore()
	if _, err := mem.Upsert(memory.Entry{OwnerID: "alice", Text: "喜欢绿茶", Source: memory.SourceExplicit}); err != nil {
		t.Fatal(err)
	}

	// Query must Rank-hit alice's entry text so a false isolation pass is impossible.
	input := "绿茶"
	settings := fakeKnobs{k: runtimecfg.Knobs{MemoryEnabled: true, MemoryAutoExtract: false}}

	var sawAlice []llm.Message
	engAlice := &Engine{
		Store: st, LLM: &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
			sawAlice = append([]llm.Message(nil), msgs...)
			return llm.Message{Role: llm.RoleAssistant, Content: "好的"}
		}}, Tools: tool.NewRegistry(), MaxSteps: 4,
		Messages: msgStore, Meta: msgStore, Memory: mem, Settings: settings,
	}
	rAlice, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: input, ConversationID: "conv-alice"})
	if err != nil {
		t.Fatal(err)
	}
	if err := engAlice.Execute(context.Background(), rAlice.ID, agent.Def{ID: "a", System: "sys"}, input); err != nil {
		t.Fatal(err)
	}
	aliceInjected := false
	for _, m := range sawAlice {
		if m.Role == llm.RoleSystem && strings.Contains(m.Content, "长期记忆") && strings.Contains(m.Content, "喜欢绿茶") {
			aliceInjected = true
			break
		}
	}
	if !aliceInjected {
		t.Fatalf("control: alice Run must inject own memory with query %q; saw=%+v", input, sawAlice)
	}

	var sawBob []llm.Message
	engBob := &Engine{
		Store: st, LLM: &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
			sawBob = append([]llm.Message(nil), msgs...)
			return llm.Message{Role: llm.RoleAssistant, Content: "嗯"}
		}}, Tools: tool.NewRegistry(), MaxSteps: 4,
		Messages: msgStore, Meta: msgStore, Memory: mem, Settings: settings,
	}
	rBob, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: input, ConversationID: "conv-bob"})
	if err != nil {
		t.Fatal(err)
	}
	if err := engBob.Execute(context.Background(), rBob.ID, agent.Def{ID: "a", System: "sys"}, input); err != nil {
		t.Fatal(err)
	}
	for _, m := range sawBob {
		if strings.Contains(m.Content, "喜欢绿茶") || strings.Contains(m.Content, "长期记忆") {
			t.Fatalf("bob Run must not inject alice memory (query %q would Rank-hit); saw=%+v", input, sawBob)
		}
	}
}

func TestMemoryAutoExtractOnSucceeded(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_ = msgStore.EnsureMeta(conversation.Meta{ID: "conv-a", OwnerID: "alice", Source: "ui"})
	mem := memory.NewMemoryStore()

	calls := 0
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		calls++
		if calls == 1 {
			return llm.Message{Role: llm.RoleAssistant, Content: "好的，已记下你喜欢绿茶"}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: `[{"key":"tea","text":"喜欢绿茶"}]`}
	}}
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4,
		Messages: msgStore, Meta: msgStore, Memory: mem,
		Settings: fakeKnobs{k: runtimecfg.Knobs{MemoryEnabled: true, MemoryAutoExtract: true}},
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "请记住我喜欢绿茶", ConversationID: "conv-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "请记住我喜欢绿茶"); err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("expected extract Chat call; calls=%d", calls)
	}
	list, err := mem.List("alice", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range list {
		if e.Source == memory.SourceAuto && strings.Contains(e.Text, "喜欢绿茶") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected auto memory entry; list=%+v", list)
	}
}

func TestMemoryDisabledSkipsInjectAndExtract(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_ = msgStore.EnsureMeta(conversation.Meta{ID: "conv-a", OwnerID: "alice", Source: "ui"})
	mem := memory.NewMemoryStore()
	if _, err := mem.Upsert(memory.Entry{OwnerID: "alice", Text: "喜欢绿茶", Source: memory.SourceExplicit}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		calls++
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "好的"}
	}}
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4,
		Messages: msgStore, Meta: msgStore, Memory: mem,
		Settings: fakeKnobs{k: runtimecfg.Knobs{MemoryEnabled: false, MemoryAutoExtract: true}},
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "绿茶", ConversationID: "conv-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "绿茶"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("MemoryEnabled=false must not call extract Chat; calls=%d", calls)
	}
	for _, m := range saw {
		if strings.Contains(m.Content, "长期记忆") || strings.Contains(m.Content, "喜欢绿茶") {
			t.Fatalf("MemoryEnabled=false must not inject; saw=%+v", saw)
		}
	}
	list, _ := mem.List("alice", 20, 0)
	for _, e := range list {
		if e.Source == memory.SourceAuto {
			t.Fatalf("MemoryEnabled=false must not auto-extract; list=%+v", list)
		}
	}
}
