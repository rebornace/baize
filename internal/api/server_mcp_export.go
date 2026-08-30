package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
)

const mcpExportEndpointPath = "/v0/mcp/export"

func (s *Server) mcpExportHTTP() http.Handler {
	return mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store:      s.Store,
		Registry:   s.Registry,
		Identities: s.Identities,
		Enabled:    s.MCPExportEnabled,
	})
}

func (s *Server) handleGetMCPExportSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       s.MCPExportEnabled,
		"endpoint_path": mcpExportEndpointPath,
	})
}

func (s *Server) handleListMCPExportIdentities(w http.ResponseWriter, r *http.Request) {
	ids, err := s.Store.ListMCPExportIdentities()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if ids == nil {
		ids = []store.MCPExportIdentity{}
	}
	writeJSON(w, http.StatusOK, ids)
}

func (s *Server) handlePostMCPExportIdentity(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID      string            `json:"id"`
		Name    string            `json:"name"`
		Scheme  string            `json:"scheme"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		id = "mei_" + uuid.NewString()
	}
	identity := store.MCPExportIdentity{
		ID:      id,
		Name:    name,
		Scheme:  strings.TrimSpace(body.Scheme),
		Headers: body.Headers,
	}
	if err := s.Store.UpsertMCPExportIdentity(identity); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	got, err := s.Store.GetMCPExportIdentity(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, got)
}

func (s *Server) handleGetMCPExportIdentity(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing identity id")
		return
	}
	got, err := s.Store.GetMCPExportIdentity(id)
	if err != nil {
		if errors.Is(err, store.ErrMCPExportIdentityNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "identity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) handlePatchMCPExportIdentity(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing identity id")
		return
	}
	existing, err := s.Store.GetMCPExportIdentity(id)
	if err != nil {
		if errors.Is(err, store.ErrMCPExportIdentityNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "identity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var body struct {
		Name    *string            `json:"name"`
		Scheme  *string            `json:"scheme"`
		Headers *map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		existing.Name = name
	}
	if body.Scheme != nil {
		existing.Scheme = strings.TrimSpace(*body.Scheme)
	}
	if body.Headers != nil {
		existing.Headers = *body.Headers
	}
	if err := s.Store.UpsertMCPExportIdentity(existing); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	got, err := s.Store.GetMCPExportIdentity(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, got)
}

func (s *Server) handleDeleteMCPExportIdentity(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing identity id")
		return
	}
	if err := s.Store.DeleteMCPExportIdentity(id); err != nil {
		if errors.Is(err, store.ErrMCPExportIdentityNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "identity not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListMCPExportKeys(w http.ResponseWriter, r *http.Request) {
	identityID := strings.TrimSpace(r.URL.Query().Get("identity_id"))
	keys, err := s.Store.ListMCPExportKeys(identityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if keys == nil {
		keys = []store.MCPExportKey{}
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) handlePostMCPExportKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string `json:"name"`
		IdentityID string `json:"identity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	identityID := strings.TrimSpace(body.IdentityID)
	if identityID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "identity_id is required")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}
	if _, err := s.Store.GetMCPExportIdentity(identityID); err != nil {
		if errors.Is(err, store.ErrMCPExportIdentityNotFound) {
			writeError(w, http.StatusBadRequest, "invalid_request", "unknown identity_id")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	plaintext, hash, prefix, err := mcpexport.GenerateKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	key := store.MCPExportKey{
		ID:         "mek_" + uuid.NewString(),
		Name:       name,
		IdentityID: identityID,
		KeyHash:    hash,
		Prefix:     prefix,
	}
	if err := s.Store.InsertMCPExportKey(key); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":          key.ID,
		"name":        key.Name,
		"identity_id": key.IdentityID,
		"token":       plaintext,
		"prefix":      key.Prefix,
	})
}

func (s *Server) handleDeleteMCPExportKey(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing key id")
		return
	}
	if err := s.Store.RevokeMCPExportKey(id); err != nil {
		if errors.Is(err, store.ErrMCPExportKeyNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}