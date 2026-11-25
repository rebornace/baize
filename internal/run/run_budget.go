package run

import (
	"fmt"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// effectiveRunBudget resolves the runtime override first, then the Engine
// fields (YAML baseline). Zero values disable that budget.
func (e *Engine) effectiveRunBudget() (time.Duration, int) {
	if e.Settings != nil {
		k := e.Settings.Knobs()
		return time.Duration(k.MaxDurationSec) * time.Second, k.MaxTotalTokens
	}
	return e.MaxDuration, e.MaxTotalTokens
}

// runBudgetDeadline returns the wall-clock deadline for one active run
// segment. Zero means the duration budget is off.
func runBudgetDeadline(maxDuration time.Duration) time.Time {
	if maxDuration <= 0 {
		return time.Time{}
	}
	return time.Now().Add(maxDuration)
}

// existingRunTokens sums usage already persisted for this run (earlier HITL or
// child-receipt segments) so a token budget covers the whole run, not just the
// current segment.
func (e *Engine) existingRunTokens(runID string, maxTokens int) int {
	if maxTokens <= 0 {
		return 0
	}
	evs, err := e.Store.ListEvents(runID)
	if err != nil {
		return 0
	}
	total := 0
	for _, ev := range evs {
		if ev.Type != EventLLMUsage {
			continue
		}
		total += tokenCount(ev.Data["total_tokens"])
	}
	return total
}

func tokenCount(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// runBudgetReason reports why the active run must stop, or "" when budget
// remains. The caller emits the event and enters the grace turn.
func (e *Engine) runBudgetReason(deadline time.Time, tokens int, maxDuration time.Duration, maxTokens int) string {
	if maxTokens > 0 && tokens >= maxTokens {
		return fmt.Sprintf("max_total_tokens=%d", maxTokens)
	}
	if maxDuration > 0 && !deadline.IsZero() && !time.Now().Before(deadline) {
		return fmt.Sprintf("max_duration=%s", maxDuration)
	}
	return ""
}

func (e *Engine) emitRunBudgetExceeded(runID, reason string, tokens int, elapsed time.Duration) {
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventRunBudgetExceeded,
		Data: map[string]any{
			"reason":       reason,
			"total_tokens": tokens,
			"elapsed_ms":   elapsed.Milliseconds(),
		},
	})
}

func budgetLimitHint(locale, reason string) string {
	if isChineseLocale(locale) {
		return fmt.Sprintf(
			"[系统] 本次运行已用尽预算（%s）。请不要再调用任何工具；仅根据对话与已有工具结果，用现有信息给出尽量完整的结论或说明缺口。",
			reason,
		)
	}
	return fmt.Sprintf(
		"[system] This run has exhausted its budget (%s). Do not call any more tools; use the conversation and existing tool results to give the most complete conclusion possible, or state what is missing.",
		reason,
	)
}
