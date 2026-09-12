package skill

import "github.com/rebornace/baize/internal/llm"

const ActivateToolName = "activate_skill"

func ActivateToolSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ActivateToolName,
		Description: "Activate one or more skills from the Available skills list. " +
			"Use when the user's request matches a skill workflow. " +
			"Provide id for a single skill or ids for multiple. " +
			"After activation, the skill instructions are injected and its tools become visible.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Single skill id to activate",
				},
				"ids": map[string]any{
					"type":        "array",
					"description": "Multiple skill ids to activate",
					"items": map[string]any{
						"type": "string",
					},
				},
			},
		},
	}
}
