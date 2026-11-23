package skill_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/skill"
)

var proposeTestNow = time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

func TestBuildSKILLMDRoundTrip(t *testing.T) {
	raw, err := skill.BuildSKILLMD(skill.ProposeDraft{
		Name: "ticket-followup", Description: "跟进工单",
		Tools: []string{"list_tickets", "get_ticket"},
		Body:  "1. 先 list_tickets\n2. 再 get_ticket",
	})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := skill.ParseSKILLMD(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "ticket-followup" || pkg.Description != "跟进工单" {
		t.Fatalf("%+v", pkg)
	}
	if len(pkg.Tools) != 2 || pkg.Tools[0] != "list_tickets" {
		t.Fatalf("tools=%v", pkg.Tools)
	}
	if !strings.Contains(pkg.Body, "list_tickets") {
		t.Fatalf("body=%q", pkg.Body)
	}
}

func TestValidateProposeArgsRejects(t *testing.T) {
	if err := skill.ValidateProposeArgs(map[string]any{
		"name": "x", "description": "d", "body": "",
	}, proposeTestNow); err == nil {
		t.Fatal("empty body")
	}
	if err := skill.ValidateProposeArgs(map[string]any{
		"name": "", "description": "d", "body": "步骤",
	}, proposeTestNow); err == nil {
		t.Fatal("empty name")
	}
	long := strings.Repeat("字", 20001)
	if err := skill.ValidateProposeArgs(map[string]any{
		"name": "x", "description": "d", "body": long,
	}, proposeTestNow); err == nil {
		t.Fatal("overlong body")
	}
}
