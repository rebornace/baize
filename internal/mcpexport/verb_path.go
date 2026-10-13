package mcpexport

import (
	"strings"
)

var verbPathSegments = map[string]struct{}{
	"approve":  {},
	"export":   {},
	"run":      {},
	"delete":   {},
	"create":   {},
	"update":   {},
	"trigger":  {},
	"execute":  {},
	"sync":     {},
	"send":     {},
}

// SuggestForceDenyExport returns true when a GET/HEAD path segment matches a mutating verb heuristic.
func SuggestForceDenyExport(method, path string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
	default:
		return false
	}
	for _, seg := range strings.Split(path, "/") {
		seg = strings.ToLower(strings.TrimSpace(seg))
		if seg == "" {
			continue
		}
		if _, ok := verbPathSegments[seg]; ok {
			return true
		}
	}
	return false
}
