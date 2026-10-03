package run

import (
	"strings"

	"github.com/rebornace/baize/internal/llm"
)

// decideProbeMaxRunes bounds the context sent to the decision layer. The gate
// call must never be more expensive than the extraction it tries to save, so
// the probe is aggressively truncated.
const decideProbeMaxRunes = 1500

// effectiveDecideMemory reports whether DP-1 is live: the decision master
// switch and the memory decision switch are both on. With no Settings (many
// test Engines) it stays off, preserving legacy behavior.
func (e *Engine) effectiveDecideMemory() bool {
	if e.Settings == nil || e.Decider == nil {
		return false
	}
	k := e.Settings.Knobs()
	return k.DecideEnabled && k.DecideMemoryEnabled
}

// effectiveDecideTool reports whether DP-2a is live (shadow or enforce): the
// master switch and the tool-routing switch are both on. With no Settings or
// Decider it stays off, preserving legacy behavior.
func (e *Engine) effectiveDecideTool() bool {
	if e.Settings == nil || e.Decider == nil {
		return false
	}
	k := e.Settings.Knobs()
	return k.DecideEnabled && k.DecideToolRoutingEnabled
}

// effectiveDecideToolThreshold returns the tool-count threshold above which
// the decision layer is consulted. Zero means "not configured" and is treated
// as the spec default of 12 so a partially-configured holder still behaves
// sanely.
func (e *Engine) effectiveDecideToolThreshold() int {
	if e.Settings == nil {
		return 12
	}
	if t := e.Settings.Knobs().DecideToolThreshold; t > 0 {
		return t
	}
	return 12
}

// effectiveDecideToolChoice reports whether DP-2b is live: send the narrowed
// tool set with tool_choice=required so the main model must pick one of them.
// Guarded by the same master + routing switches as DP-2a.
func (e *Engine) effectiveDecideToolChoice() bool {
	if e.Settings == nil || e.Decider == nil {
		return false
	}
	k := e.Settings.Knobs()
	return k.DecideEnabled &&
		k.DecideToolRoutingEnabled &&
		k.DecideToolChoiceEnabled
}

// effectiveDecidePreTopK returns the deterministic keyword-prefilter width:
// how many candidates survive keyword matching before the decision model picks.
// Zero/missing is treated as the spec default of 32.
func (e *Engine) effectiveDecidePreTopK() int {
	// Default 16: the empirically validated cost/success sweet spot (see
	// runtime_settings.go). Used only when knobs are unset/unavailable.
	if e.Settings == nil {
		return 16
	}
	if k := e.Settings.Knobs().DecideToolPreTopK; k > 0 {
		return k
	}
	return 16
}

// buildExtractProbe assembles the (truncated) context for the worth-extracting
// judgment. It mirrors the shape of the extraction payload but is capped.
func buildExtractProbe(input, output string) string {
	user := "用户输入：\n" + strings.TrimSpace(input) + "\n\n助手回复：\n" + strings.TrimSpace(output)
	r := []rune(user)
	if len(r) > decideProbeMaxRunes {
		user = string(r[:decideProbeMaxRunes])
	}
	return user
}

// estimateExtractionInputTokens approximates the input-token cost of the
// extraction call a skip avoids, using the exact two-message shape that call
// would have sent (system prompt + assembled user payload, including the
// per-message structural overhead). It is an estimate (no tokenizer), so the
// UI labels it as such; it deliberately excludes completion tokens since the
// whole call never happens.
func estimateExtractionInputTokens(input, output string) int {
	user := "用户输入：\n" + strings.TrimSpace(input) + "\n\n助手回复：\n" + strings.TrimSpace(output)
	return EstimateMessagesTokens([]llm.Message{
		{Role: llm.RoleSystem, Content: memoryExtractSystem},
		{Role: llm.RoleUser, Content: user},
	})
}
