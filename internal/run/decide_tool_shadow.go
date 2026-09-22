package run

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/decide"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// buildToolProbe assembles a cheap description of the candidate tools: name
// plus the first line of the description only. It never includes InputSchema;
// the probe must stay far cheaper than the prefill it might one day save.
func buildToolProbe(specs []llm.ToolSpec) string {
	var b strings.Builder
	for _, s := range specs {
		firstLine := strings.TrimSpace(s.Description)
		if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
			firstLine = strings.TrimSpace(firstLine[:i])
		}
		b.WriteString("- ")
		b.WriteString(s.Name)
		b.WriteString(": ")
		b.WriteString(firstLine)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// toolNames returns the candidate tool names in spec order.
func toolNames(specs []llm.ToolSpec) []string {
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}

// recordToolShadow consults the decision layer for which tools to keep and
// writes a decide.tool_shadow event. In shadow mode this never filters the
// specs the caller sends; it only records the layer's pick so recall can be
// measured before enforce mode is ever enabled.
//
// OnFail is VerdictYes: if every implementation abstains, the answer carries
// no narrowed list, and the caller keeps sending the full set (today's
// behavior).
func (e *Engine) recordToolShadow(ctx context.Context, runID string, turn int, specs []llm.ToolSpec) {
	ans, _ := e.Decider.Ask(ctx, decide.Question{
		Kind:    decide.KindToolCandidates,
		Context: buildToolProbe(specs),
		Options: toolNames(specs),
		OnFail:  decide.VerdictYes,
		TraceID: runID,
	})

	kept := validCandidateNames(ans.Values, specs)
	data := map[string]any{
		"turn":       turn,
		"total":      len(specs),
		"kept":       kept,
		"kept_count": len(kept),
		"source":     ans.Source,
		"degraded":   ans.Degraded,
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventDecideToolShadow,
		Data: data,
	})
}

// validCandidateNames restricts the layer's pick to names that actually exist
// and removes duplicates, preserving spec order, so a hallucinated tool name
// can never pollute the recorded set.
func validCandidateNames(picked []string, specs []llm.ToolSpec) []string {
	allowed := make(map[string]bool, len(specs))
	for _, s := range specs {
		allowed[s.Name] = true
	}
	seen := make(map[string]bool, len(picked))
	kept := make([]string, 0, len(picked))
	for _, name := range picked {
		if !allowed[name] || seen[name] {
			continue
		}
		seen[name] = true
		kept = append(kept, name)
	}
	return kept
}
