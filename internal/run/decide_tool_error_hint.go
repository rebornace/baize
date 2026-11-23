package run

import (
	"encoding/json"
	"strings"

	"github.com/rebornace/baize/internal/llm"
)

// lastTurnHadToolError reports whether the most recent assistant tool_calls
// batch has at least one tool result marked as an error. Used to release
// sticky Top-K after a wrong-system miss so the next narrow can move.
func lastTurnHadToolError(messages []llm.Message) bool {
	if len(messages) == 0 {
		return false
	}
	// Walk back to the last assistant message that issued tool calls.
	assist := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleAssistant && len(messages[i].ToolCalls) > 0 {
			assist = i
			break
		}
	}
	if assist < 0 {
		return false
	}
	for i := assist + 1; i < len(messages); i++ {
		m := messages[i]
		if m.Role != llm.RoleTool {
			break
		}
		if toolResultIsError(m.Content) {
			return true
		}
	}
	return false
}

func toolResultIsError(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		// Non-JSON tool bodies are not treated as structured errors.
		return false
	}
	if v, ok := payload["is_error"]; ok {
		switch t := v.(type) {
		case bool:
			return t
		case string:
			return strings.EqualFold(t, "true")
		}
	}
	if _, ok := payload["error"]; ok {
		return true
	}
	return false
}
