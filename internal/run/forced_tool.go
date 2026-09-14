package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/rebornace/baize/internal/store"
)

// ExecuteForcedTool runs one tool for an existing run: run.started already appended by API.
// skipLoginGate=true when appending tool call; still honors require_approval via awaitHITL.
// On completion: StatusSucceeded (即使 tool is_error)；HITL reject 走既有 rejected。
// Output 可空（不写助手长文）；依赖 events 给 Chat。
func (e *Engine) ExecuteForcedTool(ctx context.Context, runID, toolName string, args map[string]any) error {
	runRec, err := e.Store.GetRun(runID)
	if err != nil {
		return err
	}
	if runRec == nil {
		return fmt.Errorf("run not found: %s", runID)
	}

	ctx, cancel := context.WithCancel(ctx)
	e.registerCancel(runID, cancel)
	defer e.clearCancel(runID)

	ctx = e.injectAuthCtxFromRun(ctx, runRec)
	if err := e.ensureRunStarted(runID); err != nil {
		return err
	}

	callID := fmt.Sprintf("forced-%s", toolName)
	_, _, invErr := e.invokeTool(ctx, runID, callID, toolName, args, false, true)
	if invErr != nil {
		if errors.Is(invErr, ErrHITLRejected) {
			return invErr
		}
		if errors.Is(invErr, context.Canceled) || e.isCancelled(runID) {
			e.markCancelled(runID)
			return context.Canceled
		}
		return e.finalizeFailedRun(runID, invErr)
	}

	if err := e.Store.UpdateRun(runID, store.StatusSucceeded, "", ""); err != nil {
		return err
	}
	e.recordTerminalMessage(runID)
	return nil
}
