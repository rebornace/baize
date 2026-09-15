package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/workflow"
)

var errNoWorkflow = errors.New("run has no workflow")

func beginRunInput(input string) map[string]any {
	return map[string]any{"text": input}
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

	stepApproved := make(map[string]bool, len(wf.Steps))
	for _, s := range wf.Steps {
		stepApproved[s.ID] = s.Approve
	}

	werr := wf.Run(ctx, st.workflowResults, workflow.ExecHooks{
		Emit: func(typ string, data map[string]any) error {
			return e.Store.AppendEvent(runID, store.Event{Type: typ, Data: data})
		},
		Gate: func(gctx context.Context, p store.HITLPayload) (bool, error) {
			if err := e.awaitHITLPayload(gctx, runID, p.Prompt, p.ToolName, p.Arguments); err != nil {
				if errors.Is(err, ErrHITLRejected) {
					return false, nil
				}
				return false, err
			}
			return true, nil
		},
		Invoke: func(ictx context.Context, toolName string, stepID string, args map[string]any) (map[string]any, bool, error) {
			callID := fmt.Sprintf("wf-%s-%s", st.workflowSkill, stepID)
			return e.invokeTool(ictx, runID, callID, toolName, args, stepApproved[stepID])
		},
	})
	if werr != nil {
		if workflow.Rejected(werr) {
			e.finalizeRejectedRun(runID)
			return ErrHITLRejected
		}
		return e.finalizeFailedRun(runID, werr)
	}

	content := "workflow completed"
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
