package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// InvokeFunc invokes one registered tool for a workflow step. Mirrors
// tool.Registry.Invoke: isErr=true means the invocation ran but reported a
// tool-level error. stepID lets the engine mint a stable per-step call id.
type InvokeFunc func(ctx context.Context, tool string, stepID string, args map[string]any) (map[string]any, bool, error)

// EmitFunc persists one event; the engine wraps store.AppendEvent.
type EmitFunc func(typ string, data map[string]any) error

// GateFunc routes an Approve-gated step through HITL; approved=false stops Run.
// It is kept for callers of Run that do not need resumable state.
type GateFunc func(ctx context.Context, payload store.HITLPayload) (approved bool, err error)

// GateStateFunc is the resumable gate. It receives the current state so the
// engine can persist skill/step/tree before returning a pause error.
type GateStateFunc func(ctx context.Context, payload store.HITLPayload, state State) (approved bool, err error)

// State is the resumable position of a linear workflow. NextStep is the index
// of the next step to run; Tree holds the input plus completed step results.
type State struct {
	Skill    string
	NextStep int
	StepID   string
	Tree     map[string]any
}

// ExecHooks carries the engine-provided collaborators for Run/RunFrom.
type ExecHooks struct {
	Emit      EmitFunc
	Gate      GateFunc
	GateState GateStateFunc
	Invoke    InvokeFunc
}

// ErrRejected is returned (wrapped) when a step was rejected at the HITL gate.
var ErrRejected = errors.New("rejected")

// ErrNoRetry marks failures that must not be retried (HITL pauses, checks
// that need a human, or cancellation). The engine wraps those errors so the
// retry loop can stop while callers still see the original sentinel.
var ErrNoRetry = errors.New("workflow: no retry")

// Rejected reports whether err was caused by a human rejecting a workflow step.
func Rejected(err error) bool {
	return errors.Is(err, ErrRejected)
}

