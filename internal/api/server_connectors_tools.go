package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/connector/specimport"
	"github.com/rebornace/baize/internal/connector/specstore"
	"github.com/rebornace/baize/internal/skill/loginmanage"
	"github.com/rebornace/baize/internal/store"
)

func (s *Server) handlePutConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing connector id")
		return
	}
	var body struct {
		Type                 string          `json:"type"`
		Spec                 string          `json:"spec"`
		SpecContent          string          `json:"spec_content"`
		SpecURL              string          `json:"spec_url"`
		ImportFormat         string          `json:"import_format"`
		BaseURL              string          `json:"base_url"`
		ExecutionCallbackURL *string         `json:"execution_callback_url"`
		RequireApproval      *[]string       `json:"require_approval"`
		RequireLogin         *[]string       `json:"require_login"`
		Auth                 authBody        `json:"auth"`
		MCP                  store.MCPConfig `json:"mcp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Type == "" {
		body.Type = "openapi"
	}
	if body.Type != "openapi" && body.Type != "http" && body.Type != "mcp" {
		writeError(w, http.StatusBadRequest, "invalid_request", "unsupported connector type")
		return
	}
	if body.Spec != "" && body.SpecContent != "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "spec and spec_content are mutually exclusive")
		return
	}
	if body.Spec != "" && strings.TrimSpace(body.SpecURL) != "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "spec and spec_url are mutually exclusive")
		return
	}
	if body.SpecContent != "" && strings.TrimSpace(body.SpecURL) != "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "spec_content and spec_url are mutually exclusive")
		return
	}
	const maxSpecContentSize = 4 << 20
	if len(body.SpecContent) > maxSpecContentSize {
		writeError(w, http.StatusBadRequest, "invalid_request", "spec_content exceeds 4 MiB limit")
		return
	}

	importFormat := body.ImportFormat
	if importFormat == "" {
		importFormat = specimport.FormatAuto
	}
	if !validImportFormat(importFormat) {
		writeError(w, http.StatusBadRequest, "unsupported_import_format", "unsupported import_format")
		return
	}

	specPath := body.Spec
	importFormatDetected := ""
	specImportContent := body.SpecContent
	if strings.TrimSpace(body.SpecURL) != "" {
		fetched, err := specimport.FetchSpecFromURL(body.SpecURL)
		if err != nil {
			if errors.Is(err, specimport.ErrInvalidSpecURL) {
				writeError(w, http.StatusBadRequest, "invalid_spec_url", err.Error())
				return
			}
			if errors.Is(err, specimport.ErrFetchBlocked) {
				writeError(w, http.StatusBadRequest, "spec_fetch_blocked", err.Error())
				return
			}
			if errors.Is(err, specimport.ErrFetchFailed) {
				writeError(w, http.StatusBadRequest, "spec_fetch_failed", err.Error())
				return
			}
			if errors.Is(err, specimport.ErrInvalidSpec) {
				writeError(w, http.StatusBadRequest, "invalid_spec", err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		specImportContent = string(fetched)
	}
	if specImportContent != "" {
		if s.Blobs == nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "blob store is not configured")
			return
		}
		if len(specImportContent) > maxSpecContentSize {
			writeError(w, http.StatusBadRequest, "invalid_request", "spec_content exceeds 4 MiB limit")
			return
		}
		content := []byte(specImportContent)
		normalized, detected, err := specimport.Normalize(content, importFormat, body.BaseURL)
		if err != nil {
			if errors.Is(err, specimport.ErrUnsupportedFormat) {
				writeError(w, http.StatusBadRequest, "unsupported_import_format", err.Error())
				return
			}
			if errors.Is(err, specimport.ErrInvalidSpec) {
				writeError(w, http.StatusBadRequest, "invalid_spec", err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		specKey, err := specstore.Write(r.Context(), s.Blobs, id, content, normalized)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		specPath = specKey
		importFormatDetected = detected
	} else if body.Type == "openapi" && specPath == "" {
		existing, err := s.Store.GetConnector(id)
		if err != nil || strings.TrimSpace(existing.Spec) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "spec is required")
			return
		}
		if specstore.IsBlobSpecKey(existing.Spec) {
			if s.Blobs == nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "blob store is not configured")
				return
			}
			if _, err := s.Blobs.Get(r.Context(), existing.Spec); err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "spec is required")
				return
			}
		} else if specstore.IsLegacyConnectorFSPath(existing.Spec) {
			writeError(w, http.StatusBadRequest, "invalid_request", "filesystem spec path is no longer supported; re-import via spec_content or spec_url")
			return
		} else if _, err := os.Stat(existing.Spec); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "spec is required")
			return
		}
		specPath = existing.Spec
		importFormatDetected = existing.ImportFormat
	}
	if body.Type == "openapi" && specPath == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "spec is required")
		return
	}
	if body.Type == "http" && strings.TrimSpace(body.BaseURL) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "base_url is required")
		return
	}

	// Persist the auth configuration shape (mode + references), not the
	// resolved secrets, on the stored Connector. Capture defaults / clearing
	// for HTTP are handled inside connector.Apply. MCP connectors ignore auth.
	// Single GetConnector when either capture or execution_callback_url is
	// omitted so both preserve-on-omit paths share one store read.
	var existingConn store.Connector
	var hasExisting bool
	needExisting := body.ExecutionCallbackURL == nil || (body.Type != "mcp" && body.Auth.Capture == nil) || (body.Type == "mcp" && body.MCP.OAuth != nil)
	if needExisting {
		if existing, err := s.Store.GetConnector(id); err == nil {
			existingConn = existing
			hasExisting = true
		}
	}

	var connectorAuth store.ConnectorAuth
	if body.Type != "mcp" {
		capture := store.CaptureAuth{}
		if body.Auth.Capture != nil {
			capture = store.CaptureAuth{
				ToolNameGlob:   body.Auth.Capture.ToolNameGlob,
				TokenJSONPaths: body.Auth.Capture.TokenJSONPaths,
				LabelJSONPaths: body.Auth.Capture.LabelJSONPaths,
				HeaderTemplate: body.Auth.Capture.HeaderTemplate,
				DefaultScheme:  body.Auth.Capture.DefaultScheme,
			}
		} else if hasExisting {
			// 连接器页不再编辑 capture；省略时保留 Tools 页配置的登录捕获，
			// 避免一次普通保存把本人登录链路清空。
			capture = existingConn.Auth.Capture
		}
		connectorAuth = store.ConnectorAuth{
			Mode: body.Auth.Mode,
			Static: store.StaticAuth{
				Headers: body.Auth.Static.Headers,
			},
			Passthrough: store.PassThruAuth{
				Headers: body.Auth.Passthrough.Headers,
			},
			VaultRef: store.VaultRefAuth{
				Headers: body.Auth.VaultRef.Headers,
			},
			Capture: capture,
		}
	}

	callbackURL := ""
	if body.ExecutionCallbackURL != nil {
		callbackURL = strings.TrimSpace(*body.ExecutionCallbackURL)
	} else if hasExisting {
		callbackURL = existingConn.ExecutionCallbackURL
	}

	if body.Type == "mcp" {
		mergeMCPOAuthPreserveSecrets(&body.MCP, existingConn, hasExisting)
	}

	c, infos, err := connector.Apply(connector.ApplyInput{
		Store:                s.Store,
		Registry:             s.Registry,
		Identities:           s.Identities,
		Blobs:                s.Blobs,
		ID:                   id,
		Type:                 body.Type,
		Spec:                 specPath,
		ImportFormat:         importFormatDetected,
		BaseURL:              body.BaseURL,
		ExecutionCallbackURL: callbackURL,
		RequireApproval:      body.RequireApproval,
		RequireLogin:         body.RequireLogin,
		Auth:                 connectorAuth,
		MCP:                  body.MCP,
		CallbackSigner:       s.CallbackSigner,
		CallbackSecret:       s.CallbackSecret,
		CallbackPublicBase:   s.publicBaseURL(),
		CallbackTTL:          s.CallbackTTL,
	})
	if err != nil {
		if errors.Is(err, authcred.ErrInvalidAuth) {
			writeError(w, http.StatusBadRequest, "invalid_auth", err.Error())
			return
		}
		if errors.Is(err, httpplugin.ErrInvalidPlugin) {
			writeError(w, http.StatusBadRequest, "invalid_plugin", err.Error())
			return
		}
		if errors.Is(err, mcpbridge.ErrInvalidMCP) {
			writeError(w, http.StatusBadRequest, "invalid_mcp", err.Error())
			return
		}
		if errors.Is(err, httpplugin.ErrToolConflict) || errors.Is(err, openapi.ErrToolConflict) {
			writeError(w, http.StatusConflict, "tool_conflict", err.Error())
			return
		}
		if errors.Is(err, openapi.ErrInvalidSpec) {
			writeError(w, http.StatusBadRequest, "invalid_spec", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	// Single-process single-active-connector assumption (matches existing
	// runtime): a successful PUT updates the server's active auth mode and
	// passthrough whitelist so subsequent POST /runs pick the right headers.
	if body.Type != "mcp" {
		s.AuthMode = authcred.NormalizeMode(body.Auth.Mode)
		s.AuthWhitelist = body.Auth.Passthrough.Headers
	}

	resp := map[string]any{
		"id":                     c.ID,
		"type":                   c.Type,
		"spec":                   c.Spec,
		"import_format_detected": c.ImportFormat,
		"base_url":               c.BaseURL,
		"execution_callback_url": c.ExecutionCallbackURL,
		"require_approval":       nonNilStrings(c.RequireApproval),
		"require_login":          nonNilStrings(c.RequireLogin),
		"auth":                   c.Auth,
		"tools":                  infos,
	}
	if c.Type == "mcp" {
		resp["mcp"] = redactMCPForAPI(c.MCP)
	}
	s.syncLoginManagedSkill(id)
	writeJSON(w, http.StatusOK, resp)
}

// syncLoginManagedSkill refreshes the managed login skill for one connector
// then reloads the skill catalog. Sync/Reload failures are logged only so the
// connector mutation HTTP response stays successful.
func (s *Server) syncLoginManagedSkill(connectorID string) {
	if s == nil || s.SkillCatalog == nil || s.SkillCatalog.Blobs == nil {
		return
	}
	if err := loginmanage.SyncConnector(s.Store, s.SkillCatalog.Blobs, connectorID); err != nil {
		log.Printf("loginmanage: sync connector %q: %v", connectorID, err)
		return
	}
	if err := s.SkillCatalog.Reload(); err != nil {
		log.Printf("loginmanage: reload skills after sync %q: %v", connectorID, err)
	}
}

func validImportFormat(format string) bool {
	switch format {
	case specimport.FormatAuto, specimport.FormatOpenAPI3, specimport.FormatSwagger2, specimport.FormatPostman:
		return true
	default:
		return false
	}
}

// authBody mirrors store.ConnectorAuth / authcred.Config for JSON decoding of
// PUT /v0/connectors/{id} body.auth. Kept local to avoid pulling store types
// into the request body struct directly.
type authBody struct {
	Mode   string `json:"mode"`
	Static struct {
		Headers map[string]string `json:"headers"`
	} `json:"static"`
	Passthrough struct {
		Headers []string `json:"headers"`
	} `json:"passthrough"`
	VaultRef struct {
		Headers map[string]string `json:"headers"`
	} `json:"vault_ref"`
	Capture *struct {
		ToolNameGlob   string   `json:"tool_name_glob"`
		TokenJSONPaths []string `json:"token_json_paths"`
		LabelJSONPaths []string `json:"label_json_paths"`
		HeaderTemplate string   `json:"header_template"`
		DefaultScheme  string   `json:"default_scheme"`
	} `json:"capture"`
}

func (s *Server) handleListConnectors(w http.ResponseWriter, r *http.Request) {
	wantType := strings.TrimSpace(r.URL.Query().Get("type"))
	all := s.Store.ListConnectors()
	out := make([]map[string]any, 0, len(all))
	for _, c := range all {
		if wantType != "" && c.Type != wantType {
			continue
		}
		out = append(out, s.connectorResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": out})
}

func (s *Server) handleGetConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	writeJSON(w, http.StatusOK, s.connectorResponse(c))
}

func (s *Server) connectorResponse(c store.Connector) map[string]any {
	tools := s.Store.ListToolsByConnector(c.ID)
	if tools == nil {
		tools = []store.Tool{}
	}
	resp := map[string]any{
		"id":                     c.ID,
		"type":                   c.Type,
		"spec":                   c.Spec,
		"import_format_detected": c.ImportFormat,
		"base_url":               c.BaseURL,
		"execution_callback_url": c.ExecutionCallbackURL,
		"require_approval":       nonNilStrings(c.RequireApproval),
		"require_login":          nonNilStrings(c.RequireLogin),
		"auth":                   c.Auth,
		"tools":                  tools,
	}
	if c.Type == "mcp" {
		resp["mcp"] = redactMCPForAPI(c.MCP)
	}
	return resp
}

func (s *Server) handleGetTools(w http.ResponseWriter, r *http.Request) {
	tools := s.Store.ListTools()
	if tools == nil {
		tools = []store.Tool{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tools": tools})
}

func (s *Server) handlePatchTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing tool name")
		return
	}
	var body struct {
		Enabled         *bool   `json:"enabled"`
		RequireLogin    *bool   `json:"require_login"`
		RequireApproval *bool   `json:"require_approval"`
		Title           *string `json:"title"`
		Description     *string `json:"description"`
		Export          *string `json:"export"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Enabled == nil && body.RequireLogin == nil && body.RequireApproval == nil && body.Title == nil && body.Description == nil && body.Export == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "at least one of enabled, require_login, require_approval, title, description, or export is required")
		return
	}
	if body.Export != nil {
		switch strings.TrimSpace(*body.Export) {
		case "", "default", "force_allow", "force_deny":
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", "export must be default, force_allow, or force_deny")
			return
		}
	}

	row, err := s.Store.GetTool(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "tool not found")
		return
	}

	// Track whether enabled actually changes. Only an enabled transition
	// (false→true) needs a full re-register to restore the invoker closure and
	// mutating HITL. Toggling only require_login / require_approval on an
	// already-enabled row is served by Registry in-place setters so we don't
	// drop the baked-in RequireApproval flag (RegisterOneFromConnector does not
	// know RequireApprovalMutating and would otherwise lose mutating HITL).
	// Export-only patches update the store catalog and never re-register.
	enabledChanged := body.Enabled != nil && *body.Enabled != row.Enabled

	if body.Enabled != nil {
		row.Enabled = *body.Enabled
	}
	if body.RequireLogin != nil {
		row.RequireLogin = *body.RequireLogin
	}
	if body.RequireApproval != nil {
		row.RequireApproval = *body.RequireApproval
	}
	if body.Title != nil {
		row.Title = *body.Title
	}
	if body.Description != nil {
		row.Description = *body.Description
		row.DescriptionCustom = true
	}
	if body.Export != nil {
		export := strings.TrimSpace(*body.Export)
		if export == "" {
			export = "default"
		}
		if export == "default" {
			row.Export = ""
		} else {
			row.Export = export
		}
	}
	s.Store.UpsertTool(row)

	c, err := s.Store.GetConnector(row.ConnectorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if !row.Enabled {
		s.Registry.Unregister(name)
	} else if enabledChanged {
		// false→true: re-register to rebuild the invoker closure and restore
		// mutating HITL from the baked-in row.RequireApproval flag.
		if err := s.registerOne(c, row); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	} else {
		// Row stays enabled. Prefer in-place Registry updates so we don't drop
		// the baked-in RequireApproval flag. Title-only patches skip Registry.
		if body.RequireLogin != nil {
			if err := s.Registry.SetRequireLogin(name, row.RequireLogin); err != nil {
				// Tool is enabled in the store but not in the Registry (e.g., a
				// recovery path after a failed registration). Fall back to a full
				// re-register so the row and Registry converge.
				if err := s.registerOne(c, row); err != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
					return
				}
			}
		}
		if body.RequireApproval != nil {
			if err := s.Registry.SetRequireApproval(name, row.RequireApproval); err != nil {
				if err := s.registerOne(c, row); err != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
					return
				}
			}
		}
		if body.Description != nil {
			if err := s.Registry.SetDescription(name, row.Description); err != nil {
				if err := s.registerOne(c, row); err != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
					return
				}
			}
		}
	}

	c.RequireLogin = syncRequireLoginList(c.RequireLogin, name, row.RequireLogin)
	c.RequireApproval = syncRequireLoginList(c.RequireApproval, name, row.RequireApproval)
	s.Store.UpsertConnector(c)

	s.syncLoginManagedSkill(row.ConnectorID)
	writeJSON(w, http.StatusOK, row)
}

