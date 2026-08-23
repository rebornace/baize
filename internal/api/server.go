package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/ui"
	"github.com/rebornace/baize/internal/webhook"
)

// Runner executes and resumes runs (implemented by run.Engine).
type Runner interface {
	Execute(ctx context.Context, runID string, ag agent.Def, input string) error
	ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error
}

type Server struct {
	Store          store.Store
	Registry       *tool.Registry
	Runner         Runner
	SkillCatalog   *skill.Catalog
	Identities     identity.Store
	Messages       conversation.Store // optional; nil = no message persistence
	Hub            *eventbus.Hub      // optional; nil = SSE replay only (no live fan-out)
	DefaultAgentID string
	// AuthMode is the active connector's normalized auth mode. Only "passthrough"
	// changes POST /runs and POST /runs/{id}/resume behavior to pick headers.
	AuthMode string
	// AuthWhitelist is the passthrough header whitelist. nil → default
	// ["Authorization"]; len==0 → no headers. Only used when AuthMode=="passthrough".
	AuthWhitelist []string
	// OperatorToken / AdminToken configure the control-plane gate. When both
	// are empty the gate is off and all routes behave as before. When at least
	// one is set, /v0 routes require a Bearer token whose role meets MinRole.
	OperatorToken string
	AdminToken    string
	Webhook       *webhook.Dispatcher // optional; nil = no outbound webhook delivery
	mux           *http.ServeMux
}

func NewServer(st store.Store, reg *tool.Registry, runner Runner) *Server {
	s := &Server{
		Store:      st,
		Registry:   reg,
		Runner:     runner,
		Identities: identity.NewMemoryStore(),
		mux:        http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2, ok := s.authorize(w, r)
		if !ok {
			return
		}
		s.mux.ServeHTTP(w, r2)
	})
}

// authorize enforces the control-plane gate. It returns the (possibly
// context-enriched) request and true to proceed, or writes an error response
// and returns false to short-circuit. When the gate is off it is a no-op.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	path := r.URL.Path
	if path == "/healthz" || strings.HasPrefix(path, "/ui/") || path == "/ui" {
		return r, true
	}
	tok := controlplane.Tokens{Operator: s.OperatorToken, Admin: s.AdminToken}
	if !tok.Enabled() {
		return r, true
	}
	min := controlplane.MinRole(r.Method, r.URL.Path)
	if min == controlplane.RoleNone {
		return r, true
	}
	role, ok := controlplane.Authenticate(r.Header.Get("Authorization"), tok)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "需要控制面口令")
		return nil, false
	}
	if !role.AtLeast(min) {
		writeError(w, http.StatusForbidden, "forbidden", "需要管理员口令")
		return nil, false
	}
	// 控制面口令只用于过门禁。门开着且鉴权成功后，从交给 mux 的 request 上
	// 删掉 Authorization，避免 passthrough 把它 PickHeaders 进 Run 的
	// passthrough_json（SQLite），进而被机器路径打到下游 API。门关着时
	// authorize 在上面已提前 return，不会执行到这里，故不影响现有 passthrough。
	r.Header.Del("Authorization")
	return r.WithContext(controlplane.WithRole(r.Context(), role)), true
}

