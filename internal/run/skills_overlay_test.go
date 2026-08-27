package run

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/tool"
)

func TestSpecsForRunIncludesConnectorToolsOutsideSkill(t *testing.T) {
	cat, err := skill.LoadCatalog([]string{"../../skills", "../../examples/skills"}, t.TempDir())
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
