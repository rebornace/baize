package hostedlogin

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/store"
)

// BuildCandidates infers hosted login methods from login tools (glob match) and
// OpenAPI securitySchemes. Tool-derived candidates come first (sorted by tool
// name), then security schemes (sorted by scheme name).
func BuildCandidates(tools []store.Tool, glob string, schemes map[string]any) []store.HostedLoginMethod {
	var out []store.HostedLoginMethod
	sortedTools := append([]store.Tool(nil), tools...)
	sort.Slice(sortedTools, func(i, j int) bool {
		return sortedTools[i].Name < sortedTools[j].Name
	})
	for _, tl := range sortedTools {
		if !MatchToolGlob(glob, tl.Name) {
			continue
		}
		if m, ok := candidateFromLoginTool(tl.Name, tl.InputSchema); ok {
			out = append(out, m)
		}
	}
	if len(schemes) > 0 {
		names := make([]string, 0, len(schemes))
		for name := range schemes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			raw, ok := schemes[name].(map[string]any)
			if !ok {
				continue
			}
			if m, ok := candidateFromSecurityScheme(name, raw); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func candidateFromLoginTool(loginTool string, inputSchema map[string]any) (store.HostedLoginMethod, bool) {
	draft := DraftRecipe(loginTool, inputSchema)
	m, ok := draft.DefaultMethod()
	if !ok || m.Mode == "" {
		return store.HostedLoginMethod{}, false
	}
	m.ID = "tool-" + loginTool
	if strings.TrimSpace(m.Label) == "" {
		m.Label = loginTool
	}
	return m, true
}

func candidateFromSecurityScheme(name string, raw map[string]any) (store.HostedLoginMethod, bool) {
	draft := DraftRecipeFromSecurity(map[string]any{name: raw})
	m, ok := draft.DefaultMethod()
	if !ok || m.Mode == "" {
		return store.HostedLoginMethod{}, false
	}
	m.ID = "sec-" + name
	if strings.TrimSpace(m.Label) == "" {
		m.Label = name
	}
	return m, true
}

// MatchToolGlob reports whether toolName matches glob (same rules as connector draft).
func MatchToolGlob(glob, name string) bool {
	glob = strings.TrimSpace(glob)
	name = strings.TrimSpace(name)
	if glob == "" || glob == "*" {
		return true
	}
	if strings.EqualFold(glob, "__none__") {
		return false
	}
	ok, err := filepath.Match(glob, name)
	return err == nil && ok
}
