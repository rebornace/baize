package schedule

import (
	"errors"
	"time"

	storepkg "github.com/rebornace/baize/internal/store"
)

const (
	errConversationNotFound = "schedule: conversation not found"
	errOwnerMismatch        = "schedule: conversation owner mismatch"
	errOpenRunFailed        = "schedule: failed to open run"
)

// errConversationBusy means the target conversation already has an active run.
// The tick is skipped quietly: release the claim, push the retry out by one
// scan interval, and try again on a later scan.
var errConversationBusy = errors.New("schedule: conversation busy")

// busyRetryDelay spaces out retries while the conversation is busy. It matches
// the default 15s scanner interval (bootstrap.scheduleScanInterval) and keeps
// the claim loop from re-picking the same row within one scan.
const busyRetryDelay = 15 * time.Second

// RunOpener starts a scheduled run for a conversation.
type RunOpener interface {
	Open(conversationID, instruction, ownerID string) (runID string, err error)
}

// RunStatus reports a run's terminal state. status is meaningful when
// terminal is true (for example "failed" or "succeeded").
type RunStatus func(runID string) (status string, terminal bool, err error)

type scanLocker interface {
	scanLock()
	scanUnlock()
}

// Scan claims due schedules, opens runs, and advances recurring next_run_at values.
func Scan(store Store, meta MetaReader, opener RunOpener, runStatus RunStatus, now time.Time) error {
	if store == nil {
		return errors.New("schedule: store required")
	}
	if sl, ok := store.(scanLocker); ok {
		sl.scanLock()
		defer sl.scanUnlock()
	}
	// Reclaim claims abandoned by a crashed replica. Healthy in-flight claims
	// carry a future lease and are left alone, so this is safe every tick.
	if err := store.ClearStalePending(now); err != nil {
		return err
	}
	if err := reconcileRunningRuns(store, runStatus, now); err != nil {
		return err
	}
	if err := advanceInFlightDue(store, now); err != nil {
		return err
	}
	pendingFail := map[string]string{}
	defer func() {
		for id, msg := range pendingFail {
			_ = store.FailOpen(id, msg)
		}
	}()
	for {
		it, ok, err := store.ClaimDue(now)
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		softMsg, err := processClaimed(store, meta, opener, now, it)
		if errors.Is(err, errConversationBusy) {
			// Busy is expected: release the pending claim and retry next tick.
			_ = store.FailOpen(it.ID, "")
			_ = store.AdvanceNextRunAt(it.ID, now.Add(busyRetryDelay))
			continue
		}
		if err != nil {
			return err
		}
		if softMsg != "" {
			pendingFail[it.ID] = softMsg
		}
	}
	return nil
}

func reconcileRunningRuns(store Store, runStatus RunStatus, now time.Time) error {
	if runStatus == nil {
		return nil
	}
	rows, err := store.ListWithRunningRun()
	if err != nil {
		return err
	}
	for _, it := range rows {
		if it.RunningRunID == "" || it.RunningRunID == ClaimPending {
			continue
		}
		status, terminal, err := runStatus(it.RunningRunID)
		if err != nil || !terminal {
			continue
		}
		if err := finishScheduledTerminal(store, it, status, now); err != nil {
			return err
		}
	}
	return nil
}

// retryDelay is exponential: 1m, 2m, 4m, capped at 10m.
func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Minute << (attempt - 1)
	if d > 10*time.Minute {
		d = 10 * time.Minute
	}
	return d
}

// finishScheduledTerminal applies retry/backoff or the post-run status. Retry
// writes are conditional on the run id, so two replicas cannot consume the
// same retry twice.
func finishScheduledTerminal(store Store, it Item, status string, now time.Time) error {
	// A user pause during an in-flight run wins: release the run and keep the
	// schedule paused instead of applying retry/cadence.
	if it.Status == StatusPaused {
		_, err := store.FinishScheduledRun(it.ID, it.RunningRunID, StatusPaused, it.NextRunAt, it.LastError)
		return err
	}
	failed := status == string(storepkg.StatusFailed)
	if failed && it.MaxRetries > 0 && it.RetryCount < it.MaxRetries {
		next := now.UTC().Add(retryDelay(it.RetryCount + 1))
		_, err := store.ScheduleRetry(it.ID, it.RunningRunID, it.RetryCount+1, next, "scheduled run failed")
		return err
	}
	next := it.NextRunAt
	lastErr := ""
	if failed {
		lastErr = "scheduled run failed"
		if it.MaxRetries > 0 {
			lastErr = "scheduled run failed; retries exhausted"
		}
	}
	var st string
	switch it.Kind {
	case "once", KindFollowup:
		st = StatusCompleted
	default:
		st = StatusActive
		if n, err := Next(itemDraft(it), now); err == nil {
			next = n
		}
	}
	_, err := store.FinishScheduledRun(it.ID, it.RunningRunID, st, next, lastErr)
	return err
}

func advanceInFlightDue(store Store, now time.Time) error {
	rows, err := store.ListWithRunningRun()
	if err != nil {
		return err
	}
	for _, it := range rows {
		if it.RunningRunID == "" || it.RunningRunID == ClaimPending {
			continue
		}
		if it.Status != StatusActive {
			continue
		}
		if it.Kind != "daily" && it.Kind != "weekly" {
			continue
		}
		if it.NextRunAt.After(now) {
			continue
		}
		next, err := Next(itemDraft(it), now)
		if err != nil {
			continue
		}
		if err := store.AdvanceNextRunAt(it.ID, next); err != nil {
			return err
		}
	}
	return nil
}

func processClaimed(store Store, meta MetaReader, opener RunOpener, now time.Time, it Item) (softFailMsg string, err error) {
	owner, err := conversationOwner(meta, it)
	if err != nil {
		return err.Error(), nil
	}
	runID, err := opener.Open(it.ConversationID, it.Instruction, owner)
	if err != nil {
		if errors.Is(err, storepkg.ErrConversationBusy) {
			return "", errConversationBusy
		}
		return errOpenRunFailed, nil
	}
	switch it.Kind {
	case "once", KindFollowup:
		if err := store.CompleteOpen(it.ID, runID, StatusCompleted, it.NextRunAt); err != nil {
			if setErr := store.SetRunningRunID(it.ID, runID); setErr != nil {
				return "", setErr
			}
			return "", err
		}
	case "daily", "weekly":
		next, nerr := Next(itemDraft(it), now)
		if nerr != nil {
			return errOpenRunFailed, nil
		}
		if err := store.CompleteOpen(it.ID, runID, StatusActive, next); err != nil {
			if setErr := store.SetRunningRunID(it.ID, runID); setErr != nil {
				return "", setErr
			}
			return "", err
		}
	default:
		return errOpenRunFailed, nil
	}
	return "", nil
}

func conversationOwner(meta MetaReader, it Item) (string, error) {
	if meta == nil {
		return "", errors.New(errConversationNotFound)
	}
	owner, err := meta.GetMeta(it.ConversationID)
	if err != nil {
		return "", errors.New(errConversationNotFound)
	}
	if owner != it.OwnerID {
		return "", errors.New(errOwnerMismatch)
	}
	return owner, nil
}

func itemDraft(it Item) Draft {
	return Draft{
		Kind:        it.Kind,
		Instruction: it.Instruction,
		Timezone:    it.Timezone,
		FireAt:      it.FireAt,
		TimeOfDay:   it.TimeOfDay,
		Weekday:     it.Weekday,
	}
}
