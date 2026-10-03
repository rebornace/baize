package dbutil

import (
	"fmt"
	"strings"
)

// RebindPostgres converts SQLite-style ? placeholders to PostgreSQL $n placeholders.
func RebindPostgres(query string) string {
	if !strings.Contains(query, "?") {
		return query
	}
	var b strings.Builder
	n := 1
	for _, r := range query {
		if r == '?' {
			b.WriteString(fmt.Sprintf("$%d", n))
			n++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
