package llm

import (
	"strings"

	"github.com/rebornace/baize/internal/store"
)

// AutoProfileID is the sentinel value for the "smart routing (Auto)" choice.
// It is NOT a real model profile id: a turn tagged with it lets the router pick
// a concrete model based on the turn's content (task difficulty + image/vision
// needs). The interactive chat exposes it as an explicit "智能路由 / Auto"
// option; channel inbound (WeChat etc.) has no manual picker and is always Auto.
const AutoProfileID = "auto"

// RoutingProfile is the subset of a stored model profile the router needs. It
// is defined here (rather than taking store.ModelProfile) so the routing logic
// stays easy to unit-test in isolation.
type RoutingProfile struct {
	ID             string
	SupportsVision bool
	// Tier is one of store.AutoTier*; normalize via store.NormalizeAutoTier.
	Tier string
}

// ProfileSelection is the outcome of resolving which model serves a turn.
type ProfileSelection struct {
	// ProfileID is the model profile to pin the run to. "" means no profile
	// was selected (no profiles configured, or an image turn with no vision
	// model); callers surface a friendly error / degrade.
	ProfileID string
	// Auto reports whether the choice was made by the auto router (vs. an
	// explicit manual profile selection).
	Auto bool
}

// TaskSignals are the deterministic, per-turn inputs used to judge task
// difficulty. They are intentionally cheap: no extra LLM call, no latency.
type TaskSignals struct {
	// Text is the user's message text (already stripped of skill mentions at
	// the web entry point; raw inbound text for channels).
	Text string
	// HasImages reports the turn actually carries image content.
	HasImages bool
	// FileCount is the number of non-image attachments (docx/pdf/code/…).
	FileCount int
	// HasCode reports a fenced code block / code-like payload in the text.
	HasCode bool
}

// DetectCode heuristically reports whether the text embeds a code block (a
// fenced ``` block). It is deliberately cheap and conservative.
func DetectCode(text string) bool {
	return strings.Contains(text, "```")
}

// NormalizeProfileChoice maps a raw per-run model choice to either the Auto
// sentinel or a concrete profile id. An empty/unset choice means Auto (the
// interactive default and the only mode for channel inbound).
func NormalizeProfileChoice(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, AutoProfileID) {
		return AutoProfileID
	}
	return raw
}

// InferTier guesses a model's capability tier from its model/profile name for
// the "auto-suggest on create" default. Administrators can always override it.
// Unknown names resolve to the standard tier.
func InferTier(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return store.AutoTierStandard
	}
	// Strong/reasoning models. Checked before light so names like "o3-mini"
	// classify as reasoning-capable.
	for _, sub := range []string{
		"reasoning", "thinking", "deepseek-r", "o1", "o3", "o4", "r1",
		"opus", "ultra", "-pro", "pro-", "-max", "-reason",
	} {
		if strings.Contains(n, sub) {
			return store.AutoTierPower
		}
	}
	// Fast / cheap models.
	for _, sub := range []string{
		"mini", "nano", "micro", "flash", "haiku", "small", "lite", "speed",
	} {
		if strings.Contains(n, sub) {
			return store.AutoTierLight
		}
	}
	return store.AutoTierStandard
}

// Task classification thresholds (rune counts). Tunable heuristics live here so
// policy is concentrated in one place.
const (
	lightMaxRunes    = 80
	powerLongRunes   = 4000
	powerReasonRunes = 150 // a reasoning cue plus at least this much text
	powerCodeRunes   = 60  // a reasoning cue plus code/files at this length
	powerMinFiles    = 3
)

// reasoningKeywords hint at a non-trivial analytical task. Both Chinese and
// English cues are included.
var reasoningKeywords = []string{
	"分析", "规划", "推理", "设计", "重构", "排查", "调试", "优化", "架构", "方案",
	"为什么", "怎么解决", "如何实现", "诊断", "评估", "权衡", "根因",
	"analyze", "analyse", "reason", "plan", "design", "refactor", "architect",
	"optimize", "optimise", "investigate", "diagnose", "debug", "root cause",
	"trade-off", "tradeoff", "implement", "compare",
}

