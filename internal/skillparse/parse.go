package skillparse

import (
	"regexp"
	"strings"
	"unicode"
)

var mentionRe = regexp.MustCompile(`(^|[\s])[@/]([a-zA-Z0-9][a-zA-Z0-9_-]*)\b`)

// MentionOnlyFallback is the model-facing user text when the turn is only
// @id / /id skill mentions. Stripping those markers would otherwise leave an
// empty user message (blank chat bubble + no login intent).
const MentionOnlyFallback = "请立即按已激活技能开始执行。若为登录类技能，请立刻调用该技能列出的登录相关工具；缺少必填参数时向用户询问，不要只问候或空转。"

// Parse extracts skill ids from @id and /id mentions in input, removes those
// markers, and returns cleaned text with consecutive whitespace collapsed to a
// single space.
func Parse(input string) (cleaned string, ids []string) {
	matches := mentionRe.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return collapseWhitespace(strings.TrimSpace(input)), nil
	}

	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		id := input[m[4]:m[5]]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	cleaned = input
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		removeStart := m[2]
		if m[3] > m[2] {
			removeStart = m[3]
		}
		cleaned = cleaned[:removeStart] + cleaned[m[1]:]
	}

	return collapseWhitespace(strings.TrimSpace(cleaned)), ids
}

// IsMentionOnly reports whether input contains skill mentions and nothing else
// after those markers are stripped.
func IsMentionOnly(input string) bool {
	cleaned, ids := Parse(input)
	return cleaned == "" && len(ids) > 0
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace && b.Len() > 0 {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return b.String()
}
