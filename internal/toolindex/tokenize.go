package toolindex

import (
	"strings"
	"unicode"
)

// Tokens builds the matching token set for text: Latin/digit words (camelCase
// split) plus adjacent Han bigrams.
func Tokens(s string) map[string]bool {
	set := make(map[string]bool)
	var hanRun []rune
	flushHan := func() {
		if len(hanRun) == 0 {
			return
		}
		for i := 0; i+1 < len(hanRun); i++ {
			set[string(hanRun[i:i+2])] = true
		}
		// Single-character Han still carries intent (e.g. 删).
		if len(hanRun) == 1 {
			set[string(hanRun)] = true
		}
		hanRun = hanRun[:0]
	}

	var word []rune
	flushWord := func() {
		if len(word) == 0 {
			return
		}
		for _, part := range splitCamel(string(word)) {
			part = strings.ToLower(strings.TrimSpace(part))
			if part != "" {
				set[part] = true
			}
		}
		word = word[:0]
	}

	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			flushWord()
			hanRun = append(hanRun, r)
		case isLatinAlphaDigit(r):
			flushHan()
			word = append(word, r)
		default:
			flushHan()
			flushWord()
		}
	}
	flushHan()
	flushWord()
	return set
}

// QueryTokens returns raw content tokens for lexical BM25. Cross-lingual
// matching is the Embedder's job — no verb synonym expansion here.
func QueryTokens(query string) map[string]bool {
	return Tokens(query)
}

// RefineQueryTokens drops filler stopwords for deterministic routing scorers.
// It does not translate or synonym-expand.
func RefineQueryTokens(raw map[string]bool) map[string]bool {
	out := make(map[string]bool, len(raw))
	for t := range raw {
		if IsStopword(t) {
			continue
		}
		out[t] = true
	}
	return out
}

func isLatinAlphaDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func splitCamel(word string) []string {
	chunks := strings.FieldsFunc(word, func(r rune) bool {
		return r == '_' || r == '-' || r >= '0' && r <= '9'
	})
	var out []string
	for _, c := range chunks {
		runes := []rune(c)
		start := 0
		for i := 1; i < len(runes); i++ {
			if unicode.IsLower(runes[i-1]) && unicode.IsUpper(runes[i]) {
				out = append(out, string(runes[start:i]))
				start = i
				continue
			}
			if i+1 < len(runes) && unicode.IsUpper(runes[i-1]) &&
				unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i+1]) {
				out = append(out, string(runes[start:i]))
				start = i
			}
		}
		out = append(out, string(runes[start:]))
	}
	return out
}

// PathTokens extracts path segments as tokens (/admin/users/{id} → admin, users).
func PathTokens(path string) map[string]bool {
	set := make(map[string]bool)
	for _, seg := range strings.Split(path, "/") {
		seg = strings.TrimSpace(seg)
		if seg == "" || strings.HasPrefix(seg, "{") {
			continue
		}
		for t := range Tokens(seg) {
			set[t] = true
		}
	}
	return set
}
