package llm

import (
	"strings"
)

const (
	ThinkingOff    = "off"
	ThinkingLow    = "low"
	ThinkingMedium = "medium"
	ThinkingHigh   = "high"

	DialectAuto     = "auto"
	DialectOpenAI   = "openai"
	DialectDeepSeek = "deepseek"
	DialectQwen     = "qwen"
	DialectOmit     = "omit"
)

// ThinkingToggle is the DeepSeek-style thinking object for request bodies.
type ThinkingToggle struct {
	Type string `json:"type"`
}

// OpenRouterReasoning is the OpenRouter reasoning.effort object.
type OpenRouterReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// ThinkingFields holds at most one dialect's protocol fields for a chat request.
type ThinkingFields struct {
	ReasoningEffort string
	Thinking        *ThinkingToggle
	EnableThinking  *bool
	ThinkingBudget  *int
	Reasoning       *OpenRouterReasoning
}

var reasoningModelSubstrings = []string{
	"o1", "o3", "o4", "gpt-5", "gpt-6", "grok", "gemini-2.5", "gemini-3", "r1",
}

// InferDialect resolves which thinking protocol to use (spec §7.1).
func InferDialect(dialect, model, baseURL string) string {
	d := strings.TrimSpace(strings.ToLower(dialect))
	if d != "" && d != DialectAuto {
		return d
	}

	base := strings.ToLower(baseURL)
	modelLower := strings.ToLower(model)

	if strings.Contains(base, "openrouter.ai") {
		return "openrouter"
	}
	if strings.Contains(modelLower, "deepseek") || strings.Contains(base, "deepseek") {
		return DialectDeepSeek
	}
	if strings.Contains(modelLower, "qwen") ||
		strings.Contains(base, "qwen") ||
		strings.Contains(base, "dashscope") ||
		strings.Contains(base, "aliyuncs.com") {
		return DialectQwen
	}
	if modelLooksReasoning(modelLower) {
		return DialectOpenAI
	}
	return DialectOmit
}

func modelLooksReasoning(modelLower string) bool {
	for _, sub := range reasoningModelSubstrings {
		if strings.Contains(modelLower, sub) {
			return true
		}
	}
	return false
}

// ApplyThinking maps product level to dialect-specific request fields (spec §7.2).
func ApplyThinking(dialect, level string) ThinkingFields {
	switch dialect {
	case DialectOpenAI:
		return applyOpenAI(level)
	case DialectDeepSeek:
		return applyDeepSeek(level)
	case DialectQwen:
		return applyQwen(level)
	case "openrouter":
		return applyOpenRouter(level)
	case DialectOmit:
		return ThinkingFields{}
	default:
		return ThinkingFields{}
	}
}

func applyOpenAI(level string) ThinkingFields {
	switch level {
	case ThinkingOff:
		return ThinkingFields{ReasoningEffort: "none"}
	case ThinkingLow:
		return ThinkingFields{ReasoningEffort: "low"}
	case ThinkingMedium:
		return ThinkingFields{ReasoningEffort: "medium"}
	case ThinkingHigh:
		return ThinkingFields{ReasoningEffort: "high"}
	default:
		return ThinkingFields{}
	}
}

func applyDeepSeek(level string) ThinkingFields {
	switch level {
	case ThinkingOff:
		return ThinkingFields{Thinking: &ThinkingToggle{Type: "disabled"}}
	case ThinkingLow, ThinkingMedium, ThinkingHigh:
		return ThinkingFields{Thinking: &ThinkingToggle{Type: "enabled"}}
	default:
		return ThinkingFields{}
	}
}

func applyQwen(level string) ThinkingFields {
	switch level {
	case ThinkingOff:
		f := false
		return ThinkingFields{EnableThinking: &f}
	case ThinkingLow:
		t, b := true, 1024
		return ThinkingFields{EnableThinking: &t, ThinkingBudget: &b}
	case ThinkingMedium:
		t, b := true, 8192
		return ThinkingFields{EnableThinking: &t, ThinkingBudget: &b}
	case ThinkingHigh:
		t := true
		return ThinkingFields{EnableThinking: &t}
	default:
		return ThinkingFields{}
	}
}

func applyOpenRouter(level string) ThinkingFields {
	var effort string
	switch level {
	case ThinkingOff:
		effort = "none"
	case ThinkingLow:
		effort = "low"
	case ThinkingMedium:
		effort = "medium"
	case ThinkingHigh:
		effort = "high"
	default:
		return ThinkingFields{}
	}
	return ThinkingFields{Reasoning: &OpenRouterReasoning{Effort: effort}}
}

// JoinThinking concatenates non-empty thinking parts with "\n\n".
func JoinThinking(parts []string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "\n\n")
}
