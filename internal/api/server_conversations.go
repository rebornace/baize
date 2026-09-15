package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
)

func (s *Server) handleListIdentities(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if s.Identities == nil {
		writeJSON(w, http.StatusOK, []identity.PublicView{})
		return
	}
	views := s.Identities.ListPublic(convID)
	if views == nil {
		views = []identity.PublicView{}
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handlePostIdentity(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "identity store not configured")
		return
	}
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	var body struct {
		Token         string `json:"token"`
		Authorization string `json:"authorization"`
		Label         string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	raw := strings.TrimSpace(body.Token)
	if raw == "" {
		raw = strings.TrimSpace(body.Authorization)
	}
	if raw == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "token or authorization is required")
		return
	}
	id, err := identity.UpsertManualToken(s.Identities, convID, raw, body.Label)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	got, err := s.Identities.Get(convID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, identity.PublicView{
		ID:            got.ID,
		Label:         got.Label,
		Scheme:        got.Scheme,
		Source:        got.Source,
		ClaimsSummary: got.ClaimsSummary,
		IsDefault:     got.IsDefault,
	})
}

func (s *Server) handleSetDefaultIdentity(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	if !s.requireConversationAccess(w, r, r.PathValue("id")) {
		return
	}
	if err := s.Identities.SetDefault(r.PathValue("id"), r.PathValue("iid")); err != nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteIdentity(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	if !s.requireConversationAccess(w, r, r.PathValue("id")) {
		return
	}
	if err := s.Identities.Delete(r.PathValue("id"), r.PathValue("iid")); err != nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleClearIdentities(w http.ResponseWriter, r *http.Request) {
	if !s.requireConversationAccess(w, r, r.PathValue("id")) {
		return
	}
	if s.Identities != nil {
		s.Identities.ClearCaptured(r.PathValue("id"))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	out := []conversation.Summary{}
	if s.Messages != nil {
		if sum := s.Messages.ListSummaries(); sum != nil {
			out = sum
		}
	}
	if s.gateTokens().Enabled() {
		filtered, err := s.filterConversationSummaries(r, out)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		out = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

func (s *Server) filterConversationSummaries(r *http.Request, in []conversation.Summary) ([]conversation.Summary, error) {
	ms := s.metaStore()
	if ms == nil {
		return in, nil
	}
	p := s.principalFrom(r.Context())
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	filterOwner := ""
	switch p.Role {
	case controlplane.RoleAdmin:
		if scope == "mine" {
			filterOwner = p.OperatorID
			if filterOwner == "" {
				filterOwner = "admin"
			}
		}
		// default / scope=all: no owner filter
	default:
		// operators always see mine
		filterOwner = p.OperatorID
	}

	allowed := map[string]bool{}
	metas, err := ms.ListMeta(conversation.MetaFilter{OwnerID: filterOwner})
	if err != nil {
		return nil, err
	}
	for _, m := range metas {
		if filterOwner == "" {
			// admin all: still require CanAccess (always true for admin)
			if conversation.CanAccess(p, m) {
				allowed[m.ID] = true
			}
		} else {
			allowed[m.ID] = true
		}
	}
	// Admin all also includes meta-less legacy conversations.
	includeOrphans := p.Role == controlplane.RoleAdmin && scope != "mine"

	out := make([]conversation.Summary, 0, len(in))
	for _, sum := range in {
		if allowed[sum.ID] {
			out = append(out, sum)
			continue
		}
		if includeOrphans {
			_, gerr := ms.GetMeta(sum.ID)
			if errors.Is(gerr, conversation.ErrMetaNotFound) {
				out = append(out, sum)
				continue
			}
			if gerr != nil {
				return nil, gerr
			}
		}
	}
	return out, nil
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if s.Messages == nil {
		writeJSON(w, http.StatusOK, []conversation.Message{})
		return
	}
	msgs := s.Messages.List(convID)
	if msgs == nil {
		msgs = []conversation.Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleClearMessages(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if s.Messages != nil {
		s.Messages.Clear(convID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleDeleteConversation permanently removes a conversation: its message
// history and rolling summary, ownership meta (so it leaves the sidebar list),
// and any captured session identities. An actively running conversation is
// rejected (cancel it first) to avoid deleting state a run is writing to.
func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if s.Store != nil {
		if busy, err := s.Store.HasActiveRun(convID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		} else if busy {
			writeError(w, http.StatusConflict, "conversation_busy",
				"conversation has an active run; cancel it before deleting")
			return
		}
	}
	// Messages + rolling summary (a fully-cleared conversation already drops
	// out of the sidebar summary list).
	if s.Messages != nil {
		s.Messages.Clear(convID)
	}
	// Ownership meta so it cannot reappear and stays out of the meta-backed
	// list / ACL scope.
	if ms := s.metaStore(); ms != nil {
		if err := ms.DeleteMeta(convID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	// Captured session identities for this conversation.
	if s.Identities != nil {
		s.Identities.ClearCaptured(convID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": convID})
}

func (s *Server) handleRollbackMessages(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	msgID := strings.TrimSpace(r.PathValue("message_id"))
	if convID == "" || msgID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation or message id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if s.Messages == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "message store not configured")
		return
	}
	busy, err := s.Store.HasActiveRun(convID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if busy {
		writeError(w, http.StatusConflict, "conversation_busy", "conversation has an active run")
		return
	}

	var body struct {
		Regenerate bool   `json:"regenerate"`
		AgentID    string `json:"agent_id"`
	}
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
			return
		}
	}

	deleted, err := s.Messages.TruncateFrom(convID, msgID)
	if errors.Is(err, conversation.ErrMessageNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "message not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	msgs := s.Messages.List(convID)
	if msgs == nil {
		msgs = []conversation.Message{}
	}
	out := map[string]any{
		"conversation_id": convID,
		"deleted_count":   deleted,
		"messages":        msgs,
	}

	if body.Regenerate {
		input := lastUserMessageContent(msgs)
		lastUser := lastUserMessage(msgs)
		if input == "" {
			writeError(w, http.StatusConflict, "regenerate_unavailable", "no user message to regenerate from")
			return
		}
		agentID := strings.TrimSpace(body.AgentID)
		if agentID == "" {
			agentID = s.DefaultAgentID
		}
		if agentID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
			return
		}
		reuseUser := lastUser != nil && lastUser.Content == input
		runRec, err := s.createAndExecuteRun(r, agentID, input, convID, "", executeRunOpts{
			skipUserAppend: reuseUser,
			rebindUserID:   lastUserID(lastUser),
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		out["regenerated_run"] = map[string]string{
			"run_id": runRec.ID,
			"status": string(runRec.Status),
		}
		out["messages"] = s.Messages.List(convID)
	}

	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleForkConversation(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if s.Messages == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "message store not configured")
		return
	}
	busy, err := s.Store.HasActiveRun(convID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if busy {
		writeError(w, http.StatusConflict, "conversation_busy", "conversation has an active run")
		return
	}

	var body struct {
		ThroughMessageID string `json:"through_message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	throughID := strings.TrimSpace(body.ThroughMessageID)
	if throughID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "through_message_id is required")
		return
	}

	newID, copied, err := s.Messages.Fork(convID, throughID)
	if errors.Is(err, conversation.ErrMessageNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "message not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !s.ensureConversationMeta(w, r, newID) {
		return
	}
	msgs := s.Messages.List(newID)
	if msgs == nil {
		msgs = []conversation.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source_conversation_id": convID,
		"conversation_id":        newID,
		"copied_count":           copied,
		"messages":               msgs,
	})
}

func lastUserMessageContent(msgs []conversation.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == conversation.RoleUser {
			return msgs[i].Content
		}
	}
	return ""
}

func lastUserMessage(msgs []conversation.Message) *conversation.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == conversation.RoleUser {
			m := msgs[i]
			return &m
		}
	}
	return nil
}

func lastUserID(m *conversation.Message) string {
	if m == nil {
		return ""
	}
	return m.ID
}
