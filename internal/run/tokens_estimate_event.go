package run

import (
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// emitTokenEstimate records the estimator-vs-real prompt comparison for the
// payload actually sent. Observability only: it never feeds back into
// compaction. Skipped when the provider reported no real usage, so a
// mock/unreporting provider cannot record a misleading real=0.
func (e *Engine) emitTokenEstimate(runID string, turn int, messages []llm.Message, tools []llm.ToolSpec, u llm.Usage) {
	if e == nil || e.Store == nil || u.PromptTokens <= 0 {
		return
	}
	msgsEst := EstimateMessagesTokens(messages)
	toolsEst := EstimateToolsTokens(tools) // now includes InputSchema
	est := msgsEst + toolsEst

	real := u.PromptTokens
	data := map[string]any{
		"turn":               turn,
		"estimated":          est,
		"estimated_messages": msgsEst,
		"estimated_tools":    toolsEst,
		"real":               real,
		"delta":              est - real,
		"ratio":              roundHundredth(float64(est) / float64(real)),
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventTokenEstimate,
		Data: data,
	})
}

func roundHundredth(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
