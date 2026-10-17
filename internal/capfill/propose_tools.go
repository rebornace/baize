package capfill

import (
	"context"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// ToolMeta pairs the propose_plugin spec with its invoker for registration.
type ToolMeta struct {
	Spec    llm.ToolSpec
	Invoker tool.Invoker
}

// ProposePluginSpec is the LLM tool schema for proposing a read-only plugin placeholder.
func ProposePluginSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ProposeName,
		Description: "Propose a read-only HTTP plugin placeholder (connector + disabled tools in settings). " +
			"Requires human approval before anything is saved; tools stay disabled until an admin enables them.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"connector_id": map[string]any{
					"type":        "string",
					"description": "Connector id (lowercase, hyphens)",
				},
				"goal": map[string]any{
					"type":        "string",
					"description": "One-line capability goal",
				},
				"gap": map[string]any{
					"type":        "string",
					"description": "Why existing tools cannot satisfy the goal",
				},
				"read_only": map[string]any{
					"type":        "boolean",
					"description": "Must be true; drafts are read-only only",
				},
				"risk": map[string]any{
					"type":        "string",
					"description": "Risk notes (PII, scan scope, etc.)",
				},
				"tools": map[string]any{
					"type":        "array",
					"description": "At least one tool to register under the connector",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Tool name (lowercase, hyphens)",
							},
							"description": map[string]any{
								"type":        "string",
								"description": "Short tool description",
							},
						},
						"required": []string{"name"},
					},
				},
				"base_url": map[string]any{
					"type":        "string",
					"description": "Optional HTTP plugin base URL",
				},
				"execution_callback_url": map[string]any{
					"type":        "string",
					"description": "Optional execution callback URL",
				},
			},
			"required": []string{"connector_id", "goal", "gap", "read_only", "risk", "tools"},
		},
	}
}

// ProposePluginToolMeta builds propose_plugin for RegisterSpecApproved(..., true).
func ProposePluginToolMeta(st store.Store) ToolMeta {
	return ToolMeta{
		Spec:    ProposePluginSpec(),
		Invoker: ProposePluginInvoker(st),
	}
}

// ProposePluginInvoker writes disabled connector/tool placeholders after HITL approval.
func ProposePluginInvoker(st store.Store) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		if st == nil {
			return map[string]any{"error": "store unavailable"}, true, nil
		}
		if controlplane.RoleFrom(ctx) == controlplane.RoleOperator {
			return map[string]any{"error": "propose_plugin requires admin role"}, true, nil
		}
		if err := ValidateProposePluginArgs(args, time.Now()); err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		d, err := DraftFromProposePluginArgs(args)
		if err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		if err := InstallPluginPlaceholder(st, d); err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		toolNames := make([]string, len(d.Tools))
		for i, t := range d.Tools {
			toolNames[i] = t.Name
		}
		return map[string]any{
			"ok":           true,
			"connector_id": d.ConnectorID,
			"tools":        toolNames,
		}, false, nil
	}
}
