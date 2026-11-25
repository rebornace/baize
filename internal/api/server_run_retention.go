package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

const runRetentionSettingKey = "run_retention_days"
const defaultRunRetentionInterval = 6 * time.Hour
const runRetentionBatchSize = 500
const runRetentionMaxPerPass = 5000

var runRetentionMu sync.Mutex

func (s *Server) runRetentionDays() int {
	if s.Store == nil {
		return 0
	}
	raw, ok, err := s.Store.GetSetting(runRetentionSettingKey)
	if err != nil || !ok {
		return 0
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
		return 0
	}
	return days
}

func (s *Server) handleGetRunRetention(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"days": s.runRetentionDays()})
}

func (s *Server) handlePutRunRetention(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Days < 0 || body.Days > 3650 {
		writeError(w, http.StatusBadRequest, "invalid_request", "days must be between 0 and 3650")
		return
	}
	raw, _ := json.Marshal(body.Days)
	if err := s.Store.UpsertSetting(runRetentionSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": body.Days})
}

// cleanupExpiredRuns deletes terminal runs older than the configured window.
// It never touches queued/running/waiting_human runs.
func (s *Server) cleanupExpiredRuns() (int, error) {
	days := s.runRetentionDays()
	if days <= 0 || s.Store == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	total := 0
	for {
		n, err := s.Store.DeleteTerminalRunsBefore(cutoff, runRetentionBatchSize)
		if err != nil {
			return total, err
		}
		total += n
		if n < runRetentionBatchSize || total >= runRetentionMaxPerPass {
			break
		}
	}
	if total > 0 {
		log.Printf("run retention: removed %d terminal runs older than %s", total, cutoff.Format("2006-01-02"))
	}
	return total, nil
}

// StartRunRetention runs an immediate cleanup and then repeats until ctx ends.
func (s *Server) StartRunRetention(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultRunRetentionInterval
	}
	run := func() {
		if !runRetentionMu.TryLock() {
			return
		}
		defer runRetentionMu.Unlock()
		if _, err := s.cleanupExpiredRuns(); err != nil {
			log.Printf("run retention: cleanup failed: %v", err)
		}
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
