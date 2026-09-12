package skillparse

import (
	"regexp"
	"strings"
	"unicode"
)

var mentionRe = regexp.MustCompile(`(^|[\s])[@/]([a-zA-Z0-9][a-zA-Z0-9_-]*)\b`)

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
