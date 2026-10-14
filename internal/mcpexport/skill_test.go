package mcpexport_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
)

func TestExportWorkflowSkillMarkdownThemes(t *testing.T) {
	body := mcpexport.ExportWorkflowSkillMarkdown()
	checks := []string{
		"拒绝",
		"decline",
		"不要重试",
		"do not retry",
		"导出身份",
		"export identity",
		"/v0/auth/hosted",
		"粘贴密码",
		"paste passwords",
		"hosted_login_required",
		"hosted_login_url",
		"-32042",
		"中英",
	}
	for _, sub := range checks {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(sub)) {
			t.Fatalf("markdown missing theme %q", sub)
		}
	}
}

func TestExportServerInstructionsThemes(t *testing.T) {
	body := mcpexport.ExportServerInstructions()
	checks := []string{
		"托管登录",
		"hosted_login_required",
		"hosted_login_url",
		"-32042",
		"切勿要求用户",
		"never ask",
		"baize_export_workflow",
		"中英",
	}
	for _, sub := range checks {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(sub)) {
			t.Fatalf("instructions missing theme %q", sub)
		}
	}
}
