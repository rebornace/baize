package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/workflow"
)

func (e *Engine) workflowBySkill(id string) *workflow.Workflow {
	if e.Skills == nil {
		return nil
	}
	pkg, ok := e.Skills.Get(id)
	if !ok {
		return nil
	}
	return pkg.Workflow
}

// continueWorkflowFromHITL resumes a paused workflow step after a human
// decision. Approval and login waits share the same persisted position: the
// current step is executed once, its result is written into the tree, and
// execution continues from the next step. A login wait retries the same step;
// an approval wait acknowledges it first.
func (e *Engine) continueWorkflowFromHITL(ctx context.Context, runID string, payload *store.HITLPayload, d Decision) error {
	if payload == nil || payload.Workflow == nil {
		return e.finalizeFailedRun(runID, errors.New("workflow state missing; please re-run"))
	}
	wf := e.workflowBySkill(payload.Workflow.Skill)
	if wf == nil {
		_ = e.Store.SetHITL(runID, nil)
		return e.finalizeFailedRun(runID, fmt.Errorf("workflow %q is no longer available; please re-run", payload.Workflow.Skill))
	}
	stepIndex := payload.Workflow.StepIndex
	if stepIndex < 0 || stepIndex >= len(wf.Steps) {
		_ = e.Store.SetHITL(runID, nil)
		return e.finalizeFailedRun(runID, fmt.Errorf("workflow %q step index %d is out of range; please re-run", wf.Name, stepIndex))
	}
	step := wf.Steps[stepIndex]
	if payload.Workflow.StepID != "" && step.ID != payload.Workflow.StepID {
		_ = e.Store.SetHITL(runID, nil)
		return e.finalizeFailedRun(runID, fmt.Errorf("workflow %q changed at step %q; please re-run", wf.Name, payload.Workflow.StepID))
	}

	if !d.Approve {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventHITLRejected,
			Data: map[string]any{"decision": "reject", "comment": d.Comment, "kind": payload.Kind},
		})
		e.finalizeRejectedRun(runID)
		return nil
	}

	if wf.TimeoutMS > 0 {
		remaining := time.Duration(wf.TimeoutMS) * time.Millisecond
		if payload.Workflow.RemainingMS != nil {
			if *payload.Workflow.RemainingMS <= 0 {
				_ = e.Store.SetHITL(runID, nil)
				return e.finishWorkflowFailureAt(runID, wf, payload.Workflow.Tree, step.ID,
					fmt.Errorf("workflow timed out after %dms", wf.TimeoutMS))
			}
			remaining = time.Duration(*payload.Workflow.RemainingMS) * time.Millisecond
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, remaining)
		defer cancel()
	}

	kind := payload.Kind
	if kind == "" {
		kind = store.HITLKindApproval
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLResumed,
		Data: map[string]any{"decision": "approve", "comment": d.Comment, "kind": kind},
	})
	_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")
	e.notifyParentChildResumed(runID)

	// The gate pauses before workflow.step_started, so emit the start here. A
	// step that already emitted it before a login/approval pause keeps the
	// existing event instead of duplicating it.
	if evs, lerr := e.Store.ListEvents(runID); lerr != nil || !openWorkflowSteps(evs)[step.ID] {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: "workflow.step_started",
			Data: map[string]any{"step": step.ID, "tool": step.Tool},
		})
	}

	tree := payload.Workflow.Tree
	if tree == nil {
		tree = map[string]any{}
	}
	args, rerr := workflow.TryRenderArgs(step.Args, tree)
	if rerr != nil {
		e.emitWorkflowStepCompleted(runID, step.ID, true)
		_ = e.Store.SetHITL(runID, nil)
		return e.finalizeFailedRun(runID, fmt.Errorf("workflow step %q: %w", step.ID, rerr))
	}

	// An approve-gated step was already approved; an approval wait acknowledges
	// the tool's own gate. A login wait on a plain step keeps the tool's own
	// approval check so security gates are not bypassed.
	skipApproval := step.Approve || kind == store.HITLKindApproval
	stepCtx := workflow.WithStepState(ctx, workflow.State{
		Skill: wf.Name, NextStep: stepIndex, StepID: step.ID, Tree: tree,
	})
	content, isErr, invErr := e.invokeWorkflowStep(stepCtx, runID, wf.Name, step.ID, step.Tool, args, skipApproval)
	if errors.Is(invErr, ErrWaitingHuman) {
		return ErrWaitingHuman
	}
	if wf.TimeoutMS > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		e.emitWorkflowStepCompleted(runID, step.ID, true)
		_ = e.Store.SetHITL(runID, nil)
		return e.finishWorkflowFailureAt(runID, wf, tree, step.ID,
			fmt.Errorf("workflow timed out after %dms", wf.TimeoutMS))
	}
	if invErr != nil || isErr {
		detail := "tool returned is_error"
		if invErr != nil {
			detail = invErr.Error()
		}
		e.emitWorkflowStepCompleted(runID, step.ID, true)
		_ = e.Store.SetHITL(runID, nil)
		return e.finishWorkflowFailureAt(runID, wf, tree, step.ID, fmt.Errorf("workflow step %q: %s", step.ID, detail))
	}
	workflow.SetResult(tree, step.ID, content)
	e.emitWorkflowStepCompleted(runID, step.ID, false)
	_ = e.Store.SetHITL(runID, nil)

	state, werr := wf.RunFrom(ctx, workflow.State{
		Skill: wf.Name, NextStep: stepIndex + 1, Tree: tree,
	}, e.workflowHooks(runID, wf))
	if errors.Is(werr, ErrWaitingHuman) {
		return ErrWaitingHuman
	}
	if werr != nil {
		if workflow.Rejected(werr) {
			e.finalizeRejectedRun(runID)
			return ErrHITLRejected
		}
		return e.finishWorkflowFailure(runID, wf, state.Tree, workflowTimeoutCause(ctx, wf, werr))
	}
	return e.finishWorkflow(ctx, runID, wf, state.Tree)
}

func (e *Engine) emitWorkflowStepCompleted(runID, stepID string, isErr bool) {
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: "workflow.step_completed",
		Data: map[string]any{"step": stepID, "is_error": isErr},
	})
}