// ClassifyTask maps the actual turn content to a desired capability tier. It
// never errors and never calls an LLM: pure deterministic heuristics.
func ClassifyTask(sig TaskSignals) string {
	text := strings.TrimSpace(sig.Text)
	rlen := len([]rune(text))
	low := strings.ToLower(text)
	// Detect embedded code blocks from the text even when the caller did not
	// set HasCode explicitly (keeps classification self-contained).
	hasCode := sig.HasCode || DetectCode(text)

	hasReasonWord := false
	for _, kw := range reasoningKeywords {
		if strings.Contains(low, kw) {
			hasReasonWord = true
			break
		}
	}

	// Hard / long-context tasks -> power tier.
	if rlen >= powerLongRunes {
		return store.AutoTierPower
	}
	if sig.FileCount >= powerMinFiles {
		return store.AutoTierPower
	}
	if hasReasonWord && rlen >= powerReasonRunes {
		return store.AutoTierPower
	}
	if hasReasonWord && (hasCode || sig.FileCount > 0) && rlen >= powerCodeRunes {
		return store.AutoTierPower
	}

	// Simple chit-chat with no attachments/code -> light tier.
	if rlen > 0 && rlen <= lightMaxRunes && !sig.HasImages && sig.FileCount == 0 && !hasCode && !hasReasonWord {
		return store.AutoTierLight
	}
	return store.AutoTierStandard
}

// tierFallbacks defines which tiers to try (in order) when the ideal tier has
// no available model. Prefer the closest neighbor over jumping to the extreme.
func tierFallbacks(tier string) []string {
	switch tier {
	case store.AutoTierPower:
		return []string{store.AutoTierPower, store.AutoTierStandard, store.AutoTierLight}
	case store.AutoTierLight:
		return []string{store.AutoTierLight, store.AutoTierStandard, store.AutoTierPower}
	default:
		return []string{store.AutoTierStandard, store.AutoTierPower, store.AutoTierLight}
	}
}

// pickByTier selects the first profile matching the desired tier (with
// fallback), optionally restricted to vision-capable models. Profiles are
// assumed already ordered by creation time (stable tiebreak).
func pickByTier(profiles []RoutingProfile, tier string, requireVision bool) (id string, matched bool) {
	for _, want := range tierFallbacks(tier) {
		for _, p := range profiles {
			if requireVision && !p.SupportsVision {
				continue
			}
			if store.NormalizeAutoTier(p.Tier) == want {
				return p.ID, true
			}
		}
	}
	return "", false
}

// ResolveModel decides which model serves a turn.
//
// choice is the normalized per-run choice (AutoProfileID or a concrete id).
// sig carries the turn's content signals; profiles is the current set of
// configured model profiles (ordered by creation time for tiebreaks).
//
// Semantics:
//   - Manual (concrete) choice: always honored exactly and NEVER silently
//     rerouted. visionOK reports whether that specific profile supports vision;
//     when false on an image turn the caller warns the user. An unknown id is
//     returned as-is so the Switch falls back to its first profile.
//   - Auto: classify the task to a tier, then pick the best available model in
//     that tier (with neighbor fallback). Image turns only consider
//     vision-capable models; with none available ProfileID is "" and
//     visionOK=false (the caller degrades / warns). With no profiles at all
//     ProfileID is "" and the caller prompts to add a model.
func ResolveModel(choice string, sig TaskSignals, profiles []RoutingProfile) (sel ProfileSelection, visionOK bool) {
	if choice != "" && choice != AutoProfileID {
		// Manual choice is honored exactly and never rerouted. visionOK is only
		// meaningful on an image turn: a non-image turn is always fine.
		ok := !sig.HasImages || profileSupportsVision(choice, profiles)
		return ProfileSelection{ProfileID: choice, Auto: false}, ok
	}

	desired := ClassifyTask(sig)
	requireVision := sig.HasImages
	id, _ := pickByTier(profiles, desired, requireVision)
	if requireVision {
		// No vision model at all: caller degrades images to a text note / warns.
		return ProfileSelection{ProfileID: id, Auto: true}, id != ""
	}
	return ProfileSelection{ProfileID: id, Auto: true}, true
}

// RoutingProfilesFrom converts stored profiles to the compact router form.
func RoutingProfilesFrom(list []store.ModelProfile) []RoutingProfile {
	out := make([]RoutingProfile, 0, len(list))
	for _, p := range list {
		out = append(out, RoutingProfile{
			ID:             p.ID,
			SupportsVision: p.SupportsVision,
			Tier:           store.NormalizeAutoTier(p.AutoTier),
		})
	}
	return out
}

func profileSupportsVision(id string, profiles []RoutingProfile) bool {
	for _, p := range profiles {
		if p.ID == id {
			return p.SupportsVision
		}
	}
	return false
}
