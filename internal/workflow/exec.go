package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/rebornace/baize/internal/store"
)

// InvokeFunc invokes one registered tool for a workflow step. Mirrors
// tool.Registry.Invoke: isErr=true means the invocation ran but reported a
// tool-level error. stepID lets the engine mint a stable per-step call id.
type InvokeFunc func(ctx context.Context, tool string, stepID string, args map[string]any) (map[string]any, bool, error)

// EmitFunc persists one event; the engine wraps store.AppendEvent.
type EmitFunc func(typ string, data map[string]any) error

// GateFunc routes an Approve-gated step through HITL; approved=false stops Run.
type GateFunc func(ctx context.Context, payload store.HITLPayload) (approved bool, err error)

// ExecHooks carries the engine-provided collaborators for Run.
type ExecHooks struct {
	Emit   EmitFunc
	Gate   GateFunc
	Invoke InvokeFunc
}

// ErrRejected is returned (wrapped) when a step was rejected at the HITL gate.
var ErrRejected = errors.New("rejected")

// Rejected reports whether err was caused by a human rejecting a workflow step.
func Rejected(err error) bool {
	return errors.Is(err, ErrRejected)
}

// SetResult writes one completed step's output into tree as "<id>.result"
// (a nested map), matching Resolve's dot-path descent so later steps can
// reference it via {{<id>.result[...]}}.
func SetResult(tree map[string]any, stepID string, result map[string]any) {
	node, ok := tree[stepID].(map[string]any)
	if !ok {
		node = map[string]any{}
		tree[stepID] = node
	}
	node["result"] = result
}

// Run executes the steps linearly against tree. Args are rendered just before
// each step, and each completed step writes its "<id>.result" back into tree,
// so later steps can reference earlier results ({{a.result.ok}}). Any failure —
// unresolved reference, gate rejection or error, invoke error — aborts Run with
// an error naming the failing step.
func (w *Workflow) Run(ctx context.Context, tree map[string]any, h ExecHooks) error {
	ids := make([]string, 0, len(w.Steps))
	for _, s := range w.Steps {
		ids = append(ids, s.ID)
	}
	if err := h.Emit("workflow.started", map[string]any{"skill": w.Name, "steps": ids}); err != nil {
		return err
	}

	for _, s := range w.Steps {
		args, rerr := TryRenderArgs(s.Args, tree)
		if rerr != nil {
			return fmt.Errorf("step %q: %w", s.ID, rerr)
		}

		if s.Approve {
			approved, gerr := h.Gate(ctx, store.HITLPayload{
				Prompt:    fmt.Sprintf("workflow step %q: %s", s.ID, s.Tool),
				ToolName:  s.Tool,
				Arguments: args,
			})
			if gerr != nil {
				return fmt.Errorf("step %q: %w", s.ID, gerr)
			}
			if !approved {
				return fmt.Errorf("workflow step %q: %w", s.ID, ErrRejected)
			}
		}

		if err := h.Emit("workflow.step_started", map[string]any{"step": s.ID, "tool": s.Tool}); err != nil {
			return err
		}

		content, isErr, ierr := h.Invoke(ctx, s.Tool, s.ID, args)
		if ierr != nil || isErr {
			detail := "tool returned is_error"
			if ierr != nil {
				detail = ierr.Error()
			}
			return fmt.Errorf("step %q: invoke failed: %s", s.ID, detail)
		}

		SetResult(tree, s.ID, content)

		if err := h.Emit("workflow.step_completed", map[string]any{"step": s.ID, "is_error": false}); err != nil {
			return err
		}
	}
	return nil
}
