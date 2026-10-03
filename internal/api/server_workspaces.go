package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	wst := s.workspaceStore()
	if wst == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"workspaces": []conversation.Workspace{{
				ID:   conversation.DefaultWorkspaceID,
				Name: conversation.DefaultWorkspaceName,
			}},
		})
		return
	}
	list, err := wst.ListWorkspaces()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if list == nil {
		list = []conversation.Workspace{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": list})
}

func (s *Server) handleCreateWorkspace(w http.ResponseWriter, r *http.Request) {
	wst := s.workspaceStore()
	if wst == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "workspace store not configured")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	ws, err := wst.CreateWorkspace(strings.TrimSpace(body.Name))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ws)
}
