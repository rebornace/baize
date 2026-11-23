package procfs

import (
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/skill"
)

func RenderDraft(tl Timeline, sourceRunID string) (skill.ProposeDraft, error) {
	tools := uniqueToolNames(tl.Entries)

	var b strings.Builder
	b.WriteString("## Steps\n\n")
	for i, e := range tl.Entries {
		line := e.Summary
		if line == "" {
			line = e.Type
		}
		fmt.Fprintf(&b, "%d. [%s] %s (`%s`)\n", i+1, e.RunID, line, e.Type)
	}
	b.WriteString("\n**Warning:** 由轨迹规则合成，须人审后再依赖。Human review required before relying on this draft.\n")

	return skill.ProposeDraft{
		Name:        SuggestedSkillID(sourceRunID),
		Description: fmt.Sprintf("Trajectory draft from run %s", sourceRunID),
		Tools:       tools,
		Body:        b.String(),
	}, nil
}

func uniqueToolNames(entries []TimelineEntry) []string {
	seen := make(map[string]bool)
	var out []string
	for _, e := range entries {
		if e.ToolName == "" || seen[e.ToolName] {
			continue
		}
		seen[e.ToolName] = true
		out = append(out, e.ToolName)
	}
	return out
}
