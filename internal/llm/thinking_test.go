package llm_test

import (
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func TestInferDialectOrder(t *testing.T) {
	cases := []struct {
		dialect, model, base, want string
	}{
		{"openai", "gpt-4o", "http://localhost", llm.DialectOpenAI},
		{"omit", "o3-mini", "https://api.openai.com/v1", llm.DialectOmit},
		{llm.DialectAuto, "x", "https://openrouter.ai/api/v1", "openrouter"},
		{llm.DialectAuto, "deepseek-chat", "https://api.deepseek.com", llm.DialectDeepSeek},
		{llm.DialectAuto, "qwen-plus", "https://dashscope.aliyuncs.com/compatible-mode/v1", llm.DialectQwen},
		{llm.DialectAuto, "o3-mini", "https://example.com/v1", llm.DialectOpenAI},
		{llm.DialectAuto, "gpt-4o", "https://api.openai.com/v1", llm.DialectOmit},
		{llm.DialectAuto, "llama3", "http://127.0.0.1:11434/v1", llm.DialectOmit},
	}
	for _, c := range cases {
		got := llm.InferDialect(c.dialect, c.model, c.base)
		if got != c.want {
			t.Fatalf("%+v got %s", c, got)
		}
	}
}

func TestApplyThinkingDeepSeekOff(t *testing.T) {
	p := llm.ApplyThinking(llm.DialectDeepSeek, llm.ThinkingOff)
	if p.Thinking == nil || p.Thinking.Type != "disabled" {
		t.Fatalf("%+v", p)
	}
	if p.ReasoningEffort != "" || p.EnableThinking != nil {
		t.Fatal("only deepseek thinking field")
	}
}

func TestApplyThinkingQwenLow(t *testing.T) {
	p := llm.ApplyThinking(llm.DialectQwen, llm.ThinkingLow)
	if p.EnableThinking == nil || !*p.EnableThinking {
		t.Fatalf("enable: %+v", p)
	}
	if p.ThinkingBudget == nil || *p.ThinkingBudget != 1024 {
		t.Fatalf("budget: %+v", p)
	}
	if p.ReasoningEffort != "" || p.Thinking != nil || p.Reasoning != nil {
		t.Fatal("only qwen fields")
	}
}

func TestApplyThinkingOpenAIHigh(t *testing.T) {
	p := llm.ApplyThinking(llm.DialectOpenAI, llm.ThinkingHigh)
	if p.ReasoningEffort != "high" {
		t.Fatalf("%+v", p)
	}
	if p.Thinking != nil || p.EnableThinking != nil || p.Reasoning != nil {
		t.Fatal("only openai reasoning_effort")
	}
}

func TestApplyThinkingOpenRouterOff(t *testing.T) {
	p := llm.ApplyThinking("openrouter", llm.ThinkingOff)
	if p.Reasoning == nil || p.Reasoning.Effort != "none" {
		t.Fatalf("%+v", p)
	}
	if p.ReasoningEffort != "" || p.Thinking != nil || p.EnableThinking != nil {
		t.Fatal("only openrouter reasoning")
	}
}

func TestApplyThinkingOmit(t *testing.T) {
	for _, level := range []string{llm.ThinkingOff, llm.ThinkingLow, llm.ThinkingMedium, llm.ThinkingHigh} {
		p := llm.ApplyThinking(llm.DialectOmit, level)
		if p.ReasoningEffort != "" || p.Thinking != nil || p.EnableThinking != nil ||
			p.ThinkingBudget != nil || p.Reasoning != nil {
			t.Fatalf("level=%s %+v", level, p)
		}
	}
}

func TestJoinThinking(t *testing.T) {
	got := llm.JoinThinking([]string{"a", "", "  ", "b", "c"})
	if got != "a\n\nb\n\nc" {
		t.Fatalf("got %q", got)
	}
	if llm.JoinThinking(nil) != "" {
		t.Fatal("nil")
	}
	if llm.JoinThinking([]string{}) != "" {
		t.Fatal("empty")
	}
}
