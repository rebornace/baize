package llm

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestNormalizeProfileChoice(t *testing.T) {
	cases := map[string]string{
		"":      AutoProfileID,
		"  ":    AutoProfileID,
		"auto":  AutoProfileID,
		"AUTO":  AutoProfileID,
		" Auto": AutoProfileID,
		"mp_x":  "mp_x",
	}
	for in, want := range cases {
		if got := NormalizeProfileChoice(in); got != want {
			t.Errorf("NormalizeProfileChoice(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInferTier(t *testing.T) {
	cases := map[string]string{
		"":                 store.AutoTierStandard,
		"gpt-4o":           store.AutoTierStandard,
		"gpt-4o-mini":      store.AutoTierLight,
		"gemini-2.0-flash": store.AutoTierLight,
		"claude-3-haiku":   store.AutoTierLight,
		"nano-foo":         store.AutoTierLight,
		"o1":               store.AutoTierPower,
		"o3-mini":          store.AutoTierPower, // reasoning cue wins over mini
		"deepseek-r1":      store.AutoTierPower,
		"claude-opus":      store.AutoTierPower,
		"gemini-2.5-pro":   store.AutoTierPower,
		"QwQ-32B":          store.AutoTierStandard,
	}
	for name, want := range cases {
		if got := InferTier(name); got != want {
			t.Errorf("InferTier(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestClassifyTask(t *testing.T) {
	cases := []struct {
		name string
		sig  TaskSignals
		want string
	}{
		{"short greeting is light", TaskSignals{Text: "你好"}, store.AutoTierLight},
		{"short english is light", TaskSignals{Text: "hi there"}, store.AutoTierLight},
		{"image short is standard not light", TaskSignals{Text: "看看这张图", HasImages: true}, store.AutoTierStandard},
		{"plain medium text is standard", TaskSignals{Text: strings.Repeat("普通一句话。", 20)}, store.AutoTierStandard},
		{"many files is power", TaskSignals{Text: "帮我看看", FileCount: 3}, store.AutoTierPower},
		{"very long input is power", TaskSignals{Text: strings.Repeat("字", 4500)}, store.AutoTierPower},
		{"reason keyword + length is power", TaskSignals{Text: "请分析这个方案" + strings.Repeat("内", 400)}, store.AutoTierPower},
		{"reason keyword alone short is standard", TaskSignals{Text: "分析一下"}, store.AutoTierStandard},
		{"english reason + long is power", TaskSignals{Text: "please refactor this module carefully" + strings.Repeat("x", 300)}, store.AutoTierPower},
		{"code present without reason word is standard", TaskSignals{Text: "```go\nfmt.Println(1)\n```"}, store.AutoTierStandard},
		{"reason + code is power", TaskSignals{Text: "帮我重构这段代码，要更清晰" + strings.Repeat("x", 100) + "```go\nx:=1\n```", HasCode: true}, store.AutoTierPower},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyTask(tc.sig); got != tc.want {
				t.Fatalf("ClassifyTask = %q, want %q", got, tc.want)
			}
		})
	}
}

func tiered() []RoutingProfile {
	return []RoutingProfile{
		{ID: "mp_light", Tier: store.AutoTierLight},
		{ID: "mp_standard", Tier: store.AutoTierStandard},
		{ID: "mp_power", Tier: store.AutoTierPower},
		{ID: "mp_vision", Tier: store.AutoTierStandard, SupportsVision: true},
	}
}

func TestResolveModelManualIsHonored(t *testing.T) {
	profiles := tiered()
	sel, ok := ResolveModel("mp_standard", TaskSignals{Text: "你好"}, profiles)
	if sel.ProfileID != "mp_standard" || sel.Auto || !ok {
		t.Fatalf("manual text = %+v ok=%v, want mp_standard/auto=false/ok=true", sel, ok)
	}
	// Manual text-only model on an image turn is honored but reports !visionOK.
	sel, ok = ResolveModel("mp_standard", TaskSignals{Text: "看图", HasImages: true}, profiles)
	if sel.ProfileID != "mp_standard" || sel.Auto || ok {
		t.Fatalf("manual image text-model = %+v ok=%v, want mp_standard/auto=false/ok=false", sel, ok)
	}
	// Manual vision model on an image turn is ok.
	sel, ok = ResolveModel("mp_vision", TaskSignals{Text: "看图", HasImages: true}, profiles)
	if sel.ProfileID != "mp_vision" || !ok {
		t.Fatalf("manual vision = %+v ok=%v", sel, ok)
	}
}

func TestResolveModelAutoByTaskTier(t *testing.T) {
	profiles := tiered()
	cases := []struct {
		sig  TaskSignals
		want string
	}{
		{TaskSignals{Text: "你好"}, "mp_light"},
		{TaskSignals{Text: strings.Repeat("普通", 60)}, "mp_standard"},
		{TaskSignals{Text: strings.Repeat("字", 4500)}, "mp_power"},
	}
	for _, tc := range cases {
		sel, ok := ResolveModel(AutoProfileID, tc.sig, profiles)
		if !sel.Auto || !ok {
			t.Fatalf("auto should set auto=true/ok=true, got %+v ok=%v", sel, ok)
		}
		if sel.ProfileID != tc.want {
			t.Fatalf("sig=%+v routed to %q, want %q", tc.sig, sel.ProfileID, tc.want)
		}
	}
}

func TestResolveModelAutoTierFallback(t *testing.T) {
	// Only a light model exists: a power task degrades to light via fallback.
	only := []RoutingProfile{{ID: "mp_light", Tier: store.AutoTierLight}}
	sel, ok := ResolveModel(AutoProfileID, TaskSignals{Text: strings.Repeat("字", 4500)}, only)
	if sel.ProfileID != "mp_light" || !ok {
		t.Fatalf("power task with only light model = %+v ok=%v, want fallback mp_light", sel, ok)
	}
}

func TestResolveModelImageRequiresVision(t *testing.T) {
	// Vision model exists; an image turn must pick the vision one regardless of
	// the task's nominal tier.
	sel, ok := ResolveModel(AutoProfileID, TaskSignals{Text: "看图", HasImages: true}, tiered())
	if sel.ProfileID != "mp_vision" || !ok {
		t.Fatalf("image auto = %+v ok=%v, want mp_vision", sel, ok)
	}
	// No vision model: empty id + visionOK=false.
	noVision := []RoutingProfile{{ID: "mp_standard", Tier: store.AutoTierStandard}}
	sel, ok = ResolveModel(AutoProfileID, TaskSignals{Text: "看图", HasImages: true}, noVision)
	if sel.ProfileID != "" || ok {
		t.Fatalf("image without vision model = %+v ok=%v, want empty/false", sel, ok)
	}
}

func TestResolveModelNoProfiles(t *testing.T) {
	// Non-image turn: visionOK stays true, but the empty id makes the Switch
	// emit its friendly "no model configured" error.
	sel, ok := ResolveModel(AutoProfileID, TaskSignals{Text: "你好"}, nil)
	if sel.ProfileID != "" || !ok || !sel.Auto {
		t.Fatalf("no profiles non-image = %+v ok=%v, want empty id/auto/ok=true", sel, ok)
	}
	// Image turn with no profiles: visionOK=false so the caller warns.
	sel, ok = ResolveModel(AutoProfileID, TaskSignals{Text: "看图", HasImages: true}, nil)
	if sel.ProfileID != "" || ok {
		t.Fatalf("no profiles image = %+v ok=%v, want empty/false", sel, ok)
	}
}

func TestRoutingProfilesFrom(t *testing.T) {
	got := RoutingProfilesFrom([]store.ModelProfile{
		{ID: "a", AutoTier: "", SupportsVision: false},
		{ID: "b", AutoTier: store.AutoTierPower, SupportsVision: true},
	})
	if got[0].Tier != store.AutoTierStandard || got[1].Tier != store.AutoTierPower || !got[1].SupportsVision {
		t.Fatalf("unexpected conversion: %+v", got)
	}
}
