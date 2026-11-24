package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

const attachmentRetentionKey = "attachment_retention_days"
const attachmentRetentionBatch = 200
const defaultAttachmentRetentionInterval = 6 * time.Hour

var attachmentRetentionMu sync.Mutex

func (s *Server) handleGetAttachmentRetention(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"days": s.attachmentRetentionDays()})
}

func (s *Server) handlePutAttachmentRetention(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Store.UpsertSetting(attachmentRetentionKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": body.Days})
}

func (s *Server) attachmentRetentionDays() int {
	if s.Store == nil {
		return 0
	}
	raw, ok, err := s.Store.GetSetting(attachmentRetentionKey)
	if err != nil || !ok {
		return 0
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
		return 0
	}
	return days
}

// cleanupExpiredAttachments deletes indexed attachments older than the
// configured retention window and returns how many were removed.
func (s *Server) cleanupExpiredAttachments(ctx context.Context) (int, error) {
	days := s.attachmentRetentionDays()
	if days <= 0 || s.ChannelMedia == nil || s.Store == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	total := 0
	for {
		list, err := s.Store.ListConversationAttachmentsBefore(cutoff, attachmentRetentionBatch)
		if err != nil {
			return total, err
		}
		if len(list) == 0 {
			return total, nil
		}
		progress := 0
		for _, a := range list {
			if err := s.deleteConversationAttachment(ctx, a.ConversationID, a.Object, a.Filename); err != nil {
				log.Printf("attachment retention: delete %s/%s: %v", a.ConversationID, a.Object, err)
				continue
			}
			progress++
			total++
		}
		if progress == 0 {
			return total, nil
		}
		if len(list) < attachmentRetentionBatch {
			return total, nil
		}
	}
}

// StartAttachmentRetention runs an immediate cleanup and then repeats on the
// given interval until ctx is cancelled. It is safe with multiple replicas:
// deletes are idempotent.
func (s *Server) StartAttachmentRetention(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = defaultAttachmentRetentionInterval
	}
	run := func() {
		if !attachmentRetentionMu.TryLock() {
			return
		}
		defer attachmentRetentionMu.Unlock()
		n, err := s.cleanupExpiredAttachments(ctx)
		if err != nil {
			log.Printf("attachment retention: cleanup failed after %d: %v", n, err)
		} else if n > 0 {
			log.Printf("attachment retention: removed %d expired attachments", n)
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
