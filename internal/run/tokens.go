package run

import (
	"unicode"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/llm"
)

// perMessageTokens is the structural overhead added per chat message (role
// markers, separators). Conservative on purpose.
const perMessageTokens = 4

// EstimateTextTokens approximates token count without a tokenizer:
// CJK characters count ~1 token each; other (ASCII-ish) content counts ~1
// token per 4 bytes. This deliberately over-estimates slightly so compaction
// triggers before a real context overflow.
func EstimateTextTokens(s string) int {
	if s == "" {
		return 0
	}
	tokens := 0
	for _, r := range s {
		if isCJK(r) {
			tokens++
		}
	}
	// 扣除 CJK 字符所占字节后，剩余字节按每 4 字节 1 token 计，向上取整以保守高估。
	cjkBytes := 0
	for _, r := range s {
		if isCJK(r) {
			cjkBytes += utf8.RuneLen(r)
		}
	}
	other := len(s) - cjkBytes
	tokens += other / 4
	if other%4 != 0 {
		tokens++
	}
	return tokens
}

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3040 && r <= 0x30FF) || // hiragana/katakana
		(r >= 0xAC00 && r <= 0xD7AF) // hangul
}

// EstimateMessagesTokens sums content tokens across messages plus per-message
// overhead. Multimodal Parts (images) are ignored token-wise (images are not
// compacted in v0); text parts are counted.
func EstimateMessagesTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += perMessageTokens
		if len(m.Parts) > 0 {
			for _, p := range m.Parts {
				if p.Type == "text" {
					total += EstimateTextTokens(p.Text)
				}
			}
			continue
		}
		total += EstimateTextTokens(m.Content)
	}
	return total
}

// EstimateToolsTokens approximates the cost of tool schemas by name+description.
func EstimateToolsTokens(tools []llm.ToolSpec) int {
	total := 0
	for _, t := range tools {
		total += EstimateTextTokens(t.Name) + EstimateTextTokens(t.Description) + perMessageTokens
	}
	return total
}
