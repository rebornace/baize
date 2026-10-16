package schedule

import (
	"errors"
	"time"
)

const (
	errConversationNotFound = "schedule: conversation not found"
	errOwnerMismatch        = "schedule: conversation owner mismatch"
	errOpenRunFailed        = "schedule: failed to open run"
)

// RunOpener starts a scheduled run for a conversation.
type RunOpener interface {
	Open(conversationID, instruction, ownerID string) (runID string, err error)
}

// RunStatus reports whether a run has reached a terminal state.
type RunStatus func(runID string) (terminal bool, err error)

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
	if err := store.ClearStalePending(); err != nil {
		return err
	}
	if err := reconcileRunningRuns(store, runStatus); err != nil {
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
		if err != nil {
			return err
		}
		if softMsg != "" {
			pendingFail[it.ID] = softMsg
		}
	}
	return nil
}

func reconcileRunningRuns(store Store, runStatus RunStatus) error {
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
		terminal, err := runStatus(it.RunningRunID)
		if err != nil || !terminal {
			continue
		}
		if err := store.ReleaseRunningRun(it.ID); err != nil {
			return err
		}
	}
	return nil
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
		return errOpenRunFailed, nil
	}
	switch it.Kind {
	case "once":
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
