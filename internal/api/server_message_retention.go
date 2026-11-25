package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

const messageRetentionSettingKey = "message_retention_days"
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

// cleanupExpiredMessages deletes old conversation messages, keeping the newest
// two per conversation. Conversations with an active run are skipped so a HITL
// or in-flight turn cannot lose its user message mid-run.
func (s *Server) cleanupExpiredMessages() (int, error) {
	days := s.messageRetentionDays()
	if days <= 0 || s.Messages == nil || s.Store == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	total := 0
	for _, convID := range s.Messages.ListConversationIDs() {
		active, err := s.Store.HasActiveRun(convID)
		if err != nil || active {
			continue
		}
		n, err := s.Messages.DeleteMessagesBefore(convID, cutoff, messageRetentionKeep)
		if err != nil {
			return total, err
		}
		total += n
	}
	if total > 0 {
		log.Printf("message retention: removed %d messages older than %s", total, cutoff.Format("2006-01-02"))
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
