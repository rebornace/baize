package run

import (
	"context"
	"errors"
	"fmt"

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
// decision. The approved step is executed once, its result is written into the
// persisted tree, and execution continues from the next step.
func (e *Engine) continueWorkflowFromHITL(ctx context.Context, runID string, payload *store.HITLPayload, d Decision) error {
	wf := e.workflowBySkill(payload.Workflow.Skill)
	if wf == nil {
		_ = e.Store.SetHITL(runID, nil)
		return e.finalizeFailedRun(runID, fmt.Errorf("workflow %q is no longer available; please re-run", payload.Workflow.Skill))
	}
	if !d.Approve {
		_ = e.Store.AppendEvent(runID, store.Event{
			Type: EventHITLRejected,
			Data: map[string]any{"decision": "reject", "comment": d.Comment},
		})
		e.finalizeRejectedRun(runID)
		return nil
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventHITLResumed,
		Data: map[string]any{"decision": "approve", "comment": d.Comment, "kind": payload.Kind},
	})
	_ = e.Store.UpdateRun(runID, store.StatusRunning, "", "")

	stepID := payload.Workflow.StepID
	if stepID == "" && payload.Workflow.StepIndex >= 0 && payload.Workflow.StepIndex < len(wf.Steps) {
		stepID = wf.Steps[payload.Workflow.StepIndex].ID
	}
	content, isErr, invErr := e.invokeTool(ctx, runID,
		fmt.Sprintf("wf-%s-%s", wf.Name, stepID), payload.ToolName, payload.Arguments, true)
	if invErr != nil || isErr {
		detail := "tool returned is_error"
		if invErr != nil {
			detail = invErr.Error()
		}
		_ = e.Store.SetHITL(runID, nil)
		return e.finalizeFailedRun(runID, fmt.Errorf("workflow step %q: %s", stepID, detail))
	}
	tree := payload.Workflow.Tree
	if tree == nil {
		tree = map[string]any{}
	}
	workflow.SetResult(tree, stepID, content)
	_ = e.Store.SetHITL(runID, nil)

	_, werr := wf.RunFrom(ctx, workflow.State{
		Skill: wf.Name, NextStep: payload.Workflow.StepIndex + 1, Tree: tree,
	}, e.workflowHooks(runID, wf))
	if errors.Is(werr, ErrWaitingHuman) {
		return ErrWaitingHuman
	}
	if werr != nil {
		if workflow.Rejected(werr) {
			e.finalizeRejectedRun(runID)
			return ErrHITLRejected
		}
		return e.finalizeFailedRun(runID, werr)
	}
	return e.finishWorkflowSuccess(ctx, runID, "workflow completed")
}