func (s *Server) routes() {
	s.mux.Handle("/ui/", ui.Handler())
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /v0/ui-config", s.handleUIConfig)
	s.mux.HandleFunc("GET /v0/me", s.handleMe)
	s.mux.HandleFunc("PUT /v0/agents/{id}", s.handlePutAgent)
	s.mux.HandleFunc("GET /v0/agents/{id}", s.handleGetAgent)
	s.mux.HandleFunc("PUT /v0/connectors/{id}", s.handlePutConnector)
	s.mux.HandleFunc("GET /v0/connectors/{id}", s.handleGetConnector)
	s.mux.HandleFunc("GET /v0/tools", s.handleGetTools)
	s.mux.HandleFunc("PATCH /v0/tools/{name}", s.handlePatchTool)
	s.mux.HandleFunc("POST /v0/connectors/{id}/tools", s.handlePostConnectorTool)
	s.mux.HandleFunc("DELETE /v0/connectors/{id}/tools/{name}", s.handleDeleteConnectorTool)
	s.mux.HandleFunc("GET /v0/skills", s.handleListSkills)
	s.mux.HandleFunc("GET /v0/skills/{id}", s.handleGetSkill)
	s.mux.HandleFunc("POST /v0/skills", s.handlePostSkill)
	s.mux.HandleFunc("DELETE /v0/skills/{id}", s.handleDeleteSkill)
	s.mux.HandleFunc("POST /v0/runs", s.handlePostRun)
	s.mux.HandleFunc("POST /v0/runs/{id}/resume", s.handlePostResume)
	s.mux.HandleFunc("GET /v0/runs/{id}/events", s.handleGetEvents)
	s.mux.HandleFunc("GET /v0/runs/{id}/stream", s.handleRunStream)
	s.mux.HandleFunc("GET /v0/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("GET /v0/conversations/{id}/identities", s.handleListIdentities)
	s.mux.HandleFunc("POST /v0/conversations/{id}/identities/{iid}/default", s.handleSetDefaultIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities/{iid}", s.handleDeleteIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities", s.handleClearIdentities)
	s.mux.HandleFunc("GET /v0/conversations", s.handleListConversations)
	s.mux.HandleFunc("GET /v0/conversations/{id}/messages", s.handleListMessages)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/messages", s.handleClearMessages)
	s.mux.HandleFunc("GET /v0/settings/events-webhook", s.handleGetEventsWebhook)
	s.mux.HandleFunc("PUT /v0/settings/events-webhook", s.handlePutEventsWebhook)
	s.mux.HandleFunc("POST /v0/settings/events-webhook/test", s.handlePostEventsWebhookTest)
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Error apiError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type uiConfig struct {
	AgentID     string `json:"agent_id"`
	GateEnabled bool   `json:"gate_enabled"`
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, uiConfig{
		AgentID:     s.DefaultAgentID,
		GateEnabled: controlplane.Tokens{Operator: s.OperatorToken, Admin: s.AdminToken}.Enabled(),
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"role": string(controlplane.RoleFrom(r.Context()))})
}

func (s *Server) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing agent id")
		return
	}
	var body struct {
		System string   `json:"system"`
		Skills []string `json:"skills"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	ag := store.Agent{ID: id, System: body.System, Skills: body.Skills}
	s.Store.UpsertAgent(ag)
	writeJSON(w, http.StatusOK, ag)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing agent id")
		return
	}
	ag, err := s.Store.GetAgent(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}
	writeJSON(w, http.StatusOK, ag)
}

type skillSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tools       []string `json:"tools"`
	Source      string   `json:"source"`
}

func skillSummaryFrom(p skill.Package) skillSummary {
	tools := p.Tools
	if tools == nil {
		tools = []string{}
	}
	return skillSummary{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Tools:       tools,
		Source:      p.Source,
	}
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	pkgs := s.SkillCatalog.List()
	out := make([]skillSummary, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, skillSummaryFrom(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"skills": out})
}

func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	p, ok := s.SkillCatalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "skill not found")
		return
	}
	sum := skillSummaryFrom(p)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          sum.ID,
		"name":        sum.Name,
		"description": sum.Description,
		"tools":       sum.Tools,
		"source":      sum.Source,
		"body":        p.Body,
	})
}

func (s *Server) handlePostSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid multipart form")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "file is required")
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	var pkg skill.Package
	switch ext {
	case ".md":
		pkg, err = s.SkillCatalog.InstallMD(hdr.Filename, raw)
	case ".zip":
		pkg, err = s.SkillCatalog.InstallZip(raw)
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "file must be .md or .zip")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skillSummaryFrom(pkg))
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.SkillCatalog == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "skill catalog not configured")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing skill id")
		return
	}
	if err := s.SkillCatalog.DeleteUser(id); err != nil {
		switch {
		case errors.Is(err, skill.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", err.Error())
		case errors.Is(err, skill.ErrBuiltin):
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		default:
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	for _, a := range s.Store.ListAgents() {
		next := make([]string, 0, len(a.Skills))
		changed := false
		for _, sid := range a.Skills {
			if sid == id {
				changed = true
				continue
			}
			next = append(next, sid)
		}
		if changed {
			a.Skills = next
			s.Store.UpsertAgent(a)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePutConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing connector id")
		return
	}
	var body struct {
		Type                   string          `json:"type"`
		Spec                   string          `json:"spec"`
		BaseURL                string          `json:"base_url"`
		ExecutionCallbackURL   string          `json:"execution_callback_url"`
		RequireApproval        []string        `json:"require_approval"`
		RequireLogin           *[]string       `json:"require_login"`
		Auth                   authBody        `json:"auth"`
		MCP                    store.MCPConfig `json:"mcp"`
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
	if body.Type == "openapi" && body.Spec == "" {
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
	var connectorAuth store.ConnectorAuth
	if body.Type != "mcp" {
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
			Capture: store.CaptureAuth{
				ToolNameGlob:   body.Auth.Capture.ToolNameGlob,
				TokenJSONPaths: body.Auth.Capture.TokenJSONPaths,
				LabelJSONPaths: body.Auth.Capture.LabelJSONPaths,
				HeaderTemplate: body.Auth.Capture.HeaderTemplate,
				DefaultScheme:  body.Auth.Capture.DefaultScheme,
			},
		}
	}

	c, infos, err := connector.Apply(connector.ApplyInput{
		Store:                  s.Store,
		Registry:               s.Registry,
		Identities:             s.Identities,
		ID:                     id,
		Type:                   body.Type,
		Spec:                   body.Spec,
		BaseURL:                body.BaseURL,
		ExecutionCallbackURL:   body.ExecutionCallbackURL,
		RequireApproval:        body.RequireApproval,
		RequireLogin:           body.RequireLogin,
		Auth:                   connectorAuth,
		MCP:                    body.MCP,
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
		"id":                       c.ID,
		"type":                     c.Type,
		"spec":                     c.Spec,
		"base_url":                 c.BaseURL,
		"execution_callback_url":   c.ExecutionCallbackURL,
		"require_approval":         c.RequireApproval,
		"require_login":            c.RequireLogin,
		"auth":                     c.Auth,
		"tools":                    infos,
	}
	if c.Type == "mcp" {
		resp["mcp"] = c.MCP
	}
	writeJSON(w, http.StatusOK, resp)
}

// authBody mirrors store.ConnectorAuth / authcred.Config for JSON decoding of
// PUT /v0/connectors/{id} body.auth. Kept local to avoid pulling store types
// into the request body struct directly.
type authBody struct {
	Mode        string `json:"mode"`
	Static      struct {
		Headers map[string]string `json:"headers"`
	} `json:"static"`
	Passthrough struct {
		Headers []string `json:"headers"`
	} `json:"passthrough"`
	VaultRef struct {
		Headers map[string]string `json:"headers"`
	} `json:"vault_ref"`
	Capture struct {
		ToolNameGlob   string   `json:"tool_name_glob"`
		TokenJSONPaths []string `json:"token_json_paths"`
		LabelJSONPaths []string `json:"label_json_paths"`
		HeaderTemplate string   `json:"header_template"`
		DefaultScheme  string   `json:"default_scheme"`
	} `json:"capture"`
}

func (s *Server) handleGetConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	tools := s.Store.ListToolsByConnector(id)
	if tools == nil {
		tools = []store.Tool{}
	}
	resp := map[string]any{
		"id":                       c.ID,
		"type":                     c.Type,
		"spec":                     c.Spec,
		"base_url":                 c.BaseURL,
		"execution_callback_url":   c.ExecutionCallbackURL,
		"require_approval":         c.RequireApproval,
		"require_login":            c.RequireLogin,
		"auth":                     c.Auth,
		"tools":                    tools,
	}
	if c.Type == "mcp" {
		resp["mcp"] = c.MCP
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) loadEventsWebhook() webhook.Config {
	raw, ok, err := s.Store.GetSetting(store.SettingKeyEventsWebhook)
	if err != nil || !ok || len(raw) == 0 {
		return webhook.Config{}
	}
	var cfg webhook.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return webhook.Config{}
	}
	return cfg
}

func (s *Server) handleGetEventsWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := s.loadEventsWebhook()
	if cfg.Headers == nil {
		cfg.Headers = map[string]string{}
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutEventsWebhook(w http.ResponseWriter, r *http.Request) {
	var body webhook.Config
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Headers == nil {
		body.Headers = map[string]string{}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Store.UpsertSetting(store.SettingKeyEventsWebhook, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if s.Webhook != nil {
		s.Webhook.UpdateConfig(body)
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handlePostEventsWebhookTest(w http.ResponseWriter, r *http.Request) {
	if s.Webhook == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook_unavailable", "webhook dispatcher is not configured")
		return
	}
	if err := s.Webhook.SendTest(r.Context()); err != nil {
		switch {
		case errors.Is(err, webhook.ErrNoURL):
			writeError(w, http.StatusBadRequest, "webhook_not_configured", "webhook url is not configured")
		case errors.Is(err, authcred.ErrInvalidAuth):
			writeError(w, http.StatusBadRequest, "invalid_auth", err.Error())
		default:
			writeError(w, http.StatusBadGateway, "webhook_delivery_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
		Enabled      *bool   `json:"enabled"`
		RequireLogin *bool   `json:"require_login"`
		Title        *string `json:"title"`
		Description  *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Enabled == nil && body.RequireLogin == nil && body.Title == nil && body.Description == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "at least one of enabled, require_login, title, or description is required")
		return
	}

	row, err := s.Store.GetTool(name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "tool not found")
		return
	}

	// Track whether enabled actually changes. Only an enabled transition
	// (false→true) needs a full re-register to restore the invoker closure and
	// mutating HITL. Toggling only require_login on an already-enabled row is
	// served by Registry.SetRequireLogin so we don't drop the baked-in
	// RequireApproval flag (RegisterOneFromConnector does not know
	// RequireApprovalMutating and would otherwise lose mutating HITL).
	enabledChanged := body.Enabled != nil && *body.Enabled != row.Enabled

	if body.Enabled != nil {
		row.Enabled = *body.Enabled
	}
	if body.RequireLogin != nil {
		row.RequireLogin = *body.RequireLogin
	}
	if body.Title != nil {
		row.Title = *body.Title
	}
	if body.Description != nil {
		row.Description = *body.Description
		row.DescriptionCustom = true
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
	s.Store.UpsertConnector(c)

	writeJSON(w, http.StatusOK, row)
}

// registerOne re-registers a single enabled catalog row into the Registry by
// delegating to connector.RegisterOneFromConnector, which rebuilds the same
// invoker closure Apply uses. It does not write the Store; the caller is
// responsible for UpsertTool before calling.
func (s *Server) registerOne(c store.Connector, t store.Tool) error {
	return connector.RegisterOneFromConnector(s.Store, s.Registry, s.Identities, c, t)
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

func (s *Server) handlePostRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID        string            `json:"agent_id"`
		Input          string            `json:"input"`
		ConversationID string            `json:"conversation_id"`
		IdentityID     string            `json:"identity_id"`
		WebhookURL     string            `json:"webhook_url"`
		WebhookHeaders map[string]string `json:"webhook_headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if strings.TrimSpace(body.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}

	ag, err := s.Store.GetAgent(body.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}

	// Omitting conversation_id keeps the machine path (default connector
	// headers apply; require_login is not enforced). Chat always sends an id.
	conv := strings.TrimSpace(body.ConversationID)

	createIn := store.CreateRunInput{
		AgentID:        body.AgentID,
		Input:          body.Input,
		ConversationID: conv,
		IdentityID:     strings.TrimSpace(body.IdentityID),
	}
	// Only passthrough mode pulls headers off the inbound request into the
	// run's private PassthroughHeaders; other modes ignore the request headers
	// and rely on the connector's resolved DefaultHeaders.
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		createIn.PassthroughHeaders = authcred.PickHeaders(r.Header, s.AuthWhitelist)
	}
	if u := strings.TrimSpace(body.WebhookURL); u != "" || len(body.WebhookHeaders) > 0 {
		createIn.WebhookConfig = &store.WebhookConfig{
			URL:     u,
			Headers: body.WebhookHeaders,
		}
	}

	runRec, err := s.Store.CreateRun(createIn)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Persist the user turn before executing so the conversation window survives
	// crashes and is visible via GET /v0/conversations/{id}/messages. Execute's
	// buildMessages dedups the trailing user input against this appended entry.
	if s.Messages != nil && conv != "" {
		_, _ = s.Messages.Append(conv, conversation.Message{
			Role:    conversation.RoleUser,
			Content: body.Input,
			RunID:   runRec.ID,
		})
	}

	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	// Persist run.started before returning so the UI never polls an empty event stream
	// while the worker is still scheduling / contending on SQLite.
	_ = s.Store.AppendEvent(runRec.ID, store.Event{Type: run.EventRunStarted})
	go func(runID, input string, def agent.Def) {
		err := s.Runner.Execute(context.Background(), runID, def, input)
		if err == nil {
			return
		}
		cur, getErr := s.Store.GetRun(runID)
		if getErr != nil || cur == nil {
			return
		}
		switch cur.Status {
		case store.StatusRunning, store.StatusQueued:
			_ = s.Store.UpdateRun(runID, store.StatusFailed, "", err.Error())
			_ = s.Store.AppendEvent(runID, store.Event{
				Type: run.EventLLMError,
				Data: map[string]any{"error": err.Error()},
			})
			// Engine may have returned before recordTerminalMessage (e.g. early
			// failure while still running). Mirror its failed-note format here.
			if s.Messages != nil && cur.ConversationID != "" {
				note := strings.TrimSpace(err.Error())
				if note == "" {
					note = "运行失败"
				} else {
					note = "运行失败：" + note
				}
				_, _ = s.Messages.Append(cur.ConversationID, conversation.Message{
					Role:    conversation.RoleSystemNote,
					Content: note,
					RunID:   runID,
				})
			}
		}
	}(runRec.ID, body.Input, def)

	updated, err := s.Store.GetRun(runRec.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":          updated.ID,
		"status":          updated.Status,
		"conversation_id": conv,
	})
}

