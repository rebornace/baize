package api

import (
	"encoding/json"
	"net/http"
	"time"
)

const retentionIntervalSettingKey = "retention_interval_hours"
const retentionIntervalDefault = 6 * time.Hour
const retentionIntervalMinHours = 1
const retentionIntervalMaxHours = 168

// retentionInterval reads the global cleanup cadence; invalid/missing values
// keep the caller's fallback so a bad setting cannot stop retention.
func (s *Server) retentionInterval(fallback time.Duration) time.Duration {
	if s.Store == nil {
		return fallback
	}
	raw, ok, err := s.Store.GetSetting(retentionIntervalSettingKey)
	if err != nil || !ok {
		return fallback
	}
	var hours int
	if json.Unmarshal(raw, &hours) != nil ||
		hours < retentionIntervalMinHours || hours > retentionIntervalMaxHours {
		return fallback
	}
	return time.Duration(hours) * time.Hour
}

func (s *Server) handleGetRetentionInterval(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"hours": int(s.retentionInterval(retentionIntervalDefault) / time.Hour),
		"min":   retentionIntervalMinHours,
		"max":   retentionIntervalMaxHours,
	})
}

func (s *Server) handlePutRetentionInterval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Hours int `json:"hours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Hours < retentionIntervalMinHours || body.Hours > retentionIntervalMaxHours {
		writeError(w, http.StatusBadRequest, "invalid_request", "hours must be between 1 and 168")
		return
	}
	raw, _ := json.Marshal(body.Hours)
	if err := s.Store.UpsertSetting(retentionIntervalSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": body.Hours})
}
