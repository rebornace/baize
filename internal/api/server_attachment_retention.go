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
)

const attachmentRetentionKey = "attachment_retention_days"
const attachmentRetentionConvPrefix = "attachment_retention_days:"
const attachmentRetentionWorkspacePrefix = "attachment_retention_days_ws:"
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

// attachmentRetentionDaysFor resolves a conversation override, then its
// workspace override, then the global policy. -1 or a missing value inherits
// the next level; 0 means forever.
func (s *Server) attachmentRetentionDaysFor(convID string, global int) int {
	if s.Store == nil {
		return global
	}
	if convID != "" {
		if days, ok := s.settingRetentionDays(attachmentRetentionConvPrefix + convID); ok {
			return days
		}
	}
	if wsID := s.workspaceIDForConv(convID); wsID != "" {
		if days, ok := s.settingRetentionDays(attachmentRetentionWorkspacePrefix + wsID); ok {
			return days
		}
	}
	return global
}

// attachmentRetentionWorkspaceOverride returns the explicit workspace value, or
// -1 when it inherits the global policy.
func (s *Server) attachmentRetentionWorkspaceOverride(wsID string) int {
	if days, ok := s.settingRetentionDays(attachmentRetentionWorkspacePrefix + wsID); ok {
		return days
	}
	return -1
}

// settingRetentionDays reads a non-negative day override. A missing, malformed
// or -1 value reports ok=false so the caller can inherit.
func (s *Server) settingRetentionDays(key string) (int, bool) {
	if s.Store == nil {
		return 0, false
	}
	raw, ok, err := s.Store.GetSetting(key)
	if err != nil || !ok {
		return 0, false
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
		return 0, false
	}
	return days, true
}

// handleGetConversationAttachmentRetention returns the override and effective
// policy for one conversation (-1 = inherit global).
func (s *Server) handleGetConversationAttachmentRetention(w http.ResponseWriter, r *http.Request) {
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
	global := s.attachmentRetentionDays()
	override := -1
	if raw, ok, err := s.Store.GetSetting(attachmentRetentionConvPrefix + convID); err == nil && ok {
		var v int
		if json.Unmarshal(raw, &v) == nil && v >= -1 && v <= 3650 {
			override = v
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           override,
		"effective_days": s.attachmentRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

func (s *Server) handlePutConversationAttachmentRetention(w http.ResponseWriter, r *http.Request) {
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
	if err := s.Store.UpsertSetting(attachmentRetentionConvPrefix+convID, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.attachmentRetentionDays()
	writeJSON(w, http.StatusOK, map[string]any{
		"days":           body.Days,
		"effective_days": s.attachmentRetentionDaysFor(convID, global),
		"global_days":    global,
	})
}

type workspaceAttachmentRetentionJSON struct {
	WorkspaceID   string `json:"workspace_id"`
	Name          string `json:"name"`
	Days          int    `json:"days"`
	EffectiveDays int    `json:"effective_days"`
}

func (s *Server) handleGetWorkspaceAttachmentRetention(w http.ResponseWriter, r *http.Request) {
	list, err := s.workspaceList()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.attachmentRetentionDays()
	items := make([]workspaceAttachmentRetentionJSON, 0, len(list))
	for _, ws := range list {
		override := s.attachmentRetentionWorkspaceOverride(ws.ID)
		effective := global
		if override >= 0 {
			effective = override
		}
		items = append(items, workspaceAttachmentRetentionJSON{
			WorkspaceID: ws.ID, Name: ws.Name,
			Days: override, EffectiveDays: effective,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"global_days": global,
		"items":       items,
	})
}

func (s *Server) handlePutWorkspaceAttachmentRetention(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		Days        int    `json:"days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Days < -1 || body.Days > 3650 {
		writeError(w, http.StatusBadRequest, "invalid_request", "days must be between -1 and 3650")
		return
	}
	wsID := conversation.NormalizeWorkspaceID(body.WorkspaceID)
	if wst := s.workspaceStore(); wst != nil {
		if _, err := wst.GetWorkspace(wsID); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "workspace not found")
			return
		}
	} else if wsID != conversation.DefaultWorkspaceID {
		writeError(w, http.StatusNotFound, "not_found", "workspace not found")
		return
	}
	raw, _ := json.Marshal(body.Days)
	if err := s.Store.UpsertSetting(attachmentRetentionWorkspacePrefix+wsID, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	global := s.attachmentRetentionDays()
	effective := global
	if body.Days >= 0 {
		effective = body.Days
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id":   wsID,
		"days":           body.Days,
		"effective_days": effective,
		"global_days":    global,
	})
}

// cleanupExpiredAttachments deletes indexed attachments older than the
// configured retention window and returns how many were removed.
func (s *Server) cleanupExpiredAttachments(ctx context.Context) (int, error) {
	if s.ChannelMedia == nil || s.Store == nil {
		return 0, nil
	}
	convs, err := s.Store.ListAttachmentConversationIDs()
	if err != nil {
		return 0, err
	}
	global := s.attachmentRetentionDays()
	now := time.Now().UTC()
	total := 0
	for _, conv := range convs {
		// A queued/running/waiting_human run may still need its uploaded files
		// to resume (HITL or reconciliation). Keep the conversation intact until
		// the run settles; cleanup resumes on a later pass.
		active, aerr := s.Store.HasActiveRun(conv)
		if aerr != nil {
			log.Printf("attachment retention: active-run check %s: %v", conv, aerr)
			continue
		}
		if active {
			continue
		}
		days := s.attachmentRetentionDaysFor(conv, global)
		if days <= 0 {
			continue
		}
		cutoff := now.AddDate(0, 0, -days)
		for {
			list, lerr := s.Store.ListConversationAttachmentsBeforeConv(conv, cutoff, attachmentRetentionBatch)
			if lerr != nil {
				return total, lerr
			}
			if len(list) == 0 {
				break
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
			if progress == 0 || len(list) < attachmentRetentionBatch {
				break
			}
		}
	}
	return total, nil
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
