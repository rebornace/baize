package run

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func pruneKnobs(threshold, maxJudged int) fakeKnobs {
	return fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled:            true,
		DecideToolPruneEnabled:   true,
		DecideToolPruneThreshold: threshold,
		DecideToolPruneMaxJudged: maxJudged,
	}}
}

// countDecider records how many times Ask was invoked.
type countDecider struct {
	ans   decide.Answer
	calls int
}

func (d *countDecider) Enabled() bool { return true }

func (d *countDecider) Ask(_ context.Context, _ decide.Question) (decide.Answer, error) {
	d.calls++
	return d.ans, nil
}

// errBoom is a generic implementation failure used to prove fail-open.
var errBoom = errors.New("boom")

// errDecider returns a fixed answer and error.
type errDecider struct {
	ans decide.Answer
	err error
}

func (d *errDecider) Enabled() bool { return true }

func (d *errDecider) Ask(_ context.Context, _ decide.Question) (decide.Answer, error) {
	return d.ans, d.err
}

func bulkyToolMessages(n, size int) []llm.Message {
	msgs := []llm.Message{{Role: llm.RoleAssistant}}
	for i := 0; i < n; i++ {
		msgs = append(msgs, llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: "c" + string(rune('0'+i)),
			Content:    strings.Repeat("x", size),
		})
	}
	return msgs
}

// newPruneRun creates a persisted run and returns its id so AppendEvent works.
func newPruneRun(t *testing.T, st store.Store) string {
	t.Helper()
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

// A non-degraded No replaces the bulky result with a placeholder while keeping
// the Role/ToolCallID, and writes a decide.tool_pruned event.
func TestPruneNoReplacesBulkyResult(t *testing.T) {
	st := store.NewMemory()
	decider := &countDecider{ans: decide.Answer{Verdict: decide.VerdictNo, Source: decide.SourceRules}}
	eng := &Engine{Store: st, Decider: decider, Settings: pruneKnobs(500, 8)}

	runID := newPruneRun(t, st)
	msgs := bulkyToolMessages(1, 2500) // ~625 estimated tokens
	eng.pruneToolResults(context.Background(), runID, msgs)

	if msgs[1].Content != prunedToolPlaceholder {
		t.Fatalf("content=%q want placeholder", msgs[1].Content)
	}
	if msgs[1].Role != llm.RoleTool || msgs[1].ToolCallID != "c0" {
		t.Fatalf("role/id pairing must be preserved: %+v", msgs[1])
	}
	evs, _ := st.ListEvents(runID)
	var pruned *store.Event
	for i := range evs {
		if evs[i].Type == EventDecideToolPruned {
			pruned = &evs[i]
		}
	}
	if pruned == nil {
		t.Fatalf("missing decide.tool_pruned event; evs=%+v", evs)
	}
	if pruned.Data["tool_call_id"] != "c0" {
		t.Fatalf("tool_call_id=%v want c0", pruned.Data["tool_call_id"])
	}
	if asInt(pruned.Data, "saved_tokens") <= 0 {
		t.Fatalf("saved_tokens must be positive: %v", pruned.Data["saved_tokens"])
	}
}

// Fail open: degraded No, raw error, and Yes must all leave the result intact.
func TestPruneFailsOpen(t *testing.T) {
	cases := []struct {
		name string
		ans  decide.Answer
		err  error
	}{
		{"degraded no", decide.Answer{Verdict: decide.VerdictNo, Degraded: true}, nil},
		{"yes", decide.Answer{Verdict: decide.VerdictYes}, nil},
		{"error", decide.Answer{}, errBoom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemory()
			decider := &errDecider{ans: tc.ans, err: tc.err}
			eng := &Engine{Store: st, Decider: decider, Settings: pruneKnobs(500, 8)}

			runID := newPruneRun(t, st)
			msgs := bulkyToolMessages(1, 2500)
			original := msgs[1].Content
			eng.pruneToolResults(context.Background(), runID, msgs)

			if msgs[1].Content != original {
				t.Fatalf("result must be preserved, got placeholder")
			}
			evs, _ := st.ListEvents(runID)
			for _, ev := range evs {
				if ev.Type == EventDecideToolPruned {
					t.Fatalf("no prune event expected on %s; ev=%+v", tc.name, ev)
				}
			}
		})
	}
}

// Sub-threshold tool results are never judged.
func TestPruneSkipsBelowThreshold(t *testing.T) {
	st := store.NewMemory()
	decider := &countDecider{ans: decide.Answer{Verdict: decide.VerdictNo}}
	eng := &Engine{Store: st, Decider: decider, Settings: pruneKnobs(500, 8)}

	msgs := bulkyToolMessages(1, 100) // ~25 tokens
	eng.pruneToolResults(context.Background(), "run_1", msgs)

	if decider.calls != 0 {
		t.Fatalf("decider must not be consulted below threshold, calls=%d", decider.calls)
	}
	if msgs[1].Content != strings.Repeat("x", 100) {
		t.Fatalf("small result must be unchanged")
	}
}

// At most MaxJudged bulky results are judged per turn, largest first.
func TestPruneCapsJudgedCount(t *testing.T) {
	st := store.NewMemory()
	decider := &countDecider{ans: decide.Answer{Verdict: decide.VerdictYes}}
	eng := &Engine{Store: st, Decider: decider, Settings: pruneKnobs(500, 3)}

	msgs := bulkyToolMessages(10, 2500)
	eng.pruneToolResults(context.Background(), "run_1", msgs)

	if decider.calls != 3 {
		t.Fatalf("decider calls=%d want 3 (maxJudged)", decider.calls)
	}
}

// End-to-end through runLoop: a bulky result from turn 1 must be pruned before
// it is sent on turn 2.
func TestRunLoopPrunesBulkyResultNextTurn(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.RegisterSpec(llm.ToolSpec{Name: "big_api"}, func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"data": strings.Repeat("z", 2500)}, false, nil
	})
	var secondTurnTool string
	llmStub := &captureLLM{onChat: func(_ []llm.Message, _ []llm.ToolSpec) llm.Message {
		return llm.Message{Role: llm.RoleAssistant, Content: "first"}
	}}
	turn := 0
	llmStub.onChat = func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		if turn == 0 {
			turn++
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "big_api", Arguments: map[string]any{}},
			}}
		}
		for _, m := range msgs {
			if m.Role == llm.RoleTool {
				secondTurnTool = m.Content
			}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "go"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: reg,
		Decider:  &countDecider{ans: decide.Answer{Verdict: decide.VerdictNo, Source: decide.SourceRules}},
		Settings: pruneKnobs(500, 8),
	}
	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "go"},
	}); err != nil {
		t.Fatal(err)
	}
	if secondTurnTool != prunedToolPlaceholder {
		t.Fatalf("turn-2 tool content=%q want placeholder", secondTurnTool)
	}
}

// The prune probe must be capped.
func TestBuildPruneProbeCapped(t *testing.T) {
	big := llm.Message{Role: llm.RoleTool, Content: strings.Repeat("字", 4000)}
	probe := buildPruneProbe(big)
	body := strings.TrimPrefix(probe, "工具返回内容（判断是否值得在后续上下文中原样保留）：\n")
	if n := len([]rune(body)); n > decideProbeMaxRunes {
		t.Fatalf("probe body runes=%d must be <= %d", n, decideProbeMaxRunes)
	}
}
