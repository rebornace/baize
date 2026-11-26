package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

const archiveRetentionSettingKey = "archive_retention_days"
const defaultArchiveRetentionInterval = 6 * time.Hour
const archiveRetentionBatch = 500

var archiveRetentionMu sync.Mutex

func (s *Server) archiveRetentionDays() int {
	if s.Store == nil {
		return 0
	}
	raw, ok, err := s.Store.GetSetting(archiveRetentionSettingKey)
	if err != nil || !ok {
		return 0
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
		return 0
	}
	return days
}

func (s *Server) handleGetArchiveRetention(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"days": s.archiveRetentionDays()})
}

func (s *Server) handlePutArchiveRetention(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Store.UpsertSetting(archiveRetentionSettingKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": body.Days})
}

// cleanupExpiredArchived permanently deletes conversations archived longer than
// the configured window. Active runs are skipped.
func (s *Server) cleanupExpiredArchived() (int, error) {
	if s.Store == nil {
		return 0, nil
	}
	days := s.archiveRetentionDays()
	if days <= 0 {
		return 0, nil
	}
	ms := s.metaStore()
	if ms == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	metas, err := ms.ListMeta(conversation.MetaFilter{})
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, m := range metas {
		if m.ArchivedAt.IsZero() || !m.ArchivedAt.Before(cutoff) {
			continue
		}
		if err := s.deleteConversationData(m.ID); err != nil {
			if errors.Is(err, store.ErrConversationBusy) {
				continue
			}
			return deleted, err
		}
		deleted++
		if deleted >= archiveRetentionBatch {
			break
		}
	}
	if deleted > 0 {
		log.Printf("archive retention: removed %d archived conversations", deleted)
	}
	return deleted, nil
}

// StartArchiveRetention runs an immediate pass and then repeats until ctx ends.
func (s *Server) StartArchiveRetention(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultArchiveRetentionInterval
	}
	run := func() {
		if !archiveRetentionMu.TryLock() {
			return
		}
		defer archiveRetentionMu.Unlock()
		if _, err := s.cleanupExpiredArchived(); err != nil {
			log.Printf("archive retention: cleanup failed: %v", err)
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
