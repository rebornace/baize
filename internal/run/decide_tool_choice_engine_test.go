package run

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
)

// choiceLLM implements Chooser + StreamChooser, recording which constrained
// method the engine used. It returns plain content so the loop ends in one step.
type choiceLLM struct {
	streamPicked int32
	plainPicked  int32
	unconPicked  int32
}

func (c *choiceLLM) Chat(context.Context, []llm.Message, []llm.ToolSpec) (llm.Message, error) {
	atomic.AddInt32(&c.unconPicked, 1)
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}
func (c *choiceLLM) SupportsVision() bool { return false }
func (c *choiceLLM) ChatWithChoice(_ context.Context, _ []llm.Message, tools []llm.ToolSpec, _ llm.ToolChoice) (llm.Message, error) {
	atomic.AddInt32(&c.plainPicked, 1)
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}
func (c *choiceLLM) ChatStreamWithChoice(
	_ context.Context, _ []llm.Message, _ []llm.ToolSpec, _ llm.ToolChoice, _, _ func(string),
) (llm.Message, error) {
	atomic.AddInt32(&c.streamPicked, 1)
	return llm.Message{Role: llm.RoleAssistant, Content: "done"}, nil
}

func choiceEnforceKnobs(choice bool) fakeKnobs {
	return fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled:            true,
		DecideToolRoutingEnabled: true,
		DecideToolShadow:         false, // enforce
		DecideToolThreshold:      12,
		DecideToolTopK:           8,
		DecideToolPreTopK:        5,
		DecideToolChoiceEnabled:  choice,
	}}
}

// When DP-2b is enabled in enforce, the engine must use the streaming
// constrained call and never the unconstrained Chat.
func TestEngineUsesChoiceWhenEnabled(t *testing.T) {
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Verdict: decide.VerdictYes, Values: []string{"sys_a"}, Source: decide.SourceRemote},
	}}
	eng, r, _ := newShadowEngine(t, 40, decider, choiceEnforceKnobs(true), "thing 3", "sys_a")
	cLLM := &choiceLLM{}
	eng.LLM = cLLM

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "thing 3"},
	}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&cLLM.streamPicked) != 1 {
		t.Fatalf("streaming constrained call count=%d want 1", cLLM.streamPicked)
	}
	if atomic.LoadInt32(&cLLM.unconPicked) != 0 {
		t.Fatalf("unconstrained Chat must not run, count=%d", cLLM.unconPicked)
	}
	if atomic.LoadInt32(&cLLM.plainPicked) != 0 {
		t.Fatalf("plain constrained call must not run when streaming succeeds, count=%d", cLLM.plainPicked)
	}

	evs, _ := eng.Store.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventDecideToolShadow {
			if b, _ := ev.Data["tool_choice"].(bool); !b {
				t.Fatalf("tool_choice=%v want true", ev.Data["tool_choice"])
			}
		}
	}
}

// When DP-2b is off (even in enforce), the ordinary streamer path is used.
func TestEngineSkipsChoiceWhenDisabled(t *testing.T) {
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Verdict: decide.VerdictYes, Values: []string{"sys_a"}, Source: decide.SourceRemote},
	}}
	eng, r, _ := newShadowEngine(t, 40, decider, choiceEnforceKnobs(false), "thing 3", "sys_a")
	cLLM := &choiceLLM{}
	eng.LLM = cLLM

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "thing 3"},
	}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&cLLM.streamPicked) != 0 {
		t.Fatalf("constrained call must not run, count=%d", cLLM.streamPicked)
	}
	if atomic.LoadInt32(&cLLM.unconPicked) != 1 {
		t.Fatalf("unconstrained Chat count=%d want 1", cLLM.unconPicked)
	}
}