// registerOne re-registers a single enabled catalog row into the Registry by
// delegating to connector.RegisterOneFromConnector, which rebuilds the same
// invoker closure Apply uses. It does not write the Store; the caller is
// responsible for UpsertTool before calling.
func (s *Server) registerOne(c store.Connector, t store.Tool) error {
	return connector.RegisterOneFromConnector(s.Store, s.Registry, s.Identities, s.Blobs, c, t, connector.CallbackConfig{
		Signer:     s.CallbackSigner,
		Secret:     s.CallbackSecret,
		PublicBase: s.publicBaseURL(),
		TTL:        s.CallbackTTL,
	})
}

func (s *Server) handlePostConnectorTool(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "connector not found")
		return
	}
	typ := strings.TrimSpace(c.Type)
	if typ == "" {
		typ = "openapi"
	}
	if typ != "openapi" {
		writeError(w, http.StatusBadRequest, "invalid_request", "extra tools are only supported on openapi connectors")
		return
	}

	var body struct {
		Name            string         `json:"name"`
		Title           string         `json:"title"`
		Description     string         `json:"description"`
		Method          string         `json:"method"`
		Path            string         `json:"path"`
		InputSchema     map[string]any `json:"input_schema"`
		RequireLogin    bool           `json:"require_login"`
		RequireApproval bool           `json:"require_approval"`
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
	method := strings.ToUpper(strings.TrimSpace(body.Method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "method must be one of GET, POST, PUT, PATCH, DELETE")
		return
	}
	if !strings.HasPrefix(body.Path, "/") {
		writeError(w, http.StatusBadRequest, "invalid_request", "path must start with /")
		return
	}
	schema := body.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}

	if _, err := s.Store.GetTool(name); err == nil {
		writeError(w, http.StatusConflict, "conflict", "tool name already exists")
		return
	}

	row := store.Tool{
		ConnectorID:       id,
		Name:              name,
		Source:            store.ToolSourceExtra,
		Enabled:           true,
		Title:             strings.TrimSpace(body.Title),
		Description:       body.Description,
		DescriptionCustom: true,
		Method:            method,
		Path:              body.Path,
		InputSchema:       schema,
		RequireLogin:      body.RequireLogin,
		RequireApproval:   body.RequireApproval,
	}
	s.Store.UpsertTool(row)

	if err := s.registerOne(c, row); err != nil {
		// Roll back the row so the catalog and Registry stay consistent.
		_ = s.Store.DeleteTool(name)
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	c.RequireLogin = syncRequireLoginList(c.RequireLogin, name, body.RequireLogin)
	if body.RequireApproval {
		c.RequireApproval = syncRequireApprovalList(c.RequireApproval, name)
	}
	s.Store.UpsertConnector(c)

	s.syncLoginManagedSkill(id)
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handleDeleteConnectorTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	row, err := s.Store.GetTool(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "tool not found")
		return
	}
	if row.Source != store.ToolSourceExtra {
		writeError(w, http.StatusBadRequest, "invalid_request", "only extra tools can be deleted")
		return
	}
	if err := s.Store.DeleteTool(name); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.Registry.Unregister(name)

	c, err := s.Store.GetConnector(row.ConnectorID)
	if err == nil {
		c.RequireLogin = removeFromList(c.RequireLogin, name)
		c.RequireApproval = removeFromList(c.RequireApproval, name)
		s.Store.UpsertConnector(c)
	}

	s.syncLoginManagedSkill(row.ConnectorID)
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteConnector cascades a connector away: it first ensures the
// connector exists (404 connector_not_found otherwise), unregisters all its
// tools from the in-process Registry, then deletes the connector row and its
// tools from the Store. On success it returns 204 No Content, matching the
// single-tool DELETE handler.
func (s *Server) handleDeleteConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing connector id")
		return
	}
	if _, err := s.Store.GetConnector(id); err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	if s.Blobs != nil {
		if err := blob.DeletePrefix(r.Context(), s.Blobs, blob.PrefixConnectors+id+"/"); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	s.Registry.UnregisterConnector(id)
	if err := s.Store.DeleteConnector(id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	s.syncLoginManagedSkill(id)
	w.WriteHeader(http.StatusNoContent)
}

func removeFromList(list []string, name string) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		if n != name {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func syncRequireApprovalList(list []string, name string) []string {
	out := syncRequireLoginList(list, name, true)
	return out
}

func syncRequireLoginList(list []string, name string, require bool) []string {
	out := make([]string, 0, len(list)+1)
	seen := false
	for _, n := range list {
		if n == name {
			seen = true
			if require {
				out = append(out, n)
			}
			continue
		}
		out = append(out, n)
	}
	if require && !seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
