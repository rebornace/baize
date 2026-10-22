package procfs

import (
	"strings"
)

// SuggestedSkillID derives a user-skill id from a run id (must pass skill.BuildSKILLMD validation).
func SuggestedSkillID(runID string) string {
	slug := strings.TrimPrefix(runID, "run_")
	slug = strings.ToLower(slug)
	slug = strings.ReplaceAll(slug, "_", "-")
	return "from-" + slug
}
