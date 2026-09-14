package memory

import (
	"context"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

const (
	RememberName = "remember_fact"
	ForgetName   = "forget_fact"
)

// RememberSpec is the LLM tool schema for explicit remember.
func RememberSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        RememberName,
		Description: "Remember a durable fact about the current account for later turns and sessions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Fact text to remember",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Optional stable key; same key overwrites the prior fact for this account",
				},
			},
			"required": []string{"text"},
		},
	}
}

// ForgetSpec is the LLM tool schema for explicit forget.
func ForgetSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        ForgetName,
		Description: "Forget a previously remembered fact by key or exact text match.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "Optional key of the fact to forget",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Optional exact text of the fact to forget",
				},
			},
		},
	}
}

// StubInvoker returns a not-implemented error payload. Task 5 replaces this
// with real remember/forget invokers wired to Store + MetaStore.
func StubInvoker(_ context.Context, _ map[string]any) (map[string]any, bool, error) {
	return map[string]any{"error": "not implemented"}, true, nil
}

// Ensure StubInvoker satisfies tool.Invoker.
var _ tool.Invoker = StubInvoker
