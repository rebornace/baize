package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const runRetentionSettingKey = "run_retention_days"
const runRetentionConvPrefix = "run_retention_days:"
const defaultRunRetentionInterval = 6 * time.Hour
const runRetentionBatchSize = 500
const runRetentionMaxPerPass = 5000

// runRetentionMaxOverrides matches the store-side exclusion cap. Above it the
// cleanup switches to per-conversation sweeps to stay correct.
const runRetentionMaxOverrides = 5000

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

// runRetentionDaysFor resolves a conversation override. -1 or a missing value
// inherits the global policy; 0 means forever.
func (s *Server) runRetentionDaysFor(convID string, global int) int {
	if s.Store == nil || convID == "" {
		return global
	}
	raw, ok, err := s.Store.GetSetting(runRetentionConvPrefix + convID)
	if err != nil || !ok {
		return global
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < -1 || days > 3650 {
		return global
	}
	if days < 0 {
		return global
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

// handleGetConversationRunRetention returns the override and effective policy
// for one conversation (-1 = inherit global).
func (s *Server) handleGetConversationRunRetention(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.runRetentionDays()
	override := -1
	if raw, ok, err := s.Store.GetSetting(runRetentionConvPrefix + convID); err == nil && ok {
		var v int
		if json.Unmarshal(raw, &v) == nil && v >= -1 && v <= 3650 {
			override = v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           override,
		"effective_days": s.runRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

func (s *Server) handlePutConversationRunRetention(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var body struct {
		Days int `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Days < -1 || body.Days > 3650 {
		writeError(w, http.StatusBadRequest, "invalid_request", "days must be between -1 and 3650")
		return
	}
	raw, _ := json.Marshal(body.Days)
	if err := s.Store.UpsertSetting(runRetentionConvPrefix+convID, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.runRetentionDays()
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           body.Days,
		"effective_days": s.runRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

// cleanupExpiredRuns deletes terminal runs older than the configured window.
// It never touches queued/running/waiting_human runs. Conversations with an
// explicit override are excluded from the global sweep and handled with their
// own cutoff; 0 means forever.
func (s *Server) cleanupExpiredRuns() (int, error) {
	if s.Store == nil {
		return 0, nil
	}
	global := s.runRetentionDays()
	now := time.Now().UTC()

	overrides, err := s.Store.ListSettingsByPrefix(runRetentionConvPrefix)
	if err != nil {
		return 0, err
	}
	overrideDays := parseRetentionOverrides(overrides, runRetentionConvPrefix)

	total := 0
	// Very large override sets fall back to one sweep per conversation so the
	// global query never has to carry an unbounded exclusion list.
	if len(overrideDays) > runRetentionMaxOverrides {
		convs, cerr := s.Store.ListRunConversationIDs()
		if cerr != nil {
			return total, cerr
		}
		for _, convID := range convs {
			days, ok := overrideDays[convID]
			if !ok {
				days = global
			}
			n, derr := s.deleteRunsForConversation(convID, days, now)
			if derr != nil {
				return total, derr
			}
			total += n
			if total >= runRetentionMaxPerPass {
				break
			}
		}
		logRunRetention(total)
		return total, nil
	}

	scoped := make([]string, 0, len(overrideDays))
	for convID, days := range overrideDays {
		scoped = append(scoped, convID)
		if days <= 0 {
			continue
		}
		n, derr := s.deleteRunsForConversation(convID, days, now)
		if derr != nil {
			return total, derr
		}
		total += n
		if total >= runRetentionMaxPerPass {
			logRunRetention(total)
			return total, nil
		}
	}

	if global > 0 && total < runRetentionMaxPerPass {
		cutoff := now.AddDate(0, 0, -global)
		for {
			n, derr := s.Store.DeleteTerminalRunsBeforeExcluding(cutoff, runRetentionBatchSize, scoped)
			if derr != nil {
				return total, derr
			}
			total += n
			if n < runRetentionBatchSize || total >= runRetentionMaxPerPass {
				break
			}
		}
	}
	logRunRetention(total)
	return total, nil
}

// deleteRunsForConversation removes terminal runs for one conversation at the
// given retention window. days <= 0 keeps everything.
func (s *Server) deleteRunsForConversation(convID string, days int, now time.Time) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := now.AddDate(0, 0, -days)
	total := 0
	for {
		n, err := s.Store.DeleteTerminalRunsBeforeForConversation(convID, cutoff, runRetentionBatchSize)
		if err != nil {
			return total, err
		}
		total += n
		if n < runRetentionBatchSize {
			return total, nil
		}
	}
}

func logRunRetention(total int) {
	if total > 0 {
		log.Printf("run retention: removed %d terminal runs", total)
	}
}

// parseRetentionOverrides keeps only explicit per-conversation values (>= 0);
// -1 and malformed settings inherit the global policy.
func parseRetentionOverrides(settings map[string][]byte, prefix string) map[string]int {
	out := make(map[string]int, len(settings))
	for key, raw := range settings {
		convID := strings.TrimPrefix(key, prefix)
		if convID == "" || convID == key {
			continue
		}
		var days int
		if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
			continue
		}
		out[convID] = days
	}
	return out
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
