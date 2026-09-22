package run

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// toolShadowKnobs builds knobs with DP-2a enabled in shadow mode.
func toolShadowKnobs(threshold int) fakeKnobs {
	return fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled:            true,
		DecideToolRoutingEnabled: true,
		DecideToolShadow:         true,
		DecideToolThreshold:      threshold,
		DecideToolTopK:           8,
	}}
}

func newShadowEngine(t *testing.T, nTools int, decider decide.Ask, settings fakeKnobs) (*Engine, *store.Run, *int) {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	noop := func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}
	for i := 0; i < nTools; i++ {
		name := "tool_" + strconv.Itoa(i)
		reg.RegisterSpec(llm.ToolSpec{Name: name, Description: "does thing " + strconv.Itoa(i)}, noop)
	}
	sent := 0
	llmStub := &captureLLM{onChat: func(_ []llm.Message, tools []llm.ToolSpec) llm.Message {
		sent = len(tools)
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "thing"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store: st, LLM: llmStub, Tools: reg, Decider: decider, Settings: settings,
	}
	return eng, r, &sent
}

// AC-06: shadow mode must record the layer's pick but still send the full tool
// set to the model (zero behavior change).
func TestToolShadowSendsFullSetAndRecordsEvent(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{
		Verdict: decide.VerdictYes,
		Values:  []string{"tool_0", "tool_1"},
		Source:  decide.SourceRules,
	}}
	eng, r, sent := newShadowEngine(t, 15, decider, toolShadowKnobs(12))

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	if *sent != 15 {
		t.Fatalf("full tool set must still be sent in shadow, sent=%d want 15", *sent)
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	var shadow *store.Event
	for i := range evs {
		if evs[i].Type == EventDecideToolShadow {
			shadow = &evs[i]
		}
	}
	if shadow == nil {
		t.Fatalf("missing decide.tool_shadow event; evs=%+v", evs)
	}
	if asInt(shadow.Data, "total") != 15 {
		t.Fatalf("total=%v want 15", shadow.Data["total"])
	}
	if asInt(shadow.Data, "turn") != 0 {
		t.Fatalf("turn=%v want 0", shadow.Data["turn"])
	}
	kept, _ := shadow.Data["kept"].([]string)
	if len(kept) != 2 || kept[0] != "tool_0" || kept[1] != "tool_1" {
		t.Fatalf("kept=%v want [tool_0 tool_1]", shadow.Data["kept"])
	}
	if asInt(shadow.Data, "kept_count") != 2 {
		t.Fatalf("kept_count=%v want 2", shadow.Data["kept_count"])
	}
	if shadow.Data["source"] != decide.SourceRules {
		t.Fatalf("source=%v want rules", shadow.Data["source"])
	}
}

// AC-09: when the tool count is at or below the threshold the layer must not be
// consulted and no shadow event is written.
func TestToolShadowSkipsBelowThreshold(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{
		Values: []string{"tool_0"}, Source: decide.SourceRules,
	}}
	eng, r, sent := newShadowEngine(t, 10, decider, toolShadowKnobs(12))

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "go"},
	}); err != nil {
		t.Fatal(err)
	}
	if *sent != 10 {
		t.Fatalf("sent=%d want 10", *sent)
	}
	if len(decider.got.Options) != 0 {
		t.Fatalf("decider must not be consulted below threshold, options=%v", decider.got.Options)
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventDecideToolShadow {
			t.Fatalf("no shadow event expected below threshold; ev=%+v", ev)
		}
	}
}

// The deterministic prefilter must narrow the options handed to the decision
// model to DecideToolPreTopK, and the shadow event records that prefilter set
// separately from the model's final kept pick.
func TestToolShadowPrefilterNarrowsCandidates(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{
		Verdict: decide.VerdictYes,
		Values:  []string{"tool_10"},
		Source:  decide.SourceRules,
	}}
	knobs := fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled:            true,
		DecideToolRoutingEnabled: true,
		DecideToolShadow:         true,
		DecideToolThreshold:      12,
		DecideToolTopK:           8,
		DecideToolPreTopK:        5,
	}}
	eng, r, _ := newShadowEngine(t, 20, decider, knobs)

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	// The decision model only saw the prefiltered five.
	if len(decider.got.Options) != 5 {
		t.Fatalf("decider options=%d want 5: %v", len(decider.got.Options), decider.got.Options)
	}
	validOpts := make(map[string]bool)
	for i := 0; i < 20; i++ {
		validOpts["tool_"+strconv.Itoa(i)] = true
	}
	uniq := make(map[string]bool)
	for _, o := range decider.got.Options {
		if !validOpts[o] {
			t.Fatalf("option %q is not a registered tool", o)
		}
		if uniq[o] {
			t.Fatalf("duplicate prefilter option %q", o)
		}
		uniq[o] = true
	}

	evs, _ := eng.Store.ListEvents(r.ID)
	var shadow *store.Event
	for i := range evs {
		if evs[i].Type == EventDecideToolShadow {
			shadow = &evs[i]
		}
	}
	if shadow == nil {
		t.Fatalf("missing shadow event")
	}
	if asInt(shadow.Data, "prefilter_count") != 5 {
		t.Fatalf("prefilter_count=%v want 5", shadow.Data["prefilter_count"])
	}
	if asInt(shadow.Data, "total") != 20 {
		t.Fatalf("total=%v want 20", shadow.Data["total"])
	}
	pre, _ := shadow.Data["prefilter"].([]string)
	if len(pre) != 5 || pre[0] != "tool_0" {
		t.Fatalf("prefilter=%v want first five", shadow.Data["prefilter"])
	}
	kept, _ := shadow.Data["kept"].([]string)
	if len(kept) != 1 || kept[0] != "tool_10" {
		t.Fatalf("kept=%v want [tool_10]", shadow.Data["kept"])
	}
}

func TestToolShadowDisabledByDefault(t *testing.T) {
	decider := &stubDecider{ans: decide.Answer{
		Values: []string{"tool_0"}, Source: decide.SourceRules,
	}}
	off := fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled: false, DecideToolRoutingEnabled: true,
		DecideToolThreshold: 12,
	}}
	eng, r, _ := newShadowEngine(t, 15, decider, off)

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "go"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(decider.got.Options) != 0 {
		t.Fatalf("decider must not run when master switch off")
	}
}

// The probe must carry only names and the first description line, never the
// input JSON schema, and list all candidate tools as options.
func TestBuildToolProbeShape(t *testing.T) {
	specs := []llm.ToolSpec{
		{Name: "alpha", Description: "first line\nsecond line", InputSchema: map[string]any{"x": 1}},
		{Name: "beta", Description: "beta tool", InputSchema: map[string]any{"y": 2}},
	}
	probe := buildToolProbe(specs)
	if !strings.Contains(probe, "- alpha: first line") {
		t.Fatalf("probe missing alpha first line: %q", probe)
	}
	if strings.Contains(probe, "second line") {
		t.Fatalf("probe must drop description after first line: %q", probe)
	}
	if strings.Contains(probe, "InputSchema") || strings.Contains(probe, "\"x\"") {
		t.Fatalf("probe must not include input schema: %q", probe)
	}
	if !strings.Contains(probe, "- beta: beta tool") {
		t.Fatalf("probe missing beta: %q", probe)
	}
}
