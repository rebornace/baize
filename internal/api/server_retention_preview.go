package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

const retentionPreviewItemLimit = 20

type retentionPreviewConversation struct {
	ConversationID string `json:"conversation_id"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	Days           int    `json:"days"`
	Count          int    `json:"count"`
}

type retentionPreviewAgent struct {
	AgentID string `json:"agent_id"`
	Days    int    `json:"days"`
	Count   int    `json:"count"`
}

type retentionPreviewArchived struct {
	ConversationID string `json:"conversation_id"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	ArchivedAt     string `json:"archived_at"`
}

type retentionPreview struct {
	Messages struct {
		Total         int                            `json:"total"`
		Conversations []retentionPreviewConversation `json:"conversations"`
	} `json:"messages"`
	Runs struct {
		Total         int                            `json:"total"`
		Conversations []retentionPreviewConversation `json:"conversations"`
		Agents        []retentionPreviewAgent        `json:"agents"`
		Blocked       bool                           `json:"blocked,omitempty"`
	} `json:"runs"`
	Archived struct {
		Total         int                        `json:"total"`
		Days          int                        `json:"days"`
		Conversations []retentionPreviewArchived `json:"conversations"`
	} `json:"archived"`
}

// messageRetentionDaysResolved resolves conversation > workspace > global with
// preloaded maps, avoiding a meta lookup per conversation.
func (s *Server) messageRetentionDaysResolved(
	convID string, global int, workspaceDays map[string]int, workspaceByConv map[string]string,
) int {
	if s.Store != nil {
		if raw, ok, err := s.Store.GetSetting(messageRetentionConvPrefix + convID); err == nil && ok {
			var days int
			if json.Unmarshal(raw, &days) == nil && days >= 0 && days <= 3650 {
				return days
			}
		}
	}
	if wsID, ok := workspaceByConv[convID]; ok {
		if days, ok := workspaceDays[wsID]; ok {
			return days
		}
	}
	return global
}

func sortRetentionConversations(items []retentionPreviewConversation) []retentionPreviewConversation {
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > retentionPreviewItemLimit {
		items = items[:retentionPreviewItemLimit]
	}
	return items
}

