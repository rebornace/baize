package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
)

// fakeCompactLLM returns a canned summary and records the messages it received.
type fakeCompactLLM struct {
	reply string
	err   error
	got   []llm.Message
}

func (f *fakeCompactLLM) Chat(ctx context.Context, msgs []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	f.got = msgs
	if f.err != nil {
		return llm.Message{}, f.err
	}
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
	llmF := &fakeCompactLLM{reply: "摘要"}
	c := &Compactor{Messages: ms, LLM: llmF,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{}},
		Threshold: 0.8, ReserveTokens: 8000, KeepRecent: 8}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || changed {
		t.Fatalf("no profile must skip: changed=%v err=%v", changed, err)
	}
	if len(llmF.got) != 0 {
		t.Fatal("no profile must not call the summary LLM")
	}
	if _, ok := ms.GetRollingSummary("c"); ok {
		t.Fatal("no summary when profile missing")
	}
}

func TestMaybeCompactUnderThresholdSkips(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 4)
	llmF := &fakeCompactLLM{reply: "摘要"}
	c := &Compactor{Messages: ms, LLM: llmF,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 100000}},
		Threshold: 0.8, ReserveTokens: 8000, KeepRecent: 8}
	// budget = 100000*0.8-8000 = 72000；4 条消息（约 800 token）远低于此
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || changed {
		t.Fatalf("under threshold must skip: changed=%v err=%v", changed, err)
	}
	if len(llmF.got) != 0 {
		t.Fatal("under threshold must not call the summary LLM")
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
	// 游标消息 ID 必须指向被折叠区间的最后一条（全历史第 32 条，索引 31）
	if wantID := ms.List("c")[31].ID; sum.CoversThroughMessageID != wantID {
		t.Fatalf("cursor message id = %q, want %q", sum.CoversThroughMessageID, wantID)
	}
	// 摘要 LLM 必须收到了转录文本
	if len(llmF.got) < 2 {
		t.Fatalf("expected system + transcript messages, got %d", len(llmF.got))
	}
}

func TestMaybeCompactSummaryFailureNoDirtySummary(t *testing.T) {
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "c", 40) // 与触发用例相同的长对话 + 小 context，必然触发压缩
	llmF := &fakeCompactLLM{reply: "unused", err: errors.New("boom")}
	c := &Compactor{
		Messages: ms, LLM: llmF,
		Profiles:  fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}},
		Threshold: 0.7, ReserveTokens: 400, KeepRecent: 8,
	}
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if changed {
		t.Fatalf("summary failure must not report changed: changed=%v", changed)
	}
	if err == nil {
		t.Fatal("summary failure must return the error")
	}
	// LLM 确实被调用了（阈值已触发），但失败后不得留下脏摘要
	if len(llmF.got) == 0 {
		t.Fatal("expected the summary LLM to be called before failing")
	}
	if _, ok := ms.GetRollingSummary("c"); ok {
		t.Fatal("failed summarization must not persist a rolling summary")
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
	llm2 := &fakeCompactLLM{reply: "摘要二"}
	c.LLM = llm2
	changed, err := c.MaybeCompact(context.Background(), "c", nil, "p")
	if err != nil || !changed {
		t.Fatalf("expected incremental compaction: changed=%v err=%v", changed, err)
	}
	second, _ := ms.GetRollingSummary("c")
	if second.CoversThroughOrder <= first.CoversThroughOrder {
		t.Fatalf("cursor must advance: %d -> %d", first.CoversThroughOrder, second.CoversThroughOrder)
	}
	// 共 60 条、KeepRecent=8 => keepStart=52，cursor order=51
	if second.CoversThroughOrder != 51 {
		t.Fatalf("second cursor order = %d, want 51", second.CoversThroughOrder)
	}
	// 第二轮必须走增量整合：user 消息以「已有摘要」开头，而非全量重摘
	if len(llm2.got) < 2 {
		t.Fatalf("expected system + transcript messages on second fold, got %d", len(llm2.got))
	}
	if !strings.HasPrefix(llm2.got[1].Content, "已有摘要") {
		t.Fatalf("second fold must build on prior summary; user message prefix = %q", llm2.got[1].Content)
	}
}
