package run

import (
	"sort"

	"github.com/rebornace/baize/internal/llm"
)

// stickyNarrowState holds the last DP-2a sent tool set for a conversation so
// small ranking jitter does not reshuffle the schema prefix every turn.
type stickyNarrowState struct {
	names []string
}

// applyStickyNarrow reuses the previous conversation's narrowed set when the
// newly computed set is similar enough (Jaccard ≥ min). Significant changes
// replace the sticky snapshot. Returns the set to send and whether sticky was
// reused. Missing/disabled sticky is a no-op (returns next unchanged).
func (e *Engine) applyStickyNarrow(conversationID string, next []llm.ToolSpec, available []llm.ToolSpec) (sent []llm.ToolSpec, reused bool, jaccard float64) {
	if e == nil || conversationID == "" || !e.effectiveDecideToolSticky() {
		return next, false, 0
	}
	minJ := e.effectiveDecideToolStickyMinJaccard()
	nextNames := toolNames(next)
	e.stickyMu.Lock()
	defer e.stickyMu.Unlock()
	if e.stickyNarrow == nil {
		e.stickyNarrow = map[string]stickyNarrowState{}
	}
	prev, ok := e.stickyNarrow[conversationID]
	if !ok || len(prev.names) == 0 {
		e.stickyNarrow[conversationID] = stickyNarrowState{names: append([]string(nil), nextNames...)}
		return next, false, 0
	}
	jaccard = nameSetJaccard(prev.names, nextNames)
	if jaccard < minJ {
		e.stickyNarrow[conversationID] = stickyNarrowState{names: append([]string(nil), nextNames...)}
		return next, false, jaccard
	}
	// Reuse previous names, filtered to tools still in the current full catalog.
	byName := make(map[string]llm.ToolSpec, len(available))
	for _, s := range available {
		byName[s.Name] = s
	}
	out := make([]llm.ToolSpec, 0, len(prev.names))
	kept := make([]string, 0, len(prev.names))
	for _, name := range prev.names {
		if s, ok := byName[name]; ok {
			out = append(out, s)
			kept = append(kept, name)
		}
	}
	if len(out) == 0 {
		// Previous set vanished from catalog; fall through to fresh narrow.
		e.stickyNarrow[conversationID] = stickyNarrowState{names: append([]string(nil), nextNames...)}
		return next, false, jaccard
	}
	e.stickyNarrow[conversationID] = stickyNarrowState{names: kept}
	return out, true, jaccard
}

func nameSetJaccard(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	sa := make(map[string]struct{}, len(a))
	for _, n := range a {
		sa[n] = struct{}{}
	}
	sb := make(map[string]struct{}, len(b))
	for _, n := range b {
		sb[n] = struct{}{}
	}
	inter := 0
	for n := range sa {
		if _, ok := sb[n]; ok {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

// clearStickyNarrow drops sticky state (tests).
func (e *Engine) clearStickyNarrow(conversationID string) {
	if e == nil {
		return
	}
	e.stickyMu.Lock()
	defer e.stickyMu.Unlock()
	delete(e.stickyNarrow, conversationID)
}

// sortedNames is a test helper.
func sortedNames(specs []llm.ToolSpec) []string {
	names := toolNames(specs)
	sort.Strings(names)
	return names
}
