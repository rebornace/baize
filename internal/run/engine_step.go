package run

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/skill"
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
	var pkg skill.Package
	for _, id := range st.activated {
		if p, ok := e.Skills.Get(id); ok && p.Workflow != nil {
			wf = p.Workflow
			pkg = p
			break
		}
	}
	if wf == nil {
		return errNoWorkflow
	}
	if err := e.workflowPreflightError(pkg, wf, st.workflowResults); err != nil {
		return e.finalizeFailedRun(runID, err)
	}

	if wf.TimeoutMS > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(wf.TimeoutMS)*time.Millisecond)
		defer cancel()
	}

	e.runMu.Lock()
	st.workflowStarted = true
	st.workflowSkill = wf.Name
	e.runMu.Unlock()

	state, werr := wf.RunFrom(ctx, workflow.State{
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
		return e.finishWorkflowFailure(runID, wf, state.Tree, workflowTimeoutCause(ctx, wf, werr))
	}
	return e.finishWorkflow(ctx, runID, wf, state.Tree)
}

// workflowPreflightError catches missing tools and declared input keys before
// the first step runs, so a domain skill fails with an actionable message
// instead of a tool error mid-pipeline.
func (e *Engine) workflowPreflightError(pkg skill.Package, wf *workflow.Workflow, tree map[string]any) error {
	if err := workflow.ValidateReferences(wf); err != nil {
		return fmt.Errorf("skill %q workflow: %w", pkg.ID, err)
	}
	if e.Tools != nil {
		seen := map[string]bool{}
		var missing []string
		for _, step := range wf.Steps {
			if seen[step.Tool] {
				continue
			}
			seen[step.Tool] = true
			if _, ok := e.Tools.Get(step.Tool); !ok {
				missing = append(missing, step.Tool)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("skill %q requires missing tools: %s", pkg.ID, strings.Join(missing, ", "))
		}
	}
	if len(pkg.RequiresInput) > 0 {
		input, _ := tree["input"].(map[string]any)
		var missing []string
		for _, key := range pkg.RequiresInput {
			if key == "text" {
				continue
			}
			if _, ok := input[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("skill %q requires input_args: %s", pkg.ID, strings.Join(missing, ", "))
		}
	}
	return nil
}

// finishWorkflow records a compact completion event and uses a readable
// summary as the final assistant message. A pipeline that tolerated failed
// steps still succeeds, but reports status "partial".
func (e *Engine) finishWorkflow(ctx context.Context, runID string, wf *workflow.Workflow, tree map[string]any) error {
	content, steps := workflowCompletionSummary(wf, tree)
	status := "succeeded"
	failed := workflowToleratedFailures(wf, tree)
	if failed > 0 {
		status = "partial"
	}
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventWorkflowPrefix + "completed",
		Data: map[string]any{
			"skill":              wf.Name,
			"status":             status,
			"steps":              steps,
			"summary":            content,
			"tolerated_failures": failed,
		},
	})
	return e.finishWorkflowSuccess(ctx, runID, content)
}

// workflowToleratedFailures counts steps that failed but continued because
// the step declared continue_on_error.
func workflowToleratedFailures(wf *workflow.Workflow, tree map[string]any) int {
	n := 0
	for _, step := range wf.Steps {
		if node, ok := tree[step.ID].(map[string]any); ok {
			if _, ok := node["error"].(string); ok {
				n++
			}
		}
	}
	return n
}

// finishWorkflowFailure keeps the same completion payload shape for failed
// runs so clients can render one card and see which step stopped the workflow.
func (e *Engine) finishWorkflowFailure(runID string, wf *workflow.Workflow, tree map[string]any, cause error) error {
	return e.finishWorkflowFailureAt(runID, wf, tree, workflowFailedStep(cause), cause)
}

func (e *Engine) finishWorkflowFailureAt(runID string, wf *workflow.Workflow, tree map[string]any, stepID string, cause error) error {
	content, steps := workflowFailureSummary(wf, tree, stepID)
	safe := errors.New(redactSensitiveText(cause.Error(), 512))
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventWorkflowPrefix + "completed",
		Data: map[string]any{
			"skill":   wf.Name,
			"status":  "failed",
			"steps":   steps,
			"summary": content,
			"error":   redactSensitiveText(cause.Error(), 200),
		},
	})
	return e.finalizeFailedRunWithOutput(runID, content, safe)
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
		return nil, false, fmt.Errorf("%w: %w", workflow.ErrNoRetry, ierr)
	}
	if ierr == nil && isErr && tool.NeedsLoginPause(content) {
		cid := e.connectorIDForTool(toolName)
		if cid == "" {
			if s, ok := content["connector_id"].(string); ok {
				cid = s
			}
		}
		if lerr := e.awaitLoginDecision(ctx, runID, toolName, cid); lerr != nil {
			return nil, false, fmt.Errorf("%w: %w", workflow.ErrNoRetry, lerr)
		}
	}
	return content, isErr, ierr
}

func (e *Engine) persistWorkflowWait(ctx context.Context, runID string, state workflow.State, payload store.HITLPayload) error {
	payload.Workflow = &store.WorkflowState{
		Skill: state.Skill, StepIndex: state.NextStep, StepID: state.StepID, Tree: state.Tree,
		RemainingMS: remainingWorkflowMS(ctx),
	}
	return e.persistHITLWaiting(ctx, runID, &payload)
}

// remainingWorkflowMS snapshots the active workflow budget for a pause. Human
// wait time is excluded because resume starts a fresh deadline with this value.
func remainingWorkflowMS(ctx context.Context) *int64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil
	}
	ms := time.Until(deadline).Milliseconds()
	if ms < 0 {
		ms = 0
	}
	return &ms
}

// workflowTimeoutCause turns a deadline-cancelled step error into an
// actionable message while keeping the failing step id for the summary.
func workflowTimeoutCause(ctx context.Context, wf *workflow.Workflow, err error) error {
	if wf.TimeoutMS <= 0 {
		return err
	}
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if stepID := workflowFailedStep(err); stepID != "" {
		return fmt.Errorf("step %q: workflow timed out after %dms", stepID, wf.TimeoutMS)
	}
	return fmt.Errorf("workflow timed out after %dms", wf.TimeoutMS)
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
	return e.finalizeFailedRunWithOutput(runID, "", cause)
}

func (e *Engine) finalizeFailedRunWithOutput(runID, output string, cause error) error {
	// A concurrent cancel/expiry may have settled the run already. Do not
	// overwrite its terminal state or record a second system note.
	if cur, err := e.Store.GetRun(runID); err == nil && cur != nil {
		switch cur.Status {
		case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusRejected:
			return cause
		}
	}
	errMsg := redactSensitiveText(cause.Error(), 0)
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMError,
		Data: map[string]any{"error": errMsg},
	})
	_ = e.Store.UpdateRun(runID, store.StatusFailed, output, errMsg)
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
