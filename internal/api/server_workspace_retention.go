package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

const messageRetentionWorkspacePrefix = "message_retention_days_workspace:"
const runRetentionWorkspacePrefix = "run_retention_days_workspace:"

type workspaceRetentionJSON struct {
	WorkspaceID          string `json:"workspace_id"`
	Name                 string `json:"name"`
	MessageDays          int    `json:"message_days"`
	RunDays              int    `json:"run_days"`
	EffectiveMessageDays int    `json:"effective_message_days"`
	EffectiveRunDays     int    `json:"effective_run_days"`
}

// workspaceRetentionSetting returns the explicit override (-1 when unset or
// invalid), so callers can distinguish "inherit" from "forever" (0).
func (s *Server) workspaceRetentionSetting(prefix, workspaceID string) int {
	if s.Store == nil || workspaceID == "" {
		return -1
	}
	raw, ok, err := s.Store.GetSetting(prefix + workspaceID)
	if err != nil || !ok {
		return -1
	}
	var days int
	if json.Unmarshal(raw, &days) != nil || days < -1 || days > 3650 {
		return -1
	}
	return days
}

// workspaceRetentionDays returns explicit non-negative overrides only.
func (s *Server) workspaceRetentionDays(prefix string) map[string]int {
	out := map[string]int{}
	if s.Store == nil {
		return out
	}
	settings, err := s.Store.ListSettingsByPrefix(prefix)
	if err != nil {
		return out
	}
	for key, raw := range settings {
		id := strings.TrimPrefix(key, prefix)
		if id == "" || id == key {
			continue
		}
		var days int
		if json.Unmarshal(raw, &days) != nil || days < 0 || days > 3650 {
			continue
		}
		out[id] = days
	}
	return out
}

// workspaceByConversation maps conversation ids to workspace ids once per
// sweep/preview instead of querying meta per conversation.
func (s *Server) workspaceByConversation() map[string]string {
	out := map[string]string{}
	ms := s.metaStore()
	if ms == nil {
		return out
	}
	metas, err := ms.ListMeta(conversation.MetaFilter{})
	if err != nil {
		return out
	}
	for _, m := range metas {
		if m.WorkspaceID != "" {
			out[m.ID] = m.WorkspaceID
		}
	}
	return out
}

func (s *Server) handleGetWorkspaceRetention(w http.ResponseWriter, r *http.Request) {
	list, err := s.workspaceList()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	globalMessage := s.messageRetentionDays()
	globalRun := s.runRetentionDays()
	items := make([]workspaceRetentionJSON, 0, len(list))
	for _, ws := range list {
		messageDays := s.workspaceRetentionSetting(messageRetentionWorkspacePrefix, ws.ID)
		runDays := s.workspaceRetentionSetting(runRetentionWorkspacePrefix, ws.ID)
		effectiveMessage := globalMessage
		if messageDays >= 0 {
			effectiveMessage = messageDays
		}
		effectiveRun := globalRun
		if runDays >= 0 {
			effectiveRun = runDays
		}
		items = append(items, workspaceRetentionJSON{
			WorkspaceID: ws.ID, Name: ws.Name,
			MessageDays: messageDays, RunDays: runDays,
			EffectiveMessageDays: effectiveMessage, EffectiveRunDays: effectiveRun,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"global_message_days": globalMessage,
		"global_run_days":     globalRun,
		"items":               items,
	})
}

func (s *Server) handlePutWorkspaceRetention(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string `json:"workspace_id"`
		MessageDays *int   `json:"message_days"`
		RunDays     *int   `json:"run_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.MessageDays == nil && body.RunDays == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "message_days or run_days is required")
		return
	}
	for _, days := range []*int{body.MessageDays, body.RunDays} {
		if days != nil && (*days < -1 || *days > 3650) {
			writeError(w, http.StatusBadRequest, "invalid_request", "days must be between -1 and 3650")
			return
		}
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
	if body.MessageDays != nil {
		raw, _ := json.Marshal(*body.MessageDays)
		if err := s.Store.UpsertSetting(messageRetentionWorkspacePrefix+wsID, raw); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	if body.RunDays != nil {
		raw, _ := json.Marshal(*body.RunDays)
		if err := s.Store.UpsertSetting(runRetentionWorkspacePrefix+wsID, raw); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	globalMessage := s.messageRetentionDays()
	globalRun := s.runRetentionDays()
	messageDays := s.workspaceRetentionSetting(messageRetentionWorkspacePrefix, wsID)
	runDays := s.workspaceRetentionSetting(runRetentionWorkspacePrefix, wsID)
	effectiveMessage := globalMessage
	if messageDays >= 0 {
		effectiveMessage = messageDays
	}
	effectiveRun := globalRun
	if runDays >= 0 {
		effectiveRun = runDays
	}
	writeJSON(w, http.StatusOK, workspaceRetentionJSON{
		WorkspaceID: wsID,
		MessageDays: messageDays, RunDays: runDays,
		EffectiveMessageDays: effectiveMessage, EffectiveRunDays: effectiveRun,
	})
}
