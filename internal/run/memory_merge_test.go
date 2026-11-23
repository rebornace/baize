package run

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
)

func mergeTestEngine(t *testing.T, extractJSON string, calls *int) (*Engine, store.Store, memory.Store, string) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_ = msgStore.EnsureMeta(conversation.Meta{ID: "conv-a", OwnerID: "alice", Source: "ui"})
	mem := memory.NewMemoryStore()
	llmStub := &captureLLM{onChat: func([]llm.Message, []llm.ToolSpec) llm.Message {
		*calls++
		return llm.Message{Role: llm.RoleAssistant, Content: extractJSON}
	}}
	eng := &Engine{
		Store: st, LLM: llmStub, Memory: mem,
		Messages: msgStore, Meta: msgStore,
		Settings: fakeKnobs{k: runtimecfg.Knobs{MemoryEnabled: true, MemoryAutoExtract: true}},
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "in", ConversationID: "conv-a"})
	if err != nil {
		t.Fatal(err)
	}
	return eng, st, mem, r.ID
}

func hasSkipReason(t *testing.T, st store.Store, runID, reason string) bool {
	t.Helper()
	events, err := st.ListEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		if ev.Type != EventMemoryExtractSkipped {
			continue
		}
		if r, _ := ev.Data["reason"].(string); r == reason {
			return true
		}
	}
	return false
}

func TestAutoExtractSkipsWhenExplicitInRun(t *testing.T) {
	calls := 0
	eng, st, mem, runID := mergeTestEngine(t, `[{"key":"k1","text":"x"}]`, &calls)
	if err := st.AppendEvent(runID, store.Event{
		Type: "tool.result",
		Data: map[string]any{"name": memory.RememberName, "is_error": false},
	}); err != nil {
		t.Fatal(err)
	}
	eng.maybeExtractMemory(context.Background(), runID, "alice", "in", "out")
	if calls != 0 {
		t.Fatalf("extraction LLM must not be called for explicit_in_run, calls=%d", calls)
	}
	if !hasSkipReason(t, st, runID, "explicit_in_run") {
		t.Fatal("missing explicit_in_run skip event")
	}
	items, _ := mem.List("alice", 10, 0)
	if len(items) != 0 {
		t.Fatalf("no auto entries expected: %+v", items)
	}
}

func TestAutoExtractSkipsDuplicateText(t *testing.T) {
	calls := 0
	eng, st, mem, runID := mergeTestEngine(t, `[{"key":"k2","text":"喜欢 绿茶"}]`, &calls)
	if _, err := mem.Upsert(memory.Entry{
		OwnerID: "alice", Key: "k1", Text: "喜欢  绿茶", Source: memory.SourceExplicit,
	}); err != nil {
		t.Fatal(err)
	}
	eng.maybeExtractMemory(context.Background(), runID, "alice", "in", "out")
	if !hasSkipReason(t, st, runID, "duplicate_text") {
		t.Fatal("missing duplicate_text skip event")
	}
	items, _ := mem.List("alice", 10, 0)
	if len(items) != 1 || items[0].Key != "k1" {
		t.Fatalf("duplicate must not be inserted: %+v", items)
	}
}

func TestAutoExtractCapsAtThree(t *testing.T) {
	calls := 0
	eng, st, mem, runID := mergeTestEngine(t,
		`[{"key":"k1","text":"a"},{"key":"k2","text":"b"},{"key":"k3","text":"c"},{"key":"k4","text":"d"},{"key":"k5","text":"e"}]`,
		&calls)
	eng.maybeExtractMemory(context.Background(), runID, "alice", "in", "out")
	items, _ := mem.List("alice", 10, 0)
	if len(items) != 3 {
		t.Fatalf("want 3 auto entries, got %d: %+v", len(items), items)
	}
	if !hasSkipReason(t, st, runID, "cap_reached") {
		t.Fatal("missing cap_reached skip event")
	}
	events, _ := st.ListEvents(runID)
	for _, ev := range events {
		if ev.Type == EventMemoryExtractSkipped {
			if r, _ := ev.Data["reason"].(string); r == "cap_reached" {
				if n, _ := ev.Data["count"].(int); n != 2 {
					t.Fatalf("cap count=%v want 2", ev.Data["count"])
				}
			}
		}
	}
}

func TestAutoExtractSkipsNearDuplicate(t *testing.T) {
	calls := 0
	eng, st, mem, runID := mergeTestEngine(t,
		`[{"key":"survey_reporting_format","text":"统计问卷默认按「完成人数」口径汇报，不按配额进度，输出中文表格。"}]`,
		&calls)
	if _, err := mem.Upsert(memory.Entry{
		OwnerID: "alice",
		Key:     "preference.survey_reporting",
		Text:    "统计问卷时默认按完成人数汇报，不按配额进度；输出中文表格。",
		Source:  memory.SourceExplicit,
	}); err != nil {
		t.Fatal(err)
	}
	eng.maybeExtractMemory(context.Background(), runID, "alice", "in", "out")
	if !hasSkipReason(t, st, runID, "near_duplicate") {
		t.Fatal("missing near_duplicate skip event")
	}
	items, _ := mem.List("alice", 10, 0)
	if len(items) != 1 || items[0].Key != "preference.survey_reporting" {
		t.Fatalf("near duplicate must not be inserted: %+v", items)
	}
}

func TestAutoExtractKeepsDifferentNumbers(t *testing.T) {
	calls := 0
	eng, st, mem, runID := mergeTestEngine(t,
		`[{"key":"link_ttl","text":"链接有效期7天"}]`, &calls)
	if _, err := mem.Upsert(memory.Entry{
		OwnerID: "alice", Key: "link_ttl_old", Text: "链接有效期30天", Source: memory.SourceAuto,
	}); err != nil {
		t.Fatal(err)
	}
	eng.maybeExtractMemory(context.Background(), runID, "alice", "in", "out")
	if hasSkipReason(t, st, runID, "near_duplicate") {
		t.Fatal("different numbers must not be treated as near-duplicate")
	}
	items, _ := mem.List("alice", 10, 0)
	if len(items) != 2 {
		t.Fatalf("new entry with different numbers must be written: %+v", items)
	}
}
