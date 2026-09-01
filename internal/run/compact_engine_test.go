package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestBuildMessagesInjectsRollingSummary(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms, MaxMessages: 40}
	conv := "c1"
	for i := 0; i < 6; i++ {
		role := conversation.RoleUser
		if i%2 == 1 {
			role = conversation.RoleAssistant
		}
		ms.Append(conv, conversation.Message{Role: role, Content: "旧消息内容"})
	}
	ms.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID: conv, Summary: "此前对话：用户在做报销系统",
		CoversThroughMessageID: "x", CoversThroughOrder: 1,
	})
	msgs := e.buildMessages("系统提示", conv, "现在的问题", nil)
	if msgs[0].Role != "system" || msgs[0].Content != "系统提示" {
		t.Fatalf("first message must be the real system prompt: %+v", msgs[0])
	}
	found := false
	for _, m := range msgs {
		if m.Role == "user" && strings.Contains(m.Content, "此前对话：用户在做报销系统") {
			found = true
		}
	}
	if !found {
		t.Fatalf("rolling summary block not injected; got %d messages", len(msgs))
	}
	// 摘要块必须在历史消息之前（紧跟 system 提示）
	if !strings.Contains(msgs[1].Content, "此前对话：用户在做报销系统") {
		t.Fatalf("summary block must directly follow the system prompt; msgs[1]=%+v", msgs[1])
	}
	// 最后一条必须是当前输入
	if msgs[len(msgs)-1].Content != "现在的问题" {
		t.Fatalf("last message must be current input, got %q", msgs[len(msgs)-1].Content)
	}
}

func TestBuildMessagesNoSummaryUnchanged(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms, MaxMessages: 40}
	ms.Append("c", conversation.Message{Role: conversation.RoleUser, Content: "你好"})
	msgs := e.buildMessages("sys", "c", "在吗", nil)
	for _, m := range msgs {
		if strings.Contains(m.Content, "滚动摘要") {
			t.Fatal("must not inject summary block when none exists")
		}
	}
}

func TestBuildMessagesBlankSummaryNotInjected(t *testing.T) {
	ms := conversation.NewMemoryStore()
	e := &Engine{Messages: ms, MaxMessages: 40}
	ms.Append("c", conversation.Message{Role: conversation.RoleUser, Content: "你好"})
	if err := ms.UpsertRollingSummary(conversation.RollingSummary{
		ConversationID: "c", Summary: "   ",
	}); err != nil {
		t.Fatal(err)
	}
	msgs := e.buildMessages("sys", "c", "在吗", nil)
	for _, m := range msgs {
		if strings.Contains(m.Content, "滚动摘要") {
			t.Fatal("must not inject a blank/whitespace summary block")
		}
	}
}

// recordingLLM is a main-LLM stub that records the prompt from each Chat call
// and always returns a final assistant reply.
type recordingLLM struct{ prompts [][]llm.Message }

func (r *recordingLLM) Chat(_ context.Context, msgs []llm.Message, _ []llm.ToolSpec) (llm.Message, error) {
	r.prompts = append(r.prompts, append([]llm.Message(nil), msgs...))
	return llm.Message{Role: llm.RoleAssistant, Content: "本轮回答"}, nil
}
func (r *recordingLLM) SupportsVision() bool { return false }

// newCompactEngine builds an engine whose conversation store is shared with the
// compactor, wired with a recording main LLM.
func newCompactEngine(t *testing.T, sumLLM llm.Provider, profiles llm.ProfileSource) (*Engine, *store.Memory, *conversation.MemoryStore, *recordingLLM) {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	ms := conversation.NewMemoryStore()
	mainLLM := &recordingLLM{}
	eng := &Engine{
		Store: st, LLM: mainLLM, Tools: tool.NewRegistry(), MaxSteps: 4,
		Messages: ms, MaxMessages: 40,
		Compactor: &Compactor{
			Messages: ms, LLM: sumLLM, Profiles: profiles,
			Threshold: 0.7, ReserveTokens: 400, KeepRecent: 8,
		},
	}
	return eng, st, ms, mainLLM
}