// handleRetentionPreview reports what the next retention pass would remove
// without deleting anything.
func (s *Server) handleRetentionPreview(w http.ResponseWriter, r *http.Request) {
	if s.Store == nil || s.Messages == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "retention stores are not configured")
		return
	}
	if !runRetentionMu.TryLock() {
		writeError(w, http.StatusConflict, "retention_busy", "a retention pass is running")
		return
	}
	defer runRetentionMu.Unlock()
	if !messageRetentionMu.TryLock() {
		writeError(w, http.StatusConflict, "retention_busy", "a retention pass is running")
		return
	}
	defer messageRetentionMu.Unlock()
	if !archiveRetentionMu.TryLock() {
		writeError(w, http.StatusConflict, "retention_busy", "a retention pass is running")
		return
	}
	defer archiveRetentionMu.Unlock()

	now := time.Now().UTC()
	var out retentionPreview
	workspaceByConv := s.workspaceByConversation()

	// Messages: conversation > workspace > global.
	messageGlobal := s.messageRetentionDays()
	messageWorkspace := s.workspaceRetentionDays(messageRetentionWorkspacePrefix)
	out.Messages.Conversations = []retentionPreviewConversation{}
	for _, convID := range s.Messages.ListConversationIDs() {
		days := s.messageRetentionDaysResolved(convID, messageGlobal, messageWorkspace, workspaceByConv)
		if days <= 0 {
			continue
		}
		if active, err := s.Store.HasActiveRun(convID); err != nil || active {
			continue
		}
		cutoff := now.AddDate(0, 0, -days)
		n, err := s.Messages.CountMessagesBefore(convID, cutoff, messageRetentionKeep)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		out.Messages.Total += n
		if n > 0 {
			out.Messages.Conversations = append(out.Messages.Conversations, retentionPreviewConversation{
				ConversationID: convID, WorkspaceID: workspaceByConv[convID], Days: days, Count: n,
			})
		}
	}
	out.Messages.Conversations = sortRetentionConversations(out.Messages.Conversations)

	// Runs: conversation > workspace > agent > global.
	runGlobal := s.runRetentionDays()
	convDays := map[string]int{}
	if settings, err := s.Store.ListSettingsByPrefix(runRetentionConvPrefix); err == nil {
		convDays = parseRetentionOverrides(settings, runRetentionConvPrefix)
	}
	agentDays := map[string]int{}
	if settings, err := s.Store.ListSettingsByPrefix(runRetentionAgentPrefix); err == nil {
		agentDays = parseRetentionOverrides(settings, runRetentionAgentPrefix)
	}
	workspaceDays := s.workspaceRetentionDays(runRetentionWorkspacePrefix)
	workspaceConvs := map[string]bool{}
	convsByWorkspace := map[string][]string{}
	for convID, wsID := range workspaceByConv {
		if _, ok := workspaceDays[wsID]; ok {
			workspaceConvs[convID] = true
			convsByWorkspace[wsID] = append(convsByWorkspace[wsID], convID)
		}
	}
	if len(convDays)+len(agentDays)+len(workspaceConvs) > runRetentionMaxOverrides && len(agentDays) > 0 {
		out.Runs.Blocked = true
		writeJSON(w, http.StatusOK, out)
		return
	}

	out.Runs.Conversations = []retentionPreviewConversation{}
	out.Runs.Agents = []retentionPreviewAgent{}
	scopedSet := map[string]bool{}
	scopedConvs := make([]string, 0, len(convDays)+len(workspaceConvs))
	countForConversation := func(convID string, days int) {
		if days <= 0 {
			return
		}
		cutoff := now.AddDate(0, 0, -days)
		n, err := s.Store.CountTerminalRunsBeforeForConversation(convID, cutoff)
		if err != nil {
			return
		}
		out.Runs.Total += n
		if n > 0 {
			out.Runs.Conversations = append(out.Runs.Conversations, retentionPreviewConversation{
				ConversationID: convID, WorkspaceID: workspaceByConv[convID], Days: days, Count: n,
			})
		}
	}
	for convID, days := range convDays {
		scopedSet[convID] = true
		scopedConvs = append(scopedConvs, convID)
		countForConversation(convID, days)
	}
	for wsID, days := range workspaceDays {
		for _, convID := range convsByWorkspace[wsID] {
			if _, has := convDays[convID]; has {
				continue
			}
			countForConversation(convID, days)
		}
	}
	for convID := range workspaceConvs {
		if !scopedSet[convID] {
			scopedSet[convID] = true
			scopedConvs = append(scopedConvs, convID)
		}
	}
	scopedAgents := make([]string, 0, len(agentDays))
	for agentID, days := range agentDays {
		scopedAgents = append(scopedAgents, agentID)
		if days <= 0 {
			continue
		}
		cutoff := now.AddDate(0, 0, -days)
		n, err := s.Store.CountTerminalRunsBeforeForAgent(agentID, cutoff, scopedConvs)
		if err != nil {
			continue
		}
		out.Runs.Total += n
		if n > 0 {
			out.Runs.Agents = append(out.Runs.Agents, retentionPreviewAgent{
				AgentID: agentID, Days: days, Count: n,
			})
		}
	}
	if runGlobal > 0 {
		cutoff := now.AddDate(0, 0, -runGlobal)
		n, err := s.Store.CountTerminalRunsBeforeExcluding(cutoff, scopedConvs, scopedAgents)
		if err == nil {
			out.Runs.Total += n
		}
	}
	out.Runs.Conversations = sortRetentionConversations(out.Runs.Conversations)
	sort.Slice(out.Runs.Agents, func(i, j int) bool { return out.Runs.Agents[i].Count > out.Runs.Agents[j].Count })
	if len(out.Runs.Agents) > retentionPreviewItemLimit {
		out.Runs.Agents = out.Runs.Agents[:retentionPreviewItemLimit]
	}

	// Archived conversations: permanently delete after the configured window.
	out.Archived.Conversations = []retentionPreviewArchived{}
	if days := s.archiveRetentionDays(); days > 0 {
		out.Archived.Days = days
		cutoff := now.AddDate(0, 0, -days)
		if meta := s.metaStore(); meta != nil {
			if metas, err := meta.ListMeta(conversation.MetaFilter{}); err == nil {
				for _, m := range metas {
					if m.ArchivedAt.IsZero() || !m.ArchivedAt.Before(cutoff) {
						continue
					}
					if active, err := s.Store.HasActiveRun(m.ID); err != nil || active {
						continue
					}
					out.Archived.Total++
					if len(out.Archived.Conversations) < retentionPreviewItemLimit {
						out.Archived.Conversations = append(out.Archived.Conversations, retentionPreviewArchived{
							ConversationID: m.ID,
							WorkspaceID:    workspaceByConv[m.ID],
							ArchivedAt:     m.ArchivedAt.UTC().Format(time.RFC3339),
						})
					}
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRunRetentionNow performs one cleanup pass for messages and runs.
func (s *Server) handleRunRetentionNow(w http.ResponseWriter, r *http.Request) {
	if !runRetentionMu.TryLock() {
		writeError(w, http.StatusConflict, "retention_busy", "a retention pass is running")
		return
	}
	defer runRetentionMu.Unlock()
	if !messageRetentionMu.TryLock() {
		writeError(w, http.StatusConflict, "retention_busy", "a retention pass is running")
		return
	}
	defer messageRetentionMu.Unlock()
	if !archiveRetentionMu.TryLock() {
		writeError(w, http.StatusConflict, "retention_busy", "a retention pass is running")
		return
	}
	defer archiveRetentionMu.Unlock()

	messages, err := s.cleanupExpiredMessages()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	runs, err := s.cleanupExpiredRuns()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	archived, err := s.cleanupExpiredArchived()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"messages_deleted": messages,
		"runs_deleted":     runs,
		"archived_deleted": archived,
	})
}