// tolerableStepFailure reports whether a failed attempt may be recorded and
// skipped. Human pauses/rejections and cancellation always stop the workflow.
func tolerableStepFailure(s Step, err error, ctx context.Context) bool {
	if !s.ContinueOnError || ctx.Err() != nil {
		return false
	}
	if err != nil && errors.Is(err, ErrNoRetry) {
		return false
	}
	return true
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

// Run executes the workflow from the beginning. It remains the compatible
// entry point for callers that do not resume workflows.
func (w *Workflow) Run(ctx context.Context, tree map[string]any, h ExecHooks) error {
	_, err := w.RunFrom(ctx, State{Skill: w.Name, NextStep: 0, Tree: tree}, h)
	return err
}

// RunFrom executes the linear steps from state.NextStep. A GateState error
// (for example the engine's waiting-human sentinel) stops execution and is
// returned wrapped with the failing step id; the caller persists the state
// from the GateState callback before returning it to the worker.
func (w *Workflow) RunFrom(ctx context.Context, state State, h ExecHooks) (State, error) {
	ids := make([]string, 0, len(w.Steps))
	for _, s := range w.Steps {
		ids = append(ids, s.ID)
	}
	if state.NextStep < 0 {
		state.NextStep = 0
	}
	if state.NextStep > len(w.Steps) {
		state.NextStep = len(w.Steps)
	}
	if state.NextStep == 0 {
		if err := h.Emit("workflow.started", map[string]any{"skill": w.Name, "steps": ids}); err != nil {
			return state, err
		}
	} else {
		if err := h.Emit("workflow.resumed", map[string]any{"skill": w.Name, "next_step": state.NextStep}); err != nil {
			return state, err
		}
	}

	tree := state.Tree
	if tree == nil {
		tree = map[string]any{}
	}
	gate := h.GateState
	if gate == nil && h.Gate != nil {
		gate = func(ctx context.Context, p store.HITLPayload, _ State) (bool, error) {
			return h.Gate(ctx, p)
		}
	}

	for i := state.NextStep; i < len(w.Steps); i++ {
		s := w.Steps[i]
		if s.When != nil {
			ok, cerr := EvalCondition(s.When, tree)
			if cerr != nil {
				return state, fmt.Errorf("step %q condition: %w", s.ID, cerr)
			}
			if !ok {
				if err := h.Emit("workflow.step_skipped", map[string]any{"step": s.ID, "tool": s.Tool}); err != nil {
					return state, err
				}
				continue
			}
		}
		args, rerr := TryRenderArgs(s.Args, tree)
		if rerr != nil {
			return state, fmt.Errorf("step %q: %w", s.ID, rerr)
		}

		if s.Approve {
			if gate == nil {
				return state, fmt.Errorf("step %q: missing gate", s.ID)
			}
			approved, gerr := gate(ctx, store.HITLPayload{
				Prompt:    fmt.Sprintf("workflow step %q: %s", s.ID, s.Tool),
				ToolName:  s.Tool,
				Arguments: args,
			}, State{Skill: w.Name, NextStep: i, StepID: s.ID, Tree: tree})
			if gerr != nil {
				return state, fmt.Errorf("step %q: %w", s.ID, gerr)
			}
			if !approved {
				return state, fmt.Errorf("workflow step %q: %w", s.ID, ErrRejected)
			}
		}

		if err := h.Emit("workflow.step_started", map[string]any{"step": s.ID, "tool": s.Tool}); err != nil {
			return state, err
		}

		invokeCtx := WithStepState(ctx, State{
			Skill: w.Name, NextStep: i, StepID: s.ID, Tree: tree,
		})
		attempts := s.Retry + 1
		var content map[string]any
		var isErr bool
		var ierr error
		timedOut := false
		for attempt := 1; attempt <= attempts; attempt++ {
			attemptCtx := invokeCtx
			cancel := func() {}
			if s.TimeoutMS > 0 {
				attemptCtx, cancel = context.WithTimeout(invokeCtx, time.Duration(s.TimeoutMS)*time.Millisecond)
			}
			content, isErr, ierr = h.Invoke(attemptCtx, s.Tool, s.ID, args)
			timedOut = s.TimeoutMS > 0 && errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
			cancel()
			if timedOut {
				if err := h.Emit("workflow.step_timeout", map[string]any{
					"step":       s.ID,
					"tool":       s.Tool,
					"attempt":    attempt,
					"timeout_ms": s.TimeoutMS,
				}); err != nil {
					return state, err
				}
			}
			if ierr == nil && !isErr {
				break
			}
			if ierr != nil && (errors.Is(ierr, ErrNoRetry) || ctx.Err() != nil) {
				break
			}
			if attempt < attempts {
				if err := h.Emit("workflow.step_retry", map[string]any{
					"step":         s.ID,
					"tool":         s.Tool,
					"attempt":      attempt,
					"next_attempt": attempt + 1,
					"max_attempts": attempts,
					"tool_error":   isErr,
					"timed_out":    timedOut,
				}); err != nil {
					return state, err
				}
			}
		}
		failKind := ""
		switch {
		case timedOut:
			failKind = "timeout"
		case ierr != nil:
			failKind = "invoke_error"
		case isErr:
			failKind = "tool_error"
		}
		if failKind != "" {
			if !tolerableStepFailure(s, ierr, ctx) {
				if timedOut {
					return state, fmt.Errorf("step %q: timed out after %dms", s.ID, s.TimeoutMS)
				}
				if ierr != nil {
					// Preserve sentinels (for example the engine's waiting-human pause)
					// so callers can distinguish a pause from a real failure.
					return state, fmt.Errorf("step %q: invoke failed: %w", s.ID, ierr)
				}
				return state, fmt.Errorf("step %q: invoke failed: tool returned is_error", s.ID)
			}
			SetResult(tree, s.ID, map[string]any{"is_error": true})
			if node, ok := tree[s.ID].(map[string]any); ok {
				node["error"] = failKind
			}
			if err := h.Emit("workflow.step_failed", map[string]any{
				"step":       s.ID,
				"tool":       s.Tool,
				"error_kind": failKind,
				"tolerated":  true,
			}); err != nil {
				return state, err
			}
			// Close the started step so a cold resume is not treated as an
			// interrupted mid-step; is_error marks the tolerated failure.
			if err := h.Emit("workflow.step_completed", map[string]any{
				"step":      s.ID,
				"is_error":  true,
				"tolerated": true,
			}); err != nil {
				return state, err
			}
			continue
		}

		SetResult(tree, s.ID, content)

		if err := h.Emit("workflow.step_completed", map[string]any{"step": s.ID, "is_error": false}); err != nil {
			return state, err
		}
	}

	state.NextStep = len(w.Steps)
	state.Tree = tree
	return state, nil
}
