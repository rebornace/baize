package memory

import (
	"strings"
	"unicode/utf8"
)

// Rank scores how well text matches query for local retrieval.
// Zero means no match. Higher is better. Query must appear as a substring of text,
// or each whitespace-separated token of query must appear in text.
func Rank(query, text string) int {
	q := strings.TrimSpace(query)
	if q == "" || text == "" {
		return 0
	}
	if strings.Contains(text, q) {
		qr := utf8.RuneCountInString(q)
		tr := utf8.RuneCountInString(text)
		if tr == 0 {
			return 0
		}
		return qr*1000/tr + qr*10
	}
	score := 0
	for _, w := range strings.Fields(q) {
		if w != "" && strings.Contains(text, w) {
			score += 10
		}
	}
	return score
}
