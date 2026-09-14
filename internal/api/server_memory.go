package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/memory"
)

func (s *Server) memoryOwnerID(ctx context.Context) string {
	// Keep in lockstep with prepareConversationMeta owner resolution.
	owner := "local-dev"
	if s.gateTokens().Enabled() {
		if id := controlplane.OperatorIDFrom(ctx); id != "" {
			owner = id
		} else if controlplane.RoleFrom(ctx) == controlplane.RoleAdmin {
			owner = "admin"
		}
	}
	return owner
}

func (s *Server) requireMemoryStore(w http.ResponseWriter) bool {
	if s.Memory == nil {
		writeError(w, http.StatusServiceUnavailable, "memory_unavailable", "memory store not wired")
		return false
	}
	return true
}

func (s *Server) handleListMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemoryStore(w) {
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}
	owner := s.memoryOwnerID(r.Context())
	items, err := s.Memory.List(owner, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if items == nil {
		items = []memory.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePostMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemoryStore(w) {
		return
	}
	var body struct {
		Text string `json:"text"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "text is required")
		return
	}
	owner := s.memoryOwnerID(r.Context())
	out, err := s.Memory.Upsert(memory.Entry{
		OwnerID: owner,
		Key:     strings.TrimSpace(body.Key),
		Text:    text,
		Source:  memory.SourceExplicit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handlePatchMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemoryStore(w) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing memory id")
		return
	}
	entry, err := s.Memory.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "memory_not_found", "memory entry not found")
		return
	}
	owner := s.memoryOwnerID(r.Context())
	if entry.OwnerID != owner {
		writeError(w, http.StatusForbidden, "forbidden", "无权修改该记忆")
		return
	}
	var body struct {
		Text *string `json:"text"`
		Key  *string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Text != nil {
		text := strings.TrimSpace(*body.Text)
		if text == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "text cannot be empty")
			return
		}
		entry.Text = text
	}
	if body.Key != nil {
		entry.Key = strings.TrimSpace(*body.Key)
	}
	entry.Source = memory.SourceExplicit
	out, err := s.Memory.Upsert(entry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if !s.requireMemoryStore(w) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing memory id")
		return
	}
	entry, err := s.Memory.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "memory_not_found", "memory entry not found")
		return
	}
	owner := s.memoryOwnerID(r.Context())
	if entry.OwnerID != owner {
		writeError(w, http.StatusForbidden, "forbidden", "无权删除该记忆")
		return
	}
	if err := s.Memory.Delete(owner, id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
