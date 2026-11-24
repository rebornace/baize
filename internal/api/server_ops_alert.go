package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhook"
)

const opsAlertStateKey = "ops_alert_state"
const defaultOpsAlertInterval = 5 * time.Minute
const opsFailureWindow = 15 * time.Minute
const opsFailureThreshold = 3
const opsFailureCooldown = 30 * time.Minute
const opsWaitingCooldown = 60 * time.Minute

type opsAlertState struct {
	FailureAt string `json:"failure_at"`
	WaitingAt string `json:"waiting_at"`
}

var opsAlertMu sync.Mutex

// checkOpsAlerts sends ops.alert when recent failures or waiting approvals need
// attention. Cooldowns are persisted so repeated checks stay quiet.
func (s *Server) checkOpsAlerts(ctx context.Context) (int, error) {
	if s.Store == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	state := opsAlertState{}
	if raw, ok, err := s.Store.GetSetting(opsAlertStateKey); err == nil && ok {
		_ = json.Unmarshal(raw, &state)
	}
	sent := 0
	send := func(scope string, data map[string]any) {
		if s.Webhook == nil {
			return
		}
		data["scope"] = scope
		if err := s.Webhook.SendAlert(ctx, "ops.alert", data); err != nil {
			if !errors.Is(err, webhook.ErrNoURL) {
				log.Printf("ops alert webhook (%s): %v", scope, err)
			}
			return
		}
		sent++
	}

	failures, ferr := s.Store.ListRunsCreatedBetween(now.Add(-opsFailureWindow), now, store.StatusFailed, 200)
	if ferr != nil {
		return sent, ferr
	}
	if len(failures) >= opsFailureThreshold && cooldownElapsed(state.FailureAt, now, opsFailureCooldown) {
		send("run_failures", map[string]any{
			"count": len(failures), "window_minutes": int(opsFailureWindow.Minutes()),
		})
		state.FailureAt = now.Format(time.RFC3339)
	}

	waiting, werr := s.Store.ListWaitingHumanRuns(200)
	if werr != nil {
		return sent, werr
	}
	if len(waiting) > 0 && cooldownElapsed(state.WaitingAt, now, opsWaitingCooldown) {
		send("waiting_human", map[string]any{"count": len(waiting)})
		state.WaitingAt = now.Format(time.RFC3339)
	}

	raw, _ := json.Marshal(state)
	_ = s.Store.UpsertSetting(opsAlertStateKey, raw)
	return sent, nil
}

func cooldownElapsed(last string, now time.Time, cooldown time.Duration) bool {
	if last == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return true
	}
	return now.Sub(t) >= cooldown
}

// StartOpsAlerts checks immediately and then every interval until ctx ends.
func (s *Server) StartOpsAlerts(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultOpsAlertInterval
	}
	run := func() {
		if !opsAlertMu.TryLock() {
			return
		}
		defer opsAlertMu.Unlock()
		_, _ = s.checkOpsAlerts(ctx)
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
