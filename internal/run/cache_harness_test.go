package run

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/memory"
)

func TestBuildMessagesKeepsStablePrefixAndLateClock(t *testing.T) {
	ms := conversation.NewMemoryStore()
	if err := ms.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	_, _ = ms.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "旧问题"})
	_, _ = ms.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "旧回答"})

	mem := memory.NewMemoryStore()
	if _, err := mem.Upsert(memory.Entry{
		OwnerID: "alice", Text: "用户喜欢发票按税额汇报", Source: memory.SourceExplicit,
	}); err != nil {
		t.Fatal(err)
	}

	eng := &Engine{Messages: ms, Meta: ms, Memory: mem}
	first := eng.buildMessagesForRun("r1", "你是助手。", "c1", "发票", nil)
	second := eng.buildMessagesForRun("r2", "你是助手。", "c1", "发票总结", nil)

	if len(first) < 5 || len(second) < 5 {
		t.Fatalf("unexpected message counts: %d / %d", len(first), len(second))
	}
	if first[0].Role != "system" || !strings.Contains(first[0].Content, "你是助手。") {
		t.Fatalf("system prefix=%+v", first[0])
	}
	if strings.Contains(first[0].Content, "当前时间：") {
		t.Fatal("clock must not be in the stable system prefix")
	}
	// system + the two history messages must be byte-identical across runs.
	for i := 0; i < 3; i++ {
		if first[i].Role != second[i].Role || first[i].Content != second[i].Content {
			t.Fatalf("prefix changed at %d: %+v vs %+v", i, first[i], second[i])
		}
	}

	var clockAt, memoryAt, historyAt int = -1, -1, -1
	for i, m := range first {
		if m.Role == "assistant" && m.Content == "旧回答" {
			historyAt = i
		}
		if m.Role == "system" && strings.Contains(m.Content, "用户喜欢发票") {
			memoryAt = i
		}
		if m.Role == "system" && strings.Contains(m.Content, "当前时间：") {
			clockAt = i
		}
	}
	if historyAt < 0 || memoryAt <= historyAt || clockAt <= memoryAt {
		t.Fatalf("dynamic context order history=%d memory=%d clock=%d; msgs=%+v", historyAt, memoryAt, clockAt, first)
	}
	last := first[len(first)-1]
	if last.Role != "user" || last.Content != "发票" {
		t.Fatalf("current user must stay last: %+v", last)
	}
}
