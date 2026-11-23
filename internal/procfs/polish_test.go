package procfs

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/skill"
)

func TestMergeToolsPreserveRule(t *testing.T) {
	got := MergeToolsPreserveRule([]string{"a", "b", "c"}, []string{"b", "invented", "a"})
	if len(got) != 3 || got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Fatalf("got=%v", got)
	}
}

func TestApplyPolishResultMergesDroppedTools(t *testing.T) {
	rule, err := skill.BuildSKILLMD(skill.ProposeDraft{
		Name:        "from-abc12345",
		Description: "Trajectory draft from run x",
		Tools:       []string{"list_tickets", "get_ticket"},
		Body:        "## Steps\n\n1. list\n\n**Warning:** 须人审\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	llmOut := "---\nname: from-abc12345\ndescription: clearer\ntools:\n  - list_tickets\n---\n\n## Steps\n\nDo the list carefully.\n\n**Warning:** Human review required.\n"
	out, err := ApplyPolishResult(string(rule), llmOut)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := skill.ParseSKILLMD([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.Tools) != 2 || pkg.Tools[0] != "list_tickets" || pkg.Tools[1] != "get_ticket" {
		t.Fatalf("tools=%v", pkg.Tools)
	}
	if !strings.Contains(pkg.Body, "carefully") {
		t.Fatalf("body=%q", pkg.Body)
	}
}

func TestApplyPolishResultRejectsGarbage(t *testing.T) {
	rule, _ := skill.BuildSKILLMD(skill.ProposeDraft{Name: "from-abc12345", Description: "d", Body: "b"})
	if _, err := ApplyPolishResult(string(rule), "not a skill"); err == nil {
		t.Fatal("want error")
	}
}
