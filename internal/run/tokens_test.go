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
	if EstimateMessagesTokens(nil) != 0 {
		t.Fatal("nil messages -> 0")
	}
	// 单条 Content: 40 字节 ASCII -> 40/4=10，加每条 4 开销 = 14
	one := EstimateMessagesTokens([]llm.Message{{Role: llm.RoleUser, Content: strings.Repeat("a", 40)}})
	if one != 14 {
		t.Fatalf("one message: 4 overhead + 10 text = 14, got %d", one)
	}
	// sys=3 字节->1, hi=2->1, hello=5->ceil(5/4)=2；内容合计 4 + 3×4 开销 = 16
	three := EstimateMessagesTokens([]llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "hi"},
		{Role: llm.RoleAssistant, Content: "hello"},
	})
	if three != 16 {
		t.Fatalf("three messages: 4 content + 12 overhead = 16, got %d", three)
	}
}

func TestEstimateMessagesTokensParts(t *testing.T) {
	// text part 40 字节 -> 10；image part 字节不计；加每条 4 开销 = 14
	msg := llm.Message{
		Role: llm.RoleUser,
		Parts: []llm.ContentPart{
			{Type: "text", Text: strings.Repeat("a", 40)},
			{Type: "image", ImageBytes: []byte("fakedata")},
		},
	}
	if got := EstimateMessagesTokens([]llm.Message{msg}); got != 14 {
		t.Fatalf("parts message: 4 overhead + 10 text, image ignored = 14, got %d", got)
	}
	// Parts 非空时 Content 被忽略：即便 Content 是大量 CJK 文本，结果仍为 14
	msg.Content = strings.Repeat("中", 100)
	if got := EstimateMessagesTokens([]llm.Message{msg}); got != 14 {
		t.Fatalf("parts set => Content ignored, want 14, got %d", got)
	}
}

func TestEstimateToolsTokens(t *testing.T) {
	if EstimateToolsTokens(nil) != 0 {
		t.Fatal("nil tools -> 0")
	}
	// Name "get_weather" 11 字符 -> ceil(11/4)=3；Description 40 字符 -> 10；开销 4 = 17
	tools := []llm.ToolSpec{{Name: "get_weather", Description: strings.Repeat("d", 40)}}
	if got := EstimateToolsTokens(tools); got != 17 {
		t.Fatalf("tool: 3 name + 10 desc + 4 overhead = 17, got %d", got)
	}
}
