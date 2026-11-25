package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// handleGetOpsStatus returns a compact, read-only operations snapshot for the
// settings page: active runs, recent failures, webhook/alert state, eval gate
// and retention policies.
func (s *Server) handleGetOpsStatus(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	dayAgo := now.Add(-24 * time.Hour)
	all, err := s.Store.CountRunsByStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	recent, err := s.Store.CountRunsByStatusBetween(dayAgo, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	state := budgetAlertState{}
	if raw, ok, serr := s.Store.GetSetting(costAlertStateKey); serr == nil && ok {
		_ = json.Unmarshal(raw, &state)
	}
	scoped := 0
	for _, sc := range state.Scopes {
		if sc.DayLevel != "" || sc.MonthLevel != "" {
			scoped++
		}
	}
	gate := s.evalGateLastResult()
	gateOut := map[string]any{"last_pass": nil, "last_ran_at": "", "total": 0, "failed": 0}
	if gate != nil {
		gateOut["last_pass"] = gate.Pass
		gateOut["last_ran_at"] = gate.RanAt
		gateOut["total"] = gate.Total
		gateOut["failed"] = gate.Failed
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"runs": map[string]any{
			"queued":        all[store.StatusQueued],
			"running":       all[store.StatusRunning],
			"waiting_human": all[store.StatusWaitingHuman],
			"active":        all[store.StatusQueued] + all[store.StatusRunning] + all[store.StatusWaitingHuman],
			"succeeded_24h": recent[store.StatusSucceeded],
			"failed_24h":    recent[store.StatusFailed],
		},
		"webhook_configured": s.loadEventsWebhook().URL != "",
		"budget": map[string]any{
			"day_alert":     state.DayLevel,
			"month_alert":   state.MonthLevel,
			"scoped_alerts": scoped,
		},
		"eval_gate": gateOut,
		"retention": map[string]any{
			"attachment_days": s.attachmentRetentionDays(),
			"run_days":        s.runRetentionDays(),
			"message_days":    s.messageRetentionDays(),
		},
	})
}
