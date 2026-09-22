package run

import (
	"context"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// prunedToolPlaceholder is the model-facing text substituted for a bulky tool
// result. It preserves the tool-call pairing (same Role/ToolCallID) while
// dropping the bulk, and tells the model it can re-invoke the tool.
const prunedToolPlaceholder = "[已省略该工具返回的大体量内容以节省上下文；如需其中的具体信息，请重新调用相应工具。原始结果仍记录在本次运行日志中。]"

// effectiveDecidePrune reports whether DP-3 (redirected) is live. With no
// Settings or Decider it stays off, preserving legacy behavior.
func (e *Engine) effectiveDecidePrune() bool {
	if e.Settings == nil || e.Decider == nil {
		return false
	}
	k := e.Settings.Knobs()
	return k.DecideEnabled && k.DecideToolPruneEnabled
}

// effectiveDecidePruneThreshold returns the estimated-token size above which a
// tool result is judged. Unconfigured holders fall back to the spec default.
func (e *Engine) effectiveDecidePruneThreshold() int {
	if e.Settings != nil {
		if t := e.Settings.Knobs().DecideToolPruneThreshold; t > 0 {
			return t
		}
	}
	return 500
}

// effectiveDecidePruneMaxJudged bounds how many bulky results are judged in a
// single turn so the judgments cannot cost more than they save.
func (e *Engine) effectiveDecidePruneMaxJudged() int {
	if e.Settings != nil {
		if n := e.Settings.Knobs().DecideToolPruneMaxJudged; n > 0 {
			return n
		}
	}
	return 8
}

// pruneToolResults scans the accumulated run messages for bulky tool results
// and, for the largest up to MaxJudged, asks the decision layer whether each
// is worth keeping verbatim. A non-degraded No replaces the result with a
// short placeholder (Role + ToolCallID preserved) and records a
// decide.tool_pruned event. Everything else fails open: degraded answers, raw
// errors, Yes verdicts and sub-threshold results are all left untouched.
//
// Mutations land in the caller's backing array via index assignment, so the
// shortened content is what later turns send.
func (e *Engine) pruneToolResults(ctx context.Context, runID string, messages []llm.Message) {
	threshold := e.effectiveDecidePruneThreshold()
	maxJudged := e.effectiveDecidePruneMaxJudged()
	for _, idx := range bulkyToolCandidates(messages, threshold, maxJudged) {
		m := messages[idx]
		ans, _ := e.Decider.Ask(ctx, decide.Question{
			Kind:    decide.KindPruneToolResult,
			Context: buildPruneProbe(m),
			OnFail:  decide.VerdictYes,
			TraceID: runID,
		})
		// Fail open: only a positive, non-degraded No prunes.
		if ans.Degraded || ans.Verdict != decide.VerdictNo {
			continue
		}
		sizeBefore := EstimateMessagesTokens([]llm.Message{m})
		pruned := llm.Message{
			Role:       llm.RoleTool,
			ToolCallID: m.ToolCallID,
			Content:    prunedToolPlaceholder,
		}
		sizeAfter := EstimateMessagesTokens([]llm.Message{pruned})
		messages[idx] = pruned
		saved := sizeBefore - sizeAfter
		if saved < 0 {
			saved = 0
		}
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventDecideToolPruned,
			Data: map[string]any{
				"tool_call_id": m.ToolCallID,
				"source":       ans.Source,
				"saved_tokens": saved,
			},
		})
	}
}

// bulkyToolCandidates returns indexes of RoleTool messages whose estimated
// size exceeds threshold, ordered largest first and capped at maxJudged.
func bulkyToolCandidates(messages []llm.Message, threshold, maxJudged int) []int {
	type cand struct {
		idx  int
		size int
	}
	var cands []cand
	for i, m := range messages {
		if m.Role != llm.RoleTool {
			continue
		}
		size := EstimateMessagesTokens([]llm.Message{m})
		if size <= threshold {
			continue
		}
		cands = append(cands, cand{i, size})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].size > cands[b].size })
	if len(cands) > maxJudged {
		cands = cands[:maxJudged]
	}
	out := make([]int, len(cands))
	for i, c := range cands {
		out[i] = c.idx
	}
	return out
}

// toolMessageText extracts the model-facing text of a (possibly multimodal)
// tool message. Image parts carry no text and are skipped.
func toolMessageText(m llm.Message) string {
	if len(m.Parts) == 0 {
		return m.Content
	}
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// buildPruneProbe assembles the (trimmed) context for the worth-keeping
// judgment. The probe is capped so it is never more expensive than the bulk
// it might remove from future turns.
func buildPruneProbe(m llm.Message) string {
	txt := toolMessageText(m)
	if r := []rune(txt); len(r) > decideProbeMaxRunes {
		txt = string(r[:decideProbeMaxRunes])
	}
	return "工具返回内容（判断是否值得在后续上下文中原样保留）：\n" + txt
}
