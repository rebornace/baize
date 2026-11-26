package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

const costAlertSilenceKey = "cost_alert_silence"
const costAlertSilenceMax = 31 * 24 * time.Hour

type costAlertSilence struct {
	Until  time.Time `json:"until"`
	Reason string    `json:"reason,omitempty"`
}

// budgetAlertSilence returns the active silence window, if any. Expired
// windows are treated as absent so operators do not need to clear them.
func (s *Server) budgetAlertSilence(now time.Time) (time.Time, string) {
	if s.Store == nil {
		return time.Time{}, ""
	}
	raw, ok, err := s.Store.GetSetting(costAlertSilenceKey)
	if err != nil || !ok {
		return time.Time{}, ""
	}
	var v costAlertSilence
	if json.Unmarshal(raw, &v) != nil {
		return time.Time{}, ""
	}
	if v.Until.IsZero() || !now.Before(v.Until) {
		return time.Time{}, ""
	}
	return v.Until.UTC(), strings.TrimSpace(v.Reason)
}

func (s *Server) budgetAlertsSilenced(now time.Time) bool {
	until, _ := s.budgetAlertSilence(now)
	return !until.IsZero()
}

func (s *Server) handleGetBudgetAlertSilence(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	until, reason := s.budgetAlertSilence(now)
	writeJSON(w, http.StatusOK, map[string]any{
		"until":  nullableTime(until),
		"reason": reason,
		"active": !until.IsZero(),
	})
}

func (s *Server) handlePutBudgetAlertSilence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Until  string `json:"until"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	rawUntil := strings.TrimSpace(body.Until)
	if rawUntil == "" {
		if err := s.Store.UpsertSetting(costAlertSilenceKey, []byte(`{}`)); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"until": nil, "reason": "", "active": false})
		return
	}
	until, err := parseFlexibleTime(rawUntil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "until must be an RFC3339 timestamp")
		return
	}
	now := time.Now().UTC()
	if !until.After(now) {
		writeError(w, http.StatusBadRequest, "invalid_request", "until must be in the future")
		return
	}
	if until.After(now.Add(costAlertSilenceMax)) {
		writeError(w, http.StatusBadRequest, "invalid_request", "until must be within 31 days")
		return
	}
	reason := strings.TrimSpace(body.Reason)
	if len([]rune(reason)) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_request", "reason is too long")
		return
	}
	raw, _ := json.Marshal(costAlertSilence{Until: until.UTC(), Reason: reason})
	if err := s.Store.UpsertSetting(costAlertSilenceKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"until":  until.UTC().Format(time.RFC3339Nano),
		"reason": reason,
		"active": true,
	})
}

func parseFlexibleTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func nullableTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}
