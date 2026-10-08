package run

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/tool"
)

func TestAppendClockHint(t *testing.T) {
	got := appendClockHint("你是助手。", "zh-CN")
	if !strings.Contains(got, "当前时间：") {
		t.Fatalf("missing clock hint: %q", got)
	}
	if !strings.Contains(got, "你是助手。") {
		t.Fatalf("base lost: %q", got)
	}
	// Idempotent.
	again := appendClockHint(got, "zh-CN")
	if again != got {
		t.Fatalf("clock hint duplicated")
	}
	en := appendClockHint("You are helpful.", "en")
	if !strings.Contains(en, "Current time:") {
		t.Fatalf("en clock missing: %q", en)
	}
}

func TestSpecsForRunIncludesConnectorToolsOutsideSkill(t *testing.T) {
	cat, err := skill.LoadCatalog([]string{"../../skills", "../../examples/skills"}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "list_tickets", Description: "list"},
		ConnectorID: "ticket-api",
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}, false)
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "pet_step_daily", Description: "pet steps"},
		ConnectorID: "pet-api",
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"steps": 42}, false, nil
	}, false)

	eng := &Engine{
		Tools:  reg,
		Skills: cat,
	}
	eng.beginRunSkills("run_1", []string{"data-analytics", "ticket-triage"}, "sys")

	specs := eng.specsForRun("run_1")
	byName := make(map[string]bool, len(specs))
	for _, s := range specs {
		byName[s.Name] = true
	}
	if !byName["pet_step_daily"] {
		t.Fatalf("connector tool missing from specs; got %v", byName)
	}
	if !byName["list_tickets"] {
		t.Fatalf("skill tool missing from specs; got %v", byName)
	}
}

func TestSpecsForRunExclusiveAllowlist(t *testing.T) {
	cat, err := skill.LoadCatalog([]string{"../../skills", "../../examples/skills"}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "list_tickets", Description: "list"},
		ConnectorID: "ticket-api",
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}, false)
	reg.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "pet_step_daily", Description: "pet steps"},
		ConnectorID: "pet-api",
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"steps": 42}, false, nil
	}, false)

	eng := &Engine{Tools: reg, Skills: cat}
	eng.beginRunSkillsOpts("run_ex", []string{"ticket-triage"}, "sys", true, "")

	specs := eng.specsForRun("run_ex")
	byName := make(map[string]bool, len(specs))
	for _, s := range specs {
		byName[s.Name] = true
	}
	if byName["pet_step_daily"] {
		t.Fatalf("exclusive must hide undeclared connector tool; got %v", byName)
	}
	if !byName["list_tickets"] {
		t.Fatalf("exclusive must keep skill-declared tool; got %v", byName)
	}
	if !byName["activate_skill"] {
		t.Fatalf("exclusive must keep activate_skill; got %v", byName)
	}
}
