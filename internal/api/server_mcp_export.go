package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
)

const mcpExportEndpointPath = "/v0/mcp/export"

func (s *Server) mcpExportHTTP() http.Handler {
	return mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store:          s.Store,
		Registry:       s.Registry,
		Identities:     s.Identities,
		HostedSessions: s.HostedSessions,
		HostedWaiter:   s.HostedWaiter,
		PublicBaseURL:  s.publicBaseURL(),
		Enabled:        s.MCPExportEnabled,
		TraceToolCall:  s.MCPExportTrace,
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
		Name          string                        `json:"name"`
		IdentityID    string                        `json:"identity_id"`
		IdentitySlots []store.MCPExportIdentitySlot `json:"identity_slots"`
		AllowedTools  []string                      `json:"allowed_tools"`
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
	slots, err := s.validateExportIdentitySlots(identityID, body.IdentitySlots)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	plaintext, hash, prefix, err := mcpexport.GenerateKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	allowedTools := body.AllowedTools
	if allowedTools == nil {
		allowedTools = []string{}
	}
	key := store.MCPExportKey{
		ID:            "mek_" + uuid.NewString(),
		Name:          name,
		IdentityID:    identityID,
		IdentitySlots: slots,
		KeyHash:       hash,
		Prefix:        prefix,
		AllowedTools:  allowedTools,
	}
	if err := s.Store.InsertMCPExportKey(key); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	got, _ := s.Store.GetMCPExportKey(key.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":             got.ID,
		"name":           got.Name,
		"identity_id":    got.IdentityID,
		"identity_slots": got.IdentitySlots,
		"allowed_tools":  got.AllowedTools,
		"token":          plaintext,
		"prefix":         got.Prefix,
	})
}

func (s *Server) handlePatchMCPExportKey(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing key id")
		return
	}
	existing, err := s.Store.GetMCPExportKey(id)
	if err != nil {
		if errors.Is(err, store.ErrMCPExportKeyNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var body struct {
		Name          *string                        `json:"name"`
		AllowedTools  *[]string                      `json:"allowed_tools"`
		IdentitySlots *[]store.MCPExportIdentitySlot `json:"identity_slots"`
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
	if body.AllowedTools != nil {
		existing.AllowedTools = *body.AllowedTools
	}
	if body.IdentitySlots != nil {
		slots, err := s.validateExportIdentitySlots(existing.IdentityID, *body.IdentitySlots)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		existing.IdentitySlots = slots
	}
	if err := s.Store.UpdateMCPExportKey(existing); err != nil {
		if errors.Is(err, store.ErrMCPExportKeyNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	got, err := s.Store.GetMCPExportKey(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, got)
}

// validateExportIdentitySlots ensures every referenced identity exists and that
// a default (empty connector) slot is present — synthesizing one from
// defaultIdentityID when the client omitted it.
func (s *Server) validateExportIdentitySlots(defaultIdentityID string, slots []store.MCPExportIdentitySlot) ([]store.MCPExportIdentitySlot, error) {
	defaultIdentityID = strings.TrimSpace(defaultIdentityID)
	if _, err := s.Store.GetMCPExportIdentity(defaultIdentityID); err != nil {
		if errors.Is(err, store.ErrMCPExportIdentityNotFound) {
			return nil, errors.New("unknown identity_id")
		}
		return nil, err
	}
	out := make([]store.MCPExportIdentitySlot, 0, len(slots)+1)
	hasDefault := false
	for _, slot := range slots {
		id := strings.TrimSpace(slot.IdentityID)
		if id == "" {
			return nil, errors.New("identity_slots[].identity_id is required")
		}
		if _, err := s.Store.GetMCPExportIdentity(id); err != nil {
			if errors.Is(err, store.ErrMCPExportIdentityNotFound) {
				return nil, fmt.Errorf("unknown identity_slots identity_id %q (identity was deleted or never created)", id)
			}
			return nil, err
		}
		cleaned := store.MCPExportIdentitySlot{
			ConnectorID: strings.TrimSpace(slot.ConnectorID),
			Role:        strings.TrimSpace(slot.Role),
			IdentityID:  id,
		}
		if cleaned.ConnectorID == "" {
			hasDefault = true
		}
		out = append(out, cleaned)
	}
	if !hasDefault {
		out = append([]store.MCPExportIdentitySlot{{IdentityID: defaultIdentityID}}, out...)
	}
	return out, nil
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
