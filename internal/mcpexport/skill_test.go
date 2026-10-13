package mcpexport_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
)

func TestExportWorkflowSkillMarkdownThemes(t *testing.T) {
	body := mcpexport.ExportWorkflowSkillMarkdown()
	checks := []string{
		"decline",
		"do not retry",
		"export identity",
		"Do not treat",
		"/v0/auth/hosted",
		"paste passwords",
	}
	for _, sub := range checks {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(sub)) {
			t.Fatalf("markdown missing theme %q", sub)
		}
	}
}