func (s *Server) handleListIdentities(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeJSON(w, http.StatusOK, []identity.PublicView{})
		return
	}
	views := s.Identities.ListPublic(r.PathValue("id"))
	if views == nil {
		views = []identity.PublicView{}
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleSetDefaultIdentity(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
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
	if err := s.Identities.Delete(r.PathValue("id"), r.PathValue("iid")); err != nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleClearIdentities(w http.ResponseWriter, r *http.Request) {
	if s.Identities != nil {
		s.Identities.ClearCaptured(r.PathValue("id"))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePostResume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if runRec.Status != store.StatusWaitingHuman {
		writeError(w, http.StatusConflict, "not_waiting", "run is not waiting_human")
		return
	}

	var body struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	approve := body.Decision == "approve"
	if body.Decision != "approve" && body.Decision != "reject" {
		writeError(w, http.StatusBadRequest, "invalid_request", "decision must be approve or reject")
		return
	}

	// In passthrough mode, refresh the run's passthrough headers from the
	// resume request before resuming execution. This must happen BEFORE
	// ContinueFromHITL so the engine's injectAuthCtxFromRun picks up the
	// fresh headers. An empty pick leaves the previously stored headers intact.
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		if picked := authcred.PickHeaders(r.Header, s.AuthWhitelist); len(picked) > 0 {
			if err := s.Store.SetPassthroughHeaders(id, picked); err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
		}
	}

	_ = s.Runner.ContinueFromHITL(r.Context(), id, run.Decision{
		Approve: approve,
		Comment: body.Comment,
	})

	updated, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": updated.ID,
		"status": updated.Status,
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	writeJSON(w, http.StatusOK, runRec)
}

func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	evs, err := s.Store.ListEvents(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if evs == nil {
		evs = []store.Event{}
	}
	writeJSON(w, http.StatusOK, evs)
}

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}

	after := parseStreamAfter(r)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	evs, err := s.Store.ListEvents(id)
	if err != nil {
		return
	}
	lastSent := after
	for i, ev := range evs {
		if i <= after {
			continue
		}
		if err := writeSSEEvent(w, rc, i, ev); err != nil {
			return
		}
		lastSent = i
	}

	terminal := runRec.Status == store.StatusSucceeded || runRec.Status == store.StatusFailed
	if terminal {
		_ = writeSSEEnded(w, rc, runRec.Status)
		return
	}
	if s.Hub == nil {
		// Replay-only: cannot subscribe for live increments.
		return
	}

	sub := s.Hub.Subscribe(id)
	defer sub.Cancel()

	// Fan-out before Subscribe is dropped; drain buffer then re-read store.
	var drainErr error
	lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
	if drainErr != nil {
		return
	}
	if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
		return
	} else if catchUpTerminal {
		_ = writeSSEEnded(w, rc, status)
		return
	}
	lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
	if drainErr != nil {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			if ev.Index <= lastSent {
				continue
			}
			if err := writeSSEEvent(w, rc, ev.Index, ev.Event); err != nil {
				return
			}
			lastSent = ev.Index
		case stt, ok := <-sub.Ended:
			if !ok {
				return
			}
			// Events and Ended may both be ready; drain events first so the
			// final AppendEvent is not lost when select picks Ended.
			lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
			if drainErr != nil {
				return
			}
			_ = writeSSEEnded(w, rc, stt)
			return
		}
	}
}

