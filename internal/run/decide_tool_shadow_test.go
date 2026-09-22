package run

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/skill"
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

func newShadowEngine(t *testing.T, nTools int, decider decide.Ask, settings fakeKnobs, input, source string) (*Engine, *store.Run, *int) {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	noop := func(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}
	for i := 0; i < nTools; i++ {
		name := "tool_" + strconv.Itoa(i)
		reg.RegisterSpecApproved(llm.ToolSpec{
			Name: name, Description: "does thing " + strconv.Itoa(i), Source: source,
		}, noop, false)
	}
	sent := 0
	llmStub := &captureLLM{onChat: func(_ []llm.Message, tools []llm.ToolSpec) llm.Message {
		sent = len(tools)
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: input})
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
	eng, r, sent := newShadowEngine(t, 15, decider, toolShadowKnobs(12), "thing", "sys_a")

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
	eng, r, sent := newShadowEngine(t, 10, decider, toolShadowKnobs(12), "thing", "sys_a")

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
	eng, r, _ := newShadowEngine(t, 20, decider, knobs, "thing", "sys_a")

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
	eng, r, _ := newShadowEngine(t, 15, decider, off, "thing", "sys_a")

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "go"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(decider.got.Options) != 0 {
		t.Fatalf("decider must not run when master switch off")
	}
}

// enforceKnobs builds knobs with DP-2a enabled in ENFORCE mode (shadow=false).
func enforceKnobs(threshold, preTopK int) fakeKnobs {
	return fakeKnobs{k: runtimecfg.Knobs{
		DecideEnabled:            true,
		DecideToolRoutingEnabled: true,
		DecideToolShadow:         false,
		DecideToolThreshold:      threshold,
		DecideToolTopK:           8,
		DecideToolPreTopK:        preTopK,
	}}
}

// AC-enforce-1: in enforce mode the model must receive ONLY the narrowed
// two-level prefilter set (not the full catalog). The tool-level decision
// model must not be consulted (only the tiny system-routing decision is), so
// no per-tool model tokens are spent. The shadow event records enforce=true.
func TestToolEnforceNarrowsAndSkipsToolModel(t *testing.T) {
	// All fixture tools belong to sys_a. System routing returns it; the
	// tool-level pick must never be asked in enforce.
	decider := &stubDecider{byKind: map[string]decide.Answer{
		decide.KindSystemTargets: {Verdict: decide.VerdictYes, Values: []string{"sys_a"}, Source: decide.SourceRemote},
	}}
	eng, r, sent := newShadowEngine(t, 40, decider, enforceKnobs(12, 5), "thing 3", "sys_a")

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "thing 3"},
	}); err != nil {
		t.Fatal(err)
	}

	if *sent != 5 {
		t.Fatalf("enforce must send only the narrowed set, sent=%d want 5", *sent)
	}
	// System routing was consulted; tool candidates were not.
	sawSystem, sawTool := false, false
	for _, c := range decider.calls {
		switch c.Kind {
		case decide.KindSystemTargets:
			sawSystem = true
			if len(c.Options) != 1 || c.Options[0] != "sys_a" {
				t.Fatalf("system options=%v want [sys_a]", c.Options)
			}
		case decide.KindToolCandidates:
			sawTool = true
		}
	}
	if !sawSystem {
		t.Fatal("system routing must be consulted in enforce")
	}
	if sawTool {
		t.Fatal("tool-level model must not be consulted in enforce")
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	var shadow *store.Event
	for i := range evs {
		if evs[i].Type == EventDecideToolShadow {
			shadow = &evs[i]
		}
	}
	if shadow == nil {
		t.Fatal("missing shadow event")
	}
	if b, _ := shadow.Data["enforce"].(bool); !b {
		t.Fatalf("enforce=%v want true", shadow.Data["enforce"])
	}
	if asInt(shadow.Data, "sent_count") != 5 {
		t.Fatalf("sent_count=%v want 5", shadow.Data["sent_count"])
	}
	if b, _ := shadow.Data["model_consulted"].(bool); b {
		t.Fatalf("model_consulted=%v want false", shadow.Data["model_consulted"])
	}
	if b, _ := shadow.Data["systems_degraded"].(bool); b {
		t.Fatalf("systems_degraded=%v want false", shadow.Data["systems_degraded"])
	}
}

// AC-enforce-2: if keyword matching finds nothing in common with the catalog,
// enforce fails open and sends the full set so the model is never starved.
func TestToolEnforceFailsOpenOnNoMatch(t *testing.T) {
	decider := &stubDecider{}
	eng, r, sent := newShadowEngine(t, 40, decider, enforceKnobs(12, 5), "zzz qqq unrelated gibberish", "sys_a")

	if err := eng.runLoop(context.Background(), r.ID, []llm.Message{
		{Role: llm.RoleUser, Content: "zzz qqq unrelated gibberish"},
	}); err != nil {
		t.Fatal(err)
	}

	if *sent != 40 {
		t.Fatalf("enforce must fail open to full set on no match, sent=%d want 40", *sent)
	}
	evs, _ := eng.Store.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventDecideToolShadow {
			if b, _ := ev.Data["prefilter_empty"].(bool); !b {
				t.Fatalf("prefilter_empty=%v want true", ev.Data["prefilter_empty"])
			}
			if asInt(ev.Data, "sent_count") != 40 {
				t.Fatalf("sent_count=%v want 40", ev.Data["sent_count"])
			}
		}
	}
}

// AC-enforce-3: protected system tools (activate_skill) survive enforce even
// though keyword matching would otherwise drop them. Plain Source="" tools are
// NOT blanket-preserved (they route via keyword prefilter), and a
// connector-owned tool that was filtered stays dropped.
func TestPreserveEssentialTools(t *testing.T) {
	full := []llm.ToolSpec{
		{Name: "builtin_a"},            // Source="" -> not forced
		{Name: skill.ActivateToolName}, // protected -> appended
		{Name: "b", Source: "x"},       // connector tool -> not appended
	}
	picked := []llm.ToolSpec{{Name: "z", Source: "z"}}
	out := preserveEssentialTools(picked, full)
	names := make(map[string]bool, len(out))
	for _, s := range out {
		names[s.Name] = true
	}
	if !names[skill.ActivateToolName] {
		t.Fatalf("activate_skill must be preserved, got %v", names)
	}
	for _, dropped := range []string{"builtin_a", "b"} {
		if names[dropped] {
			t.Fatalf("%q must not be blanket re-added, got %v", dropped, names)
		}
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
