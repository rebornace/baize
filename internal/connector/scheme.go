package connector

import (
	"strings"

	"github.com/rebornace/baize/internal/connector/openapi"
)

// UniqueSecurityScheme returns the sole security scheme name across routes, or "".
func UniqueSecurityScheme(routes []openapi.ToolRoute) string {
	seen := map[string]struct{}{}
	for _, r := range routes {
		for _, s := range r.Security {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			seen[s] = struct{}{}
		}
	}
	if len(seen) != 1 {
		return ""
	}
	for s := range seen {
		return s
	}
	return ""
}