// drainSubEvents non-blocking writes any buffered subscription events with index > lastSent.
func drainSubEvents(w http.ResponseWriter, rc *http.ResponseController, sub *eventbus.Subscription, lastSent int) (int, error) {
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				return lastSent, nil
			}
			if ev.Index <= lastSent {
				continue
			}
			if err := writeSSEEvent(w, rc, ev.Index, ev.Event); err != nil {
				return lastSent, err
			}
			lastSent = ev.Index
		default:
			return lastSent, nil
		}
	}
}

// catchUpRunStream re-reads the store after Subscribe to recover events/status
// published in the ListEvents→Subscribe window (no subscriber yet).
func catchUpRunStream(w http.ResponseWriter, rc *http.ResponseController, st store.Store, id string, lastSent *int) (terminal bool, status store.Status, err error) {
	runRec, err := st.GetRun(id)
	if err != nil {
		return false, "", err
	}
	evs, err := st.ListEvents(id)
	if err != nil {
		return false, "", err
	}
	for i, ev := range evs {
		if i <= *lastSent {
			continue
		}
		if err := writeSSEEvent(w, rc, i, ev); err != nil {
			return false, "", err
		}
		*lastSent = i
	}
	if runRec.Status == store.StatusSucceeded || runRec.Status == store.StatusFailed {
		return true, runRec.Status, nil
	}
	return false, "", nil
}

