package skill_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/skill"
)

func TestParseSKILLMD(t *testing.T) {
	raw := "---\nname: ticket-triage\ndescription: 工单分诊\ntools:\n  - list_tickets\n  - create_ticket\n---\n\n# Body\n\nstep 1\n"
	pkg, err := skill.ParseSKILLMD([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "ticket-triage" || pkg.Description != "工单分诊" {
		t.Fatalf("%+v", pkg)
	}
	if len(pkg.Tools) != 2 || pkg.Tools[0] != "list_tickets" {
		t.Fatalf("tools=%v", pkg.Tools)
	}
	if !strings.Contains(pkg.Body, "# Body") {
		t.Fatalf("body=%q", pkg.Body)
	}
}
