package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"
)

const messageRetentionSettingKey = "message_retention_days"
const messageRetentionConvPrefix = "message_retention_days:"
const defaultMessageRetentionInterval = 6 * time.Hour
const messageRetentionKeep = 2

var messageRetentionMu sync.Mutex

func (s *Server) messageRetentionDays() int {
	if s.Store == nil {
		return 0
	}
	raw, ok, err := s.Store.GetSetting(messageRetentionSettingKey)
	if err != nil || !ok {
		return 0
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
		return 0
	}
	return days
}

// messageRetentionDaysFor resolves a conversation override. -1 or a missing
// value inherits the global policy; 0 means forever.
func (s *Server) messageRetentionDaysFor(convID string, global int) int {
	if s.Store == nil || convID == "" {
		return global
	}
	raw, ok, err := s.Store.GetSetting(messageRetentionConvPrefix + convID)
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

func (s *Server) handleGetMessageRetention(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"days": s.messageRetentionDays()})
}

func (s *Server) handlePutMessageRetention(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Store.UpsertSetting(messageRetentionSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": body.Days})
}

// handleGetConversationMessageRetention returns the override and effective
// policy for one conversation (-1 = inherit global).
func (s *Server) handleGetConversationMessageRetention(w http.ResponseWriter, r *http.Request) {
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
	global := s.messageRetentionDays()
	override := -1
	if raw, ok, err := s.Store.GetSetting(messageRetentionConvPrefix + convID); err == nil && ok {
		var v int
		if json.Unmarshal(raw, &v) == nil && v >= -1 && v <= 3650 {
			override = v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           override,
		"effective_days": s.messageRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

func (s *Server) handlePutConversationMessageRetention(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Store.UpsertSetting(messageRetentionConvPrefix+convID, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.messageRetentionDays()
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           body.Days,
		"effective_days": s.messageRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

// cleanupExpiredMessages deletes old conversation messages, keeping the newest
// two per conversation. Conversations with an active run are skipped so a HITL
// or in-flight turn cannot lose its user message mid-run.
func (s *Server) cleanupExpiredMessages() (int, error) {
	if s.Messages == nil || s.Store == nil {
		return 0, nil
	}
	global := s.messageRetentionDays()
	now := time.Now().UTC()
	total := 0
	for _, convID := range s.Messages.ListConversationIDs() {
		days := s.messageRetentionDaysFor(convID, global)
		if days <= 0 {
			continue
		}
		active, err := s.Store.HasActiveRun(convID)
		if err != nil || active {
			continue
		}
		cutoff := now.AddDate(0, 0, -days)
		n, err := s.Messages.DeleteMessagesBefore(convID, cutoff, messageRetentionKeep)
		if err != nil {
			return total, err
		}
		total += n
	}
	if total > 0 {
		log.Printf("message retention: removed %d messages", total)
	}
	return total, nil
}

// StartMessageRetention runs an immediate cleanup and then repeats until ctx ends.
func (s *Server) StartMessageRetention(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultMessageRetentionInterval
	}
	run := func() {
		if !messageRetentionMu.TryLock() {
			return
		}
		defer messageRetentionMu.Unlock()
		if _, err := s.cleanupExpiredMessages(); err != nil {
			log.Printf("message retention: cleanup failed: %v", err)
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
