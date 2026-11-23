package run

import (
	"errors"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const (
	// EventHITLExpired records an automatic timeout of a waiting_human run.
	EventHITLExpired = "hitl.expired"
	// EventHITLInterrupted records a wait that a restart made unrecoverable.
	EventHITLInterrupted = "hitl.interrupted"

	// HITLExpiredReason is the terminal error/reason for an expired wait.
	HITLExpiredReason = "waiting_expired"
	// HITLExpiredNote is the user-facing system note for an expired wait.
	HITLExpiredNote = "等待人工处理超时，本轮已自动取消。如需继续，请重新发起。"
	// HITLWorkflowRestartNote explains why a workflow approval cannot resume
	// after a process restart.
	HITLWorkflowRestartNote = "服务重启，工作流审批无法恢复，请重新发起"
	// HITLWorkflowStepInterruptedNote explains why a step that was executing
	// when the process stopped is not replayed automatically.
	HITLWorkflowStepInterruptedNote = "工作流步骤执行中断，可能已执行；请核对后再重新发起"

	hitlWaitingSweepLimit = 500
)

// ErrHITLExpired reports that a wait exceeded HITLApprovalTTL and was settled.
var ErrHITLExpired = errors.New(HITLExpiredReason)

// ErrHITLAlreadySettled reports that another replica (or a concurrent decision)
// already claimed or expired this wait; the run must not be finalized again.
var ErrHITLAlreadySettled = errors.New("hitl already settled")

// HITLWaitingSince returns when the run last entered waiting_human. Older rows
// may lack HITLPayload.WaitingSince; fall back to the latest hitl.waiting
// event, then to the run's creation time.
func HITLWaitingSince(st store.Store, runRec *store.Run) time.Time {
	if st == nil || runRec == nil {
		return time.Time{}
	}
	if p, err := st.GetHITL(runRec.ID); err == nil && p != nil && !p.WaitingSince.IsZero() {
		return p.WaitingSince
	}
	if evs, err := st.ListEvents(runRec.ID); err == nil {
		for i := len(evs) - 1; i >= 0; i-- {
			if evs[i].Type == EventHITLWaiting && !evs[i].Timestamp.IsZero() {
				return evs[i].Timestamp
			}
		}
	}
	return runRec.CreatedAt
}

func expirableHITLPayload(p *store.HITLPayload) bool {
	if p == nil {
		return false
	}
	// A parent waiting on a child has its own lifecycle; cancelling it here
	// could strand the child run.
	return p.Kind != store.HITLKindWaitingChild
}

// hitlOverdue reports whether the wait exceeded HITLApprovalTTL.
func (e *Engine) hitlOverdue(runRec *store.Run, p *store.HITLPayload, now time.Time) bool {
	if e.HITLApprovalTTL <= 0 || runRec == nil || !expirableHITLPayload(p) {
		return false
	}
	since := HITLWaitingSince(e.Store, runRec)
	if since.IsZero() {
		return false
	}
	return !now.Before(since.Add(e.HITLApprovalTTL))
}

// ExpireHITL settles one overdue wait. It is idempotent: a run that already
// left waiting_human, or a wait that has not reached the TTL, returns false.
func (e *Engine) ExpireHITL(runID string, now time.Time) (bool, error) {
	if e.Store == nil || e.HITLApprovalTTL <= 0 {
		return false, nil
	}
	e.hitlMu.Lock()
	defer e.hitlMu.Unlock()

	runRec, err := e.Store.GetRun(runID)
	if err != nil {
		return false, err
	}
	if runRec == nil || runRec.Status != store.StatusWaitingHuman {
		return false, nil
	}
	payload, err := e.Store.GetHITL(runID)
	if err != nil {
		return false, err
	}
	if !e.hitlOverdue(runRec, payload, now) {
		return false, nil
	}
	return e.expireHITLLocked(runRec, payload, now, HITLExpiredReason)
}

// SweepExpiredHITL settles every overdue waiting run. It is safe to call on a
// timer; TTL 0 disables it.
func (e *Engine) SweepExpiredHITL(now time.Time) (int, error) {
	if e.Store == nil || e.HITLApprovalTTL <= 0 {
		return 0, nil
	}
	runs, err := e.Store.ListWaitingHumanRuns(hitlWaitingSweepLimit)
	if err != nil {
		return 0, err
	}
	settled := 0
	for _, r := range runs {
		expired, err := e.ExpireHITL(r.ID, now)
		if err != nil {
			return settled, err
		}
		if expired {
			settled++
		}
	}
	return settled, nil
}

// ReconcileHITL runs once at startup:
//   - workflow waits without persisted state cannot resume, so fail them;
//   - workflow waits with persisted state can resume and stay waiting;
//   - waits with no HITL payload cannot be resumed, so settle them;
//   - overdue waits expire.
//
// Resumable ReAct/login/workflow waits stay waiting_human. waiting_child is untouched.
func (e *Engine) ReconcileHITL(now time.Time) (int, error) {
	if e.Store == nil {
		return 0, nil
	}
	runs, err := e.Store.ListWaitingHumanRuns(hitlWaitingSweepLimit)
	if err != nil {
		return 0, err
	}
	settled := 0
	for _, r := range runs {
		payload, err := e.Store.GetHITL(r.ID)
		if err != nil {
			return settled, err
		}
		if payload == nil {
			ok, err := e.settleMissingHITLPayload(r.ID)
			if err != nil {
				return settled, err
			}
			if ok {
				settled++
			}
			continue
		}
		if payload.Kind == store.HITLKindWaitingChild {
			// Parent waits on a child run; do not fail or expire it here.
			continue
		}
		if payload.Workflow == nil {
			evs, err := e.Store.ListEvents(r.ID)
			if err != nil {
				return settled, err
			}
			if workflowInterrupted(evs) {
				ok, err := e.failRestartedWorkflow(r.ID, payload)
				if err != nil {
					return settled, err
				}
				if ok {
					settled++
				}
				continue
			}
		}
		expired, err := e.ExpireHITL(r.ID, now)
		if err != nil {
			return settled, err
		}
		if expired {
			settled++
		}
	}
	return settled, nil
}

// expireHITLLocked settles a waiting run. Caller must hold hitlMu. It returns
// false when another replica already claimed or settled the wait.
func (e *Engine) expireHITLLocked(runRec *store.Run, payload *store.HITLPayload, now time.Time, reason string) (bool, error) {
	won, err := e.Store.ExpireRunIfWaiting(runRec.ID, HITLExpiredReason)
	if err != nil || !won {
		return false, err
	}
	since := HITLWaitingSince(e.Store, runRec)
	waited := now.Sub(since)
	if waited < 0 {
		waited = 0
	}
	data := map[string]any{
		"reason":     reason,
		"waited_sec": int64(waited / time.Second),
	}
	if payload != nil && payload.Kind != "" {
		data["kind"] = payload.Kind
	}
	if err := e.Store.AppendEvent(runRec.ID, store.Event{Type: EventHITLExpired, Data: data}); err != nil {
		return true, err
	}
	_ = e.Store.SetHITL(runRec.ID, nil)
	// Wake a same-process workflow blocked on Gate.Wait without going through
	// Engine.Cancel (which would add run.cancelled and overwrite the reason).
	e.cancelRunContext(runRec.ID)
	e.recordTerminalMessage(runRec.ID)
	return true, nil
}

// settleMissingHITLPayload cancels a wait that has no persisted payload and
// therefore cannot be resumed.
func (e *Engine) settleMissingHITLPayload(runID string) (bool, error) {
	e.hitlMu.Lock()
	defer e.hitlMu.Unlock()
	runRec, err := e.Store.GetRun(runID)
	if err != nil {
		return false, err
	}
	if runRec == nil || runRec.Status != store.StatusWaitingHuman {
		return false, nil
	}
	if p, err := e.Store.GetHITL(runID); err != nil || p != nil {
		return false, err
	}
	return e.expireHITLLocked(runRec, nil, e.now(), "waiting_payload_missing")
}

// failRestartedWorkflow fails a workflow wait whose pipeline state was lost.
func (e *Engine) failRestartedWorkflow(runID string, payload *store.HITLPayload) (bool, error) {
	e.hitlMu.Lock()
	defer e.hitlMu.Unlock()
	runRec, err := e.Store.GetRun(runID)
	if err != nil {
		return false, err
	}
	if runRec == nil || runRec.Status != store.StatusWaitingHuman {
		return false, nil
	}
	won, err := e.Store.FailRunIfWaiting(runID, HITLWorkflowRestartNote)
	if err != nil || !won {
		return false, err
	}
	data := map[string]any{"reason": "workflow_restart"}
	if payload != nil && payload.Kind != "" {
		data["kind"] = payload.Kind
	}
	if err := e.Store.AppendEvent(runID, store.Event{Type: EventHITLInterrupted, Data: data}); err != nil {
		return true, err
	}
	_ = e.Store.SetHITL(runID, nil)
	_ = e.Store.AppendEvent(runID, store.Event{
		Type: EventLLMError,
		Data: map[string]any{"error": HITLWorkflowRestartNote},
	})
	e.recordTerminalMessage(runID)
	return true, nil
}

// cancelRunContext wakes an in-process waiter without changing run state.
func (e *Engine) cancelRunContext(runID string) {
	e.cancelMu.Lock()
	cancel := e.cancels[runID]
	e.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
