package llm

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Provider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error)
}
