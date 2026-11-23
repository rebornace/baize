package skill

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

const UnarchiveName = "unarchive_skill"

// UnarchiveSpec is the LLM tool schema for restoring an archived user skill.
func UnarchiveSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: UnarchiveName,
		Description: "Restore a previously archived user skill into the active catalog (admin only)." +
			" Moves skills/archived/<id>/ back to skills/user/<id>/; does not delete history.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "Archived skill id to restore",
				},
			},
			"required": []string{"id"},
		},
	}
}

// Unarchiver restores archived skills by id.
type Unarchiver interface {
	UnarchiveUser(id string) error
}

// UnarchiveToolMeta builds unarchive_skill for RegisterSpecApproved(..., true).
func UnarchiveToolMeta(cat Unarchiver) ToolMeta {
	return ToolMeta{
		Spec:    UnarchiveSpec(),
		Invoker: UnarchiveInvoker(cat),
	}
}

// UnarchiveInvoker restores an archived skill after HITL approval.
func UnarchiveInvoker(cat Unarchiver) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		if cat == nil {
			return map[string]any{"error": "skill catalog unavailable"}, true, nil
		}
		if controlplane.RoleFrom(ctx) == controlplane.RoleOperator {
			return map[string]any{"error": "unarchive_skill requires admin role"}, true, nil
		}
		id, _ := args["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			return map[string]any{"error": "id required"}, true, nil
		}
		if err := cat.UnarchiveUser(id); err != nil {
			return map[string]any{"error": err.Error()}, true, nil
		}
		return map[string]any{"ok": true, "id": id}, false, nil
	}
}

// ValidateUnarchiveArgs is a lightweight precheck for HITL.
func ValidateUnarchiveArgs(args map[string]any, _ time.Time) error {
	id, _ := args["id"].(string)
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("id required")
	}
	return nil
}
