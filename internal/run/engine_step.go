package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workflow"
)

var errNoWorkflow = errors.New("run has no workflow")

func beginRunInput(input string, args map[string]any) map[string]any {
	out := map[string]any{"text": input}
	for k, v := range args {
		if k == "" || k == "text" {
			continue
		}
		out[k] = v
	}
	return out
}

// maybeRunWorkflow switches a run into pipeline mode when the just-activated
// skill carries a workflow.yaml, executes it to completion, and finalizes the
// run. It returns errNoWorkflow when no workflow applies.
func (e *Engine) maybeRunWorkflow(ctx context.Context, runID string) error {
	st := e.getRunSkillState(runID)
	if st == nil || st.workflowStarted || e.Skills == nil {
		return errNoWorkflow
	}
	var wf *workflow.Workflow
	for _, id := range st.activated {
		if pkg, ok := e.Skills.Get(id); ok && pkg.Workflow != nil {
			wf = pkg.Workflow
			break
		}
	}
	if wf == nil {
		return errNoWorkflow
	}

	e.runMu.Lock()
	st.workflowStarted = true
	st.workflowSkill = wf.Name
	e.runMu.Unlock()

	_, werr := wf.RunFrom(ctx, workflow.State{
		Skill: wf.Name, NextStep: 0, Tree: st.workflowResults,
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

// workflowHooks wires workflow steps into the engine. Approve steps pause by
// persisting the workflow state and returning ErrWaitingHuman; the worker
// releases its slot instead of blocking on a gate.
func (e *Engine) workflowHooks(runID string, wf *workflow.Workflow) workflow.ExecHooks {
	stepApproved := make(map[string]bool, len(wf.Steps))
	for _, s := range wf.Steps {
		stepApproved[s.ID] = s.Approve
	}
	return workflow.ExecHooks{
		Emit: func(typ string, data map[string]any) error {
			return e.Store.AppendEvent(runID, store.Event{Type: typ, Data: data})
		},
		GateState: func(ctx context.Context, payload store.HITLPayload, state workflow.State) (bool, error) {
			if err := e.persistWorkflowWait(ctx, runID, state, payload); err != nil {
				return false, err
			}
			return false, ErrWaitingHuman
		},
		Invoke: func(ctx context.Context, toolName, stepID string, args map[string]any) (map[string]any, bool, error) {
			return e.invokeWorkflowStep(ctx, runID, wf.Name, stepID, toolName, args, stepApproved[stepID])
		},
	}
}

// invokeWorkflowStep runs one workflow step and turns a login gate into a
// resumable HITL pause. The step state is carried by ctx, so the wait payload
// remembers where to resume.
func (e *Engine) invokeWorkflowStep(ctx context.Context, runID, wfName, stepID, toolName string, args map[string]any, skipApproval bool) (map[string]any, bool, error) {
	content, isErr, ierr := e.invokeTool(ctx, runID,
		fmt.Sprintf("wf-%s-%s", wfName, stepID), toolName, args, skipApproval)
	if errors.Is(ierr, ErrWaitingHuman) {
		return nil, false, ierr
	}
	if ierr == nil && isErr && tool.NeedsLoginPause(content) {
		cid := e.connectorIDForTool(toolName)
		if cid == "" {
			if s, ok := content["connector_id"].(string); ok {
				cid = s
			}
		}
		if lerr := e.awaitLoginDecision(ctx, runID, toolName, cid); lerr != nil {
			return nil, false, lerr
		}
	}
	return content, isErr, ierr
}

func (e *Engine) persistWorkflowWait(ctx context.Context, runID string, state workflow.State, payload store.HITLPayload) error {
	payload.Workflow = &store.WorkflowState{
		Skill: state.Skill, StepIndex: state.NextStep, StepID: state.StepID, Tree: state.Tree,
	}
	return e.persistHITLWaiting(ctx, runID, &payload)
}

func (e *Engine) finishWorkflowSuccess(ctx context.Context, runID, content string) error {
	_ = e.Store.SetHITL(runID, nil)
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMMessage,
		Data: map[string]any{"content": content},
	})
	if err := e.Store.UpdateRun(runID, store.StatusSucceeded, content, ""); err != nil {
		return err
	}
	e.recordTerminalMessage(runID)
	e.triggerMemoryExtract(ctx, runID, content)
	return nil
}

func (e *Engine) triggerMemoryExtract(ctx context.Context, runID, output string) {
	runRec, err := e.Store.GetRun(runID)
	if err != nil || runRec == nil {
		return
	}
	owner := e.memoryOwner(runRec.ConversationID)
	e.maybeExtractMemory(ctx, runID, owner, runRec.Input, output)
}

func (e *Engine) finalizeFailedRun(runID string, cause error) error {
	// A concurrent cancel/expiry may have settled the run already. Do not
	// overwrite its terminal state or record a second system note.
	if cur, err := e.Store.GetRun(runID); err == nil && cur != nil {
		switch cur.Status {
		case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
			return cause
		}
	}
	errMsg := cause.Error()
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMError,
		Data: map[string]any{"error": errMsg},
	})
	_ = e.Store.UpdateRun(runID, store.StatusFailed, "", errMsg)
	e.recordTerminalMessage(runID)
	return cause
}

// finalizeRejectedRun settles a run a human declined at an approval gate.
// Unlike a failure it records the neutral "rejected" status (no technical
// error reason) and a user-facing system note, so the chat never shows
// "运行失败：hitl rejected".
func (e *Engine) finalizeRejectedRun(runID string) {
	// Idempotent: the live workflow gate rejects inside waitForDecision (run
	// already settled) before maybeRunWorkflow observes the wrapped error; do
	// not append a second system note on that second pass.
	if rec, err := e.Store.GetRun(runID); err == nil && rec != nil && rec.Status == store.StatusRejected {
		return
	}
	_ = e.Store.SetHITL(runID, nil)
	_ = e.Store.UpdateRun(runID, store.StatusRejected, "", "")
	e.recordTerminalMessage(runID)
}

// workflowStepInterrupted reports whether a workflow step started but never
// completed and the run has no terminal marker. A running run in this state
// stopped mid-step (process exit), so replaying it could repeat side effects.
func workflowStepInterrupted(evs []store.Event) bool {
	return len(openWorkflowSteps(evs)) > 0
}

// openWorkflowSteps returns steps whose workflow.step_started has no matching
// workflow.step_completed (and no terminal marker reset).
func openWorkflowSteps(evs []store.Event) map[string]bool {
	open := map[string]bool{}
	for _, ev := range evs {
		switch ev.Type {
		case "workflow.step_started":
			if id, _ := ev.Data["step"].(string); id != "" {
				open[id] = true
			}
		case "workflow.step_completed":
			if id, _ := ev.Data["step"].(string); id != "" {
				delete(open, id)
			}
		case "workflow.completed", EventLLMMessage, EventLLMError, EventRunCancelled:
			open = map[string]bool{}
		}
	}
	return open
}

// workflowInterrupted reports whether the event stream shows a workflow run
// that started but never reached a terminal event (llm.message completion or
// llm.error failure) — i.e. a cold resume would land mid-pipeline.
func workflowInterrupted(evs []store.Event) bool {
	started := false
	for _, ev := range evs {
		switch ev.Type {
		case EventWorkflowPrefix + "started":
			started = true
		case "workflow.completed", EventLLMMessage, EventLLMError:
			started = false
		}
	}
	return started
}
