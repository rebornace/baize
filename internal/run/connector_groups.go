package run

import (
	"strings"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/llm"
)

// ConnectorGroupIndex maps connector Source ids ↔ named groups for DP-2a
// eligibility expansion (L1). Built from config; empty index is a no-op.
type ConnectorGroupIndex struct {
	// source -> group id
	bySource map[string]string
	// group id -> member sources
	byGroup map[string][]string
}

// BuildConnectorGroupIndex normalizes YAML groups. A connector listed in
// multiple groups keeps the first; empty ids / members are skipped.
func BuildConnectorGroupIndex(groups []config.ConnectorGroup) *ConnectorGroupIndex {
	idx := &ConnectorGroupIndex{
		bySource: map[string]string{},
		byGroup:  map[string][]string{},
	}
	for _, g := range groups {
		gid := strings.TrimSpace(g.ID)
		if gid == "" {
			continue
		}
		var members []string
		seen := map[string]bool{}
		for _, c := range g.Connectors {
			src := strings.TrimSpace(c)
			if src == "" || seen[src] {
				continue
			}
			seen[src] = true
			if _, dup := idx.bySource[src]; dup {
				continue // first group wins
			}
			idx.bySource[src] = gid
			members = append(members, src)
		}
		if len(members) > 0 {
			idx.byGroup[gid] = members
		}
	}
	if len(idx.bySource) == 0 {
		return nil
	}
	return idx
}

// SetConnectorGroups replaces the engine's group index (bootstrap / reload).
func (e *Engine) SetConnectorGroups(groups []config.ConnectorGroup) {
	if e == nil {
		return
	}
	e.connectorGroups = BuildConnectorGroupIndex(groups)
}

// expandAllowedByConnectorGroups adds every catalog source that shares a group
// with any currently allowed source. nil allowed (fail-open) is unchanged.
func expandAllowedByConnectorGroups(idx *ConnectorGroupIndex, specs []llm.ToolSpec, allowed map[string]bool) (out map[string]bool, expanded []string) {
	if idx == nil || allowed == nil || len(allowed) == 0 {
		return allowed, nil
	}
	inCatalog := map[string]bool{}
	for _, s := range specs {
		if s.Source != "" {
			inCatalog[s.Source] = true
		}
	}
	out = make(map[string]bool, len(allowed))
	for k, v := range allowed {
		out[k] = v
	}
	added := map[string]bool{}
	for src := range allowed {
		gid, ok := idx.bySource[src]
		if !ok {
			continue
		}
		for _, sib := range idx.byGroup[gid] {
			if !inCatalog[sib] || out[sib] {
				continue
			}
			out[sib] = true
			added[sib] = true
		}
	}
	if len(added) == 0 {
		return out, nil
	}
	expanded = make([]string, 0, len(added))
	for s := range added {
		expanded = append(expanded, s)
	}
	return out, expanded
}
