package run

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func TestEstimateTextTokens(t *testing.T) {
	if EstimateTextTokens("") != 0 {
		t.Fatal("empty -> 0")
	}
	ascii := EstimateTextTokens(strings.Repeat("a", 40)) // ~10 tokens
	if ascii < 8 || ascii > 14 {
		t.Fatalf("ascii 40 chars ~10 tokens, got %d", ascii)
	}
	cjk := EstimateTextTokens(strings.Repeat("中", 40)) // ~40 tokens
	if cjk < 36 || cjk > 48 {
		t.Fatalf("cjk 40 chars ~40 tokens, got %d", cjk)
	}
	if cjk <= ascii {
		t.Fatalf("cjk must cost more per char than ascii: cjk=%d ascii=%d", cjk, ascii)
	}
}

func TestEstimateMessagesTokensAddsOverhead(t *testing.T) {
	one := EstimateMessagesTokens([]llm.Message{{Role: llm.RoleUser, Content: strings.Repeat("a", 40)}})
	three := EstimateMessagesTokens([]llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "hi"},
		{Role: llm.RoleAssistant, Content: "hello"},
	})
	if one <= 0 {
		t.Fatal("one message should be > 0")
	}
	// 三条消息的结构开销（每条 +4）应使 three 明显大于单条短消息之外的基线
	if three < 12 {
		t.Fatalf("three messages should include per-message overhead, got %d", three)
	}
}

func TestEstimateToolsTokens(t *testing.T) {
	if EstimateToolsTokens(nil) != 0 {
		t.Fatal("nil tools -> 0")
	}
	tools := []llm.ToolSpec{{Name: "get_weather", Description: strings.Repeat("d", 40)}}
	if EstimateToolsTokens(tools) < 8 {
		t.Fatalf("tool with 40-char desc should count tokens, got %d", EstimateToolsTokens(tools))
	}
}
