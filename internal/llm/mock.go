package llm

import (
	"context"
	"strings"
	"unicode/utf8"
)

// Mock is a heuristic Provider for Demo A (no external LLM).
type Mock struct{}

func NewMock() *Mock {
	return &Mock{}
}

func (m *Mock) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	_ = ctx
	_ = tools

	for _, msg := range messages {
		if msg.Role == RoleTool {
			return Message{
				Role:    RoleAssistant,
				Content: summarizeToolResult(msg.Content),
			}, nil
		}
	}

	userText := lastUserContent(messages)
	lower := strings.ToLower(userText)

	switch {
	case strings.Contains(userText, "创建") || strings.Contains(lower, "create"):
		args := map[string]any{
			"title": extractTitle(userText),
		}
		if strings.Contains(userText, "紧急") {
			args["priority"] = "high"
		}
		return Message{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{{
				ID:        "call_create_ticket",
				Name:      "create_ticket",
				Arguments: args,
			}},
		}, nil
	case strings.Contains(userText, "查询") || strings.Contains(userText, "列表") || strings.Contains(lower, "list"):
		return Message{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{{
				ID:        "call_list_tickets",
				Name:      "list_tickets",
				Arguments: map[string]any{},
			}},
		}, nil
	default:
		return Message{
			Role:    RoleAssistant,
			Content: "我可以帮你创建工单或查询工单列表。请说明需要创建还是查询。",
		}, nil
	}
}

func lastUserContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

func extractTitle(text string) string {
	for _, sep := range []string{"：", ":"} {
		if i := strings.Index(text, sep); i >= 0 {
			rest := strings.TrimSpace(text[i+len(sep):])
			if rest != "" {
				return truncateRunes(rest, 80)
			}
		}
	}
	return truncateRunes(text, 80)
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

func summarizeToolResult(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return "操作已完成。"
	}
	return "操作已完成：" + content
}
