package skill

import (
	"context"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

// MDInstaller installs user SKILL.md packages and resolves them by id.
type MDInstaller interface {
	InstallMD(filename string, raw []byte) (Package, error)
	Get(id string) (Package, bool)
}

// ToolMeta pairs the propose_skill spec with its invoker for registration.
type ToolMeta struct {
	Spec    llm.ToolSpec
	Invoker tool.Invoker
}

// ProposeSpec is the LLM tool schema for proposing a new user skill.
func ProposeSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        ProposeName,
		Description: "Propose a new user skill (SKILL.md). Requires human approval before it is saved to the catalog.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill id (lowercase, hyphens)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Short description for the skill catalog",
				},
				"tools": map[string]any{
					"type":        "array",
					"description": "Optional tool names this skill may use",
					"items":       map[string]any{"type": "string"},
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Markdown body after the frontmatter",
				},
			},
			"required": []string{"name", "description", "body"},
		},
	}
}

// ProposeToolMeta builds propose_skill for RegisterSpecApproved(..., true).
func ProposeToolMeta(inst MDInstaller) ToolMeta {
	return ToolMeta{
		Spec:    ProposeSpec(),
		Invoker: ProposeInvoker(inst),
	}
}

// ProposeInvoker writes the skill to the catalog after HITL approval.
func ProposeInvoker(inst MDInstaller) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		if inst == nil {
			return map[string]any{"error": "skill catalog unavailable"}, true, nil
		}
		if controlplane.RoleFrom(ctx) == controlplane.RoleOperator {
			return map[string]any{"error": "propose_skill requires admin role"}, true, nil
		}
		if err := ValidateProposeArgs(args, time.Now()); err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		d, err := draftFromProposeArgs(args)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		raw, err := BuildSKILLMD(d)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		pkg, err := inst.InstallMD("SKILL.md", raw)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		return map[string]any{"ok": true, "id": pkg.ID}, false, nil
	}
}

func draftFromProposeArgs(args map[string]any) (ProposeDraft, error) {
	name, err := requiredStringArg(args, "name")
	if err != nil {
		return ProposeDraft{}, err
	}
	description, err := requiredStringArg(args, "description")
	if err != nil {
		return ProposeDraft{}, err
	}
	body, err := requiredStringArg(args, "body")
	if err != nil {
		return ProposeDraft{}, err
	}
	var tools []string
	if raw, ok := args["tools"]; ok && raw != nil {
		tools, err = parseToolsArg(raw)
		if err != nil {
			return ProposeDraft{}, err
		}
	}
	return ProposeDraft{
		Name:        name,
		Description: description,
		Tools:       tools,
		Body:        body,
	}, nil
}
