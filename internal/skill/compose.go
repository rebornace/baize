package skill

import (
	"sort"
	"strings"
)

func ComposeSystem(base string, cat *Catalog, activated []string) string {
	var b strings.Builder
	b.WriteString(base)
	pkgs := cat.List()
	if len(pkgs) == 0 {
		return b.String()
	}
	b.WriteString("\n\n## Available skills\n")
	for _, p := range pkgs {
		desc := p.Description
		if desc == "" {
			desc = p.Name
		}
		b.WriteString("- ")
		b.WriteString(p.ID)
		b.WriteString(" — ")
		b.WriteString(desc)
		b.WriteString("\n")
	}
	for _, id := range activated {
		p, ok := cat.Get(id)
		if !ok {
			continue
		}
		b.WriteString("\n## Skill: ")
		b.WriteString(id)
		b.WriteString("\n")
		b.WriteString(p.Body)
		b.WriteString("\n")
	}
	return b.String()
}

// VisibleTools returns tool names visible to the model.
// When defaultOrActivated is non-empty, returns the sorted union of tools
// declared by those skills intersected with enabled. When empty or nil,
// returns all enabled tool names sorted.
func VisibleTools(cat *Catalog, defaultOrActivated []string, enabled map[string]bool) []string {
	if len(defaultOrActivated) == 0 {
		out := make([]string, 0, len(enabled))
		for name, on := range enabled {
			if on {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out
	}

	seen := make(map[string]struct{})
	var out []string
	for _, id := range defaultOrActivated {
		p, ok := cat.Get(id)
		if !ok {
			continue
		}
		for _, tool := range p.Tools {
			if !enabled[tool] {
				continue
			}
			if _, dup := seen[tool]; dup {
				continue
			}
			seen[tool] = struct{}{}
			out = append(out, tool)
		}
	}
	sort.Strings(out)
	return out
}
