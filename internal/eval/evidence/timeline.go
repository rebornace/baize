package evidence

import (
	"bytes"
	"encoding/json"
)

// Event is one run event from an evidence-eval fixture or snapshot.
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ToolCall is one ordered tool invocation on the timeline (aligned with PS1 Get-ToolTimeline).
type ToolCall struct {
	Order       int
	CallID      string
	Name        string
	ArgsText    string
	HasResult   bool
	IsError     bool
	HITLWaiting bool
}

// ToolTimeline builds an ordered timeline from run events.
func ToolTimeline(events []Event) []ToolCall {
	byID := make(map[string]int)
	var timeline []ToolCall
	for _, ev := range events {
		switch ev.Type {
		case "llm.tool_call":
			var data struct {
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			if json.Unmarshal(ev.Data, &data) != nil {
				continue
			}
			argsText := ""
			if len(data.Arguments) > 0 {
				var buf bytes.Buffer
				if err := json.Compact(&buf, data.Arguments); err == nil {
					argsText = buf.String()
				} else {
					argsText = string(data.Arguments)
				}
			}
			item := ToolCall{
				Order:    len(timeline),
				CallID:   data.ID,
				Name:     data.Name,
				ArgsText: argsText,
			}
			timeline = append(timeline, item)
			byID[data.ID] = len(timeline) - 1
		case "hitl.waiting":
			var data struct {
				ToolName string `json:"tool_name"`
			}
			if json.Unmarshal(ev.Data, &data) != nil {
				continue
			}
			name := data.ToolName
			for j := len(timeline) - 1; j >= 0; j-- {
				if timeline[j].Name == name && !timeline[j].HasResult {
					timeline[j].HITLWaiting = true
					break
				}
			}
		case "tool.result":
			var data struct {
				ID         string `json:"id"`
				ToolCallID string `json:"tool_call_id"`
				IsError    bool   `json:"is_error"`
			}
			if json.Unmarshal(ev.Data, &data) != nil {
				continue
			}
			callID := data.ID
			if callID == "" {
				callID = data.ToolCallID
			}
			if idx, ok := byID[callID]; ok {
				timeline[idx].HasResult = true
				timeline[idx].IsError = data.IsError
			}
		}
	}
	return timeline
}