// promptSawSummary reports whether the captured main-LLM prompt contains a
// rolling-summary block carrying the given text.
func promptSawSummary(mainLLM *recordingLLM, text string) bool {
	for _, p := range mainLLM.prompts {
		for _, m := range p {
			if m.Role == llm.RoleUser && strings.Contains(m.Content, text) {
				return true
			}
		}
	}
	return false
}

func eventTypes(evs []store.Event) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ev.Type)
	}
	return out
}

func containsType(types []string, want string) bool {
	for _, ty := range types {
		if ty == want {
			return true
		}
	}
	return false
}

// TestExecuteCompactionEmitsCompactedEvent: when compaction folds history
// before a run, the engine records EventContextCompacted and the persisted
// summary is injected into the prompt the main LLM receives.
var errCompactBoom = errors.New("boom")

func TestExecuteCompactionEmitsCompactedEvent(t *testing.T) {
	sumLLM := &fakeCompactLLM{reply: "这是滚动摘要"}
	profiles := fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}}
	eng, st, ms, mainLLM := newCompactEngine(t, sumLLM, profiles)
	seedConv(t, ms, "conv1", 40) // long history + tiny context => compaction fires

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "本轮问题", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "本轮问题"); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsType(eventTypes(evs), EventContextCompacted) {
		t.Fatalf("expected context.compacted event, got %v", eventTypes(evs))
	}
	sum, ok := ms.GetRollingSummary("conv1")
	if !ok || sum.Summary != "这是滚动摘要" {
		t.Fatalf("summary not persisted: %+v ok=%v", sum, ok)
	}
	// The persisted summary must be injected into the prompt the MAIN llm saw.
	if !promptSawSummary(mainLLM, "这是滚动摘要") {
		t.Fatalf("main LLM prompt did not include the rolling summary block: %+v", mainLLM.prompts)
	}
}

// TestExecuteCompactionFailureIsBestEffort: a failing summarizer must not block
// the reply; the run still succeeds and an llm.error event records the skip.
func TestExecuteCompactionFailureIsBestEffort(t *testing.T) {
	sumLLM := &fakeCompactLLM{err: errCompactBoom}
	profiles := fakeProfiles{def: llm.ModelProfileView{ID: "p", ContextTokens: 4000}}
	eng, st, ms, _ := newCompactEngine(t, sumLLM, profiles)
	seedConv(t, ms, "conv1", 40)

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "本轮问题", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "本轮问题"); err != nil {
		t.Fatalf("compaction failure must not fail the run: %v", err)
	}
	rec, err := st.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != store.StatusSucceeded {
		t.Fatalf("run must still succeed, status=%s err=%q", rec.Status, rec.Error)
	}
	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sawSkip bool
	for _, ev := range evs {
		if ev.Type == EventLLMError {
			if msg, _ := ev.Data["error"].(string); strings.Contains(msg, "context compaction skipped") {
				sawSkip = true
			}
		}
	}
	if !sawSkip {
		t.Fatalf("expected a best-effort llm.error event, got %v", eventTypes(evs))
	}
	if containsType(eventTypes(evs), EventContextCompacted) {
		t.Fatal("must not emit context.compacted when summarization failed")
	}
}

// TestExecuteWithoutCompactorSkipsCompaction: with Compactor == nil the run
// behaves exactly as before (no compaction events).
func TestExecuteWithoutCompactorSkipsCompaction(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	ms := conversation.NewMemoryStore()
	seedConv(t, ms, "conv1", 40)
	mainLLM := &captureLLM{onChat: func(_ []llm.Message, _ []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "本轮回答"}
	}}
	eng := &Engine{Store: st, LLM: mainLLM, Tools: tool.NewRegistry(), MaxSteps: 4,
		Messages: ms, MaxMessages: 40} // Compactor left nil

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "本轮问题", ConversationID: "conv1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "本轮问题"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	evs, err := st.ListEvents(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if containsType(eventTypes(evs), EventContextCompacted) {
		t.Fatal("must not emit context.compacted when Compactor is nil")
	}
	if _, ok := ms.GetRollingSummary("conv1"); ok {
		t.Fatal("must not persist a summary when Compactor is nil")
	}
}
