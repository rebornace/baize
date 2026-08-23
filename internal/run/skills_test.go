package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func loadTestCatalog(t *testing.T, skills map[string]struct {
	desc  string
	tools []string
	body  string
}) *skill.Catalog {
	t.Helper()
	root := t.TempDir()
	for id, s := range skills {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		b.WriteString("---\nname: ")
		b.WriteString(id)
		b.WriteString("\ndescription: ")
		b.WriteString(s.desc)
		b.WriteString("\ntools:\n")
		for _, toolName := range s.tools {
			b.WriteString("  - ")
			b.WriteString(toolName)
			b.WriteString("\n")
		}
		b.WriteString("---\n\n")
		b.WriteString(s.body)
		b.WriteString("\n")
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cat, err := skill.LoadCatalog([]string{root}, filepath.Join(root, "_user"))
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func specNames(specs []llm.ToolSpec) map[string]bool {
	out := make(map[string]bool, len(specs))
	for _, s := range specs {
		out[s.Name] = true
	}
	return out
}

func TestExecuteFiltersToolsByDefaultSkills(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("list_tickets", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	})
	cat := loadTestCatalog(t, map[string]struct {
		desc  string
		tools []string
		body  string
	}{
		"demo": {desc: "list only", tools: []string{"list_tickets"}, body: "use list_tickets"},
	})

	var sawSpecs map[string]bool
	llmMock := &captureLLM{onChat: func(msgs []llm.Message, tools []llm.ToolSpec) llm.Message {
		if sawSpecs == nil {
			sawSpecs = specNames(tools)
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "done"}
	}}

	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if sawSpecs["create_ticket"] {
		t.Fatalf("create_ticket must be filtered; specs=%v", sawSpecs)
	}
	if !sawSpecs["list_tickets"] {
		t.Fatalf("missing list_tickets; specs=%v", sawSpecs)
	}
	if !sawSpecs[skill.ActivateToolName] {
		t.Fatalf("missing activate_skill; specs=%v", sawSpecs)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestActivateSkillExpandsTools(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("list_tickets", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"id": "1"}, false, nil
	})
	cat := loadTestCatalog(t, map[string]struct {
		desc  string
		tools []string
		body  string
	}{
		"demo":  {desc: "list", tools: []string{"list_tickets"}, body: "list body"},
		"demo2": {desc: "create", tools: []string{"create_ticket"}, body: "create body"},
	})

	var stepSpecs []map[string]bool
	llmMock := &captureLLM{onChat: func(msgs []llm.Message, tools []llm.ToolSpec) llm.Message {
		stepSpecs = append(stepSpecs, specNames(tools))
		if len(stepSpecs) == 1 {
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo2"}},
			}}
		}
		if !strings.Contains(msgs[0].Content, "create body") {
			t.Errorf("system after activate missing demo2 body: %s", msgs[0].Content)
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "ok"}
	}}

	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(stepSpecs) < 2 {
		t.Fatalf("want >=2 chat steps, got %d", len(stepSpecs))
	}
	if stepSpecs[0]["create_ticket"] {
		t.Fatalf("step1 must not include create_ticket: %v", stepSpecs[0])
	}
	if !stepSpecs[1]["create_ticket"] {
		t.Fatalf("step2 must include create_ticket after activate: %v", stepSpecs[1])
	}
	if !stepSpecs[1]["list_tickets"] {
		t.Fatalf("step2 must keep list_tickets: %v", stepSpecs[1])
	}
}

func TestActivateSkillUnknownIDIsToolError(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.Register("list_tickets", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	cat := loadTestCatalog(t, map[string]struct {
		desc  string
		tools []string
		body  string
	}{
		"demo": {desc: "list", tools: []string{"list_tickets"}, body: "body"},
	})

	llmMock := &captureLLM{onChat: func(msgs []llm.Message, tools []llm.ToolSpec) llm.Message {
		if len(msgs) == 2 { // system + user
			return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "missing"}},
			}}
		}
		return llm.Message{Role: llm.RoleAssistant, Content: "recovered"}
	}}

	ag := agent.Def{ID: "a", System: "helper", Skills: []string{"demo"}}
	r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s want succeeded", got.Status)
	}
	evs, _ := st.ListEvents(r.ID)
	var sawErr bool
	for _, ev := range evs {
		if ev.Type != EventToolResult {
			continue
		}
		if asString(ev.Data["name"]) != skill.ActivateToolName {
			continue
		}
		if isErr, _ := ev.Data["is_error"].(bool); isErr {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("expected activate_skill tool result is_error")
	}
}

func TestEmptyDefaultSkillsShowsAllEnabledAndActivate(t *testing.T) {
	for _, skills := range [][]string{nil, {}} {
		skills := skills
		name := "nil"
		if skills != nil {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			st := store.NewMemory()
			reg := tool.NewRegistry()
			reg.Register("list_tickets", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
				return map[string]any{"ok": true}, false, nil
			})
			reg.Register("create_ticket", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
				return map[string]any{"id": "1"}, false, nil
			})
			cat := loadTestCatalog(t, map[string]struct {
				desc  string
				tools []string
				body  string
			}{
				"demo2": {desc: "create", tools: []string{"create_ticket"}, body: "create body"},
			})

			var stepSpecs []map[string]bool
			llmMock := &captureLLM{onChat: func(msgs []llm.Message, tools []llm.ToolSpec) llm.Message {
				stepSpecs = append(stepSpecs, specNames(tools))
				if len(stepSpecs) == 1 {
					if !stepSpecs[0]["list_tickets"] || !stepSpecs[0]["create_ticket"] || !stepSpecs[0][skill.ActivateToolName] {
						t.Errorf("step1 want all enabled + activate_skill, got %v", stepSpecs[0])
					}
					return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
						{ID: "c1", Name: skill.ActivateToolName, Arguments: map[string]any{"id": "demo2"}},
					}}
				}
				if !strings.Contains(msgs[0].Content, "create body") {
					t.Errorf("system after activate missing demo2 body: %s", msgs[0].Content)
				}
				// Empty default: tools stay full enabled set after activate.
				if !stepSpecs[1]["list_tickets"] || !stepSpecs[1]["create_ticket"] || !stepSpecs[1][skill.ActivateToolName] {
					t.Errorf("step2 want all enabled + activate_skill, got %v", stepSpecs[1])
				}
				return llm.Message{Role: llm.RoleAssistant, Content: "ok"}
			}}

			ag := agent.Def{ID: "a", System: "helper", Skills: skills}
			r, err := st.CreateRun(store.CreateRunInput{AgentID: ag.ID, Input: "hi"})
			if err != nil {
				t.Fatal(err)
			}
			eng := &Engine{Store: st, LLM: llmMock, Tools: reg, Skills: cat}
			if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if len(stepSpecs) < 2 {
				t.Fatalf("want >=2 chat steps, got %d", len(stepSpecs))
			}
			got, _ := st.GetRun(r.ID)
			if got.Status != store.StatusSucceeded {
				t.Fatalf("status=%s", got.Status)
			}
		})
	}
}
