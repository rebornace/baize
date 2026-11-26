package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
)

const readStatePrefix = "read_state:"
const readStateMaxConversations = 500
const readStateAllKey = "*"

type readStateJSON map[string]string

// readStateKey scopes read markers to the caller. When the control plane is
// off there is a single local identity.
func (s *Server) readStateKey(ctx context.Context) string {
	id := strings.TrimSpace(controlplane.OperatorIDFrom(ctx))
	if id == "" {
		if controlplane.RoleFrom(ctx) == controlplane.RoleViewer {
			id = "viewer"
		} else {
			id = "local"
		}
	}
	return readStatePrefix + id
}

func (s *Server) loadReadState(ctx context.Context) readStateJSON {
	out := readStateJSON{}
	if s.Store == nil {
		return out
	}
	raw, ok, err := s.Store.GetSetting(s.readStateKey(ctx))
	if err != nil || !ok {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = readStateJSON{}
	}
	return out
}

func (s *Server) saveReadState(ctx context.Context, state readStateJSON) error {
	if len(state) > readStateMaxConversations {
		type entry struct {
			id string
			at time.Time
		}
		entries := make([]entry, 0, len(state))
		for id, raw := range state {
			at, err := parseFlexibleTime(raw)
			if err != nil {
				at = time.Time{}
			}
			entries = append(entries, entry{id: id, at: at})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].at.After(entries[j].at) })
		trimmed := readStateJSON{}
		for i, e := range entries {
			if i >= readStateMaxConversations {
				break
			}
			trimmed[e.id] = state[e.id]
		}
		state = trimmed
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.Store.UpsertSetting(s.readStateKey(ctx), raw)
}

// handleMarkAllConversationsRead sets a single "all read" marker. Per-message
// markers still win when they are newer; new activity after this point becomes
// unread even for conversations never opened individually.
func (s *Server) handleMarkAllConversationsRead(w http.ResponseWriter, r *http.Request) {
	state := s.loadReadState(r.Context())
	now := time.Now().UTC()
	state[readStateAllKey] = now.Format(time.RFC3339Nano)
	if err := s.saveReadState(r.Context(), state); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"read_at": now.Format(time.RFC3339Nano)})
}

// handleMarkConversationRead records "read now" for the caller and one
// conversation. Unread is derived from the latest message timestamp, so a
// timestamp is enough; per-message receipts are not needed.
func (s *Server) handleMarkConversationRead(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	state := s.loadReadState(r.Context())
	now := time.Now().UTC()
	state[convID] = now.Format(time.RFC3339Nano)
	if err := s.saveReadState(r.Context(), state); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"read_at":         now.Format(time.RFC3339Nano),
	})
}

// applySummaryUnread marks conversations with activity newer than the caller's
// last read marker. Conversations never marked read stay quiet so enabling the
// feature does not flood the sidebar with historical unread dots.
func (s *Server) applySummaryUnread(r *http.Request, out []conversation.Summary) {
	state := s.loadReadState(r.Context())
	if len(state) == 0 {
		return
	}
	var allRead time.Time
	if raw, ok := state[readStateAllKey]; ok {
		if t, err := parseFlexibleTime(raw); err == nil {
			allRead = t
		}
	}
	for i := range out {
		raw, ok := state[out[i].ID]
		if !ok {
			if !allRead.IsZero() {
				out[i].Unread = out[i].UpdatedAt.After(allRead)
			}
			continue
		}
		lastRead, err := parseFlexibleTime(raw)
		if err != nil {
			continue
		}
		if allRead.After(lastRead) {
			lastRead = allRead
		}
		out[i].Unread = out[i].UpdatedAt.After(lastRead)
	}
}
