package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
)

// fakeCompactLLM returns a canned summary and records the messages it received.
type fakeCompactLLM struct {
	reply string
	got   []llm.Message
}

func (f *fakeCompactLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	f.got = msgs
	return llm.Message{Role: llm.RoleAssistant, Content: f.reply}, nil
}
func (f *fakeCompactLLM) SupportsVision() bool { return false }

// fakeProfiles is a minimal llm.ProfileSource.
type fakeProfiles struct{ def llm.ModelProfileView }

func (f fakeProfiles) DefaultModelProfile() (llm.ModelProfileView, error) { return f.def, nil }
func (f fakeProfiles) ModelProfileByID(id string) (llm.ModelProfileView, error) {
	return f.def, nil
}

func seedConv(t *testing.T, ms conversation.Store, convID string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		role := conversation.RoleUser
		if i%2 == 1 {
			role = conversation.RoleAssistant
		}
		ms.Append(convID, conversation.Message{Role: role, Content: strings.Repeat("内容", 100)})
	}
}

func TestMaybeCompactNoProfileSkips(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 50)
	// ProfileSource returns an empty view (ID==""): mock/demo path with no
	// configured profile -> compaction disabled entirely.
	c := &Compactor{Messages: ms, LLM: &fakeCompactLLM{reply: "摘要"},
		Profiles:  fakeProfiles{def: llm.ModelProfileView{}},
		Threshold: 0.8, ReserveTokens: 8000, KeepRecent: 8}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || changed {
		t.Fatalf("no profile must skip: changed=%v err=%v", changed, err)
	}
	if _, ok := ms.GetRollingSummary("c"); ok {
		t.Fatal("no summary when profile missing")
	}
}

func TestMaybeCompactUnderThresholdSkips(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 4)
	c := &Compactor{Messages: ms, LLM: &fakeCompactLLM{reply: "摘要"},
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 100000}},
		Threshold: 0.8, ReserveTokens: 8000, KeepRecent: 8}
	// budget = 100000*0.8-8000 = 72000；4 条消息（约 800 token）远低于此
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || changed {
		t.Fatalf("under threshold must skip: changed=%v err=%v", changed, err)
	}
	if _, ok := ms.GetRollingSummary("c"); ok {
		t.Fatal("should not compact under threshold")
	}
}

func TestMaybeCompactTriggersAndFolds(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 40) // 40 条，每条 200 个 CJK（内容x100）≈ 大量 token
	llmF := &fakeCompactLLM{reply: "这是滚动摘要"}
	c := &Compactor{
		Messages: ms, LLM: llmF,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold: 0.7, ReserveTokens: 400, KeepRecent: 8,
		SummaryTimeout: 0, // 0 用默认
	}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || !changed {
		t.Fatalf("expected compaction: changed=%v err=%v", changed, err)
	}
	sum, ok := ms.GetRollingSummary("c")
	if !ok || sum.Summary != "这是滚动摘要" {
		t.Fatalf("summary not persisted: %+v ok=%v", sum, ok)
	}
	// 游标应覆盖到 recent 窗口之前那条；recent=8，40 条 => 折叠 0..31，cursor order=31
	if sum.CoversThroughOrder != 31 {
		t.Fatalf("cursor order = %d, want 31", sum.CoversThroughOrder)
	}
	// 摘要 LLM 必须收到了转录文本
	if len(llmF.got) < 2 {
		t.Fatalf("expected system + transcript messages, got %d", len(llmF.got))
	}
}

func TestMaybeCompactIncrementalExtendsCursor(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 40)
	c := &Compactor{
		Messages: ms, LLM: &fakeCompactLLM{reply: "摘要一"},
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold: 0.7, ReserveTokens: 400, KeepRecent: 8,
	}
	if _, err := c.MaybeCompact(context.Background(), "c", nil, "p"); err != nil {
		t.Fatal(err)
	}
	first, _ := ms.GetRollingSummary("c")
	// 再来 20 条，recent 窗口下移，应增量折叠
	seedConv(t, ms, "c", 20)
	c.LLM = &fakeCompactLLM{reply: "摘要二"}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || !changed {
		t.Fatalf("expected incremental compaction: changed=%v err=%v", changed, err)
	}
	second, _ := ms.GetRollingSummary("c")
	if second.CoversThroughOrder <= first.CoversThroughOrder {
		t.Fatalf("cursor must advance: %d -> %d", first.CoversThroughOrder, second.CoversThroughOrder)
	}
}