func parseStreamAfter(r *http.Request) int {
	after := -1
	if q := r.URL.Query().Get("after"); q != "" {
		if n, err := strconv.Atoi(q); err == nil {
			after = n
		} else {
			after = -1
		}
	}
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		if n, err := strconv.Atoi(id); err == nil {
			after = n
		} else {
			after = -1
		}
	}
	return after
}

func writeSSEEvent(w http.ResponseWriter, rc *http.ResponseController, index int, ev store.Event) error {
	payload, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "id: %d\ndata: %s\n\n", index, payload); err != nil {
		return err
	}
	return rc.Flush()
}

func writeSSEEnded(w http.ResponseWriter, rc *http.ResponseController, status store.Status) error {
	payload, err := json.Marshal(map[string]string{"status": string(status)})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: run.ended\ndata: %s\n\n", payload); err != nil {
		return err
	}
	return rc.Flush()
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	out := []conversation.Summary{}
	if s.Messages != nil {
		if sum := s.Messages.ListSummaries(); sum != nil {
			out = sum
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	if s.Messages == nil {
		writeJSON(w, http.StatusOK, []conversation.Message{})
		return
	}
	msgs := s.Messages.List(r.PathValue("id"))
	if msgs == nil {
		msgs = []conversation.Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleClearMessages(w http.ResponseWriter, r *http.Request) {
	if s.Messages != nil {
		s.Messages.Clear(r.PathValue("id"))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
