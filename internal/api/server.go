package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/attach"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channel/weixin"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/connector/specimport"
	"github.com/rebornace/baize/internal/connector/specstore"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/skillparse"
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

// RunWithOptions is implemented by runners that accept per-run overrides
// (skills override + multimodal user parts). When the runner does not
// implement it, the server falls back to plain Execute (no skills override,
// no image parts). run.Engine implements this; test fakes need not.
type RunWithOptions interface {
	ExecuteWithOpts(ctx context.Context, runID string, ag agent.Def, input string, opts run.RunOptions) error
}

// RunCanceller is implemented by run.Engine for cooperative cancel.
type RunCanceller interface {
	Cancel(runID string) error
}

type Server struct {
	Store          store.Store
	Registry       *tool.Registry
	Artifacts      artifact.Store // optional; nil = artifact routes unavailable
	Runner         Runner
	SkillCatalog   *skill.Catalog
	Identities     identity.Store
	Messages       conversation.Store // optional; nil = no message persistence
	Hub            *eventbus.Hub      // optional; nil = SSE replay only (no live fan-out)
	DefaultAgentID string
	// Queue optionally dispatches runs to competing workers. nil = in-process
	// goroutine execution (legacy single-instance behavior).
	Queue middleware.JobQueue
	// LeaseTTL is the worker lease duration for executed runs (default 60s).
	LeaseTTL time.Duration
	// AuthMode is the active connector's normalized auth mode. Only "passthrough"
	// changes POST /runs and POST /runs/{id}/resume behavior to pick headers.
	AuthMode string
	// AuthWhitelist is the passthrough header whitelist. nil → default
	// ["Authorization"]; len==0 → no headers. Only used when AuthMode=="passthrough".
	AuthWhitelist []string
	// OperatorToken / AdminToken configure the control-plane gate. When both
	// are empty the gate is off and all routes behave as before. When at least
	// one is set, /v0 routes require a Bearer token whose role meets MinRole.
	OperatorToken    string
	AdminToken       string
	Operators        []controlplane.Operator
	Webhook          *webhook.Dispatcher // optional; nil = no outbound webhook delivery
	Inbox            *inbox.Registry     // optional; nil = inbox routes unavailable
	InboxLimiter     *inbox.RateLimiter  // optional; nil => lazy default via inboxLimiter()
	inboxLimiterOnce sync.Once
	// InboxGate 可选；非 nil 时 inbox 入站限流走它（key=channelID）。nil 时回退 InboxLimiter。
	InboxGate func(channelID string) bool
	// MCPExportEnabled gates /v0/mcp/export (default true when set by bootstrap).
	MCPExportEnabled bool
	DataDir          string // parent dir for specstore (sqlite dir); required for spec_content PUT
	ConfigPath       string
	Config           *config.Config
	Shutdown         func(context.Context) error
	RestartProcess   func() error
	// LLM is the active provider, used to report supports_vision via ui-config
	// and to gate image attachments before a run is created. nil = no vision
	// (image attachments are rejected with vision_unsupported).
	LLM llm.Provider
	// CallbackSecret is the HMAC key used to verify sidecar plugin callback
	// tokens. Set by bootstrap; empty => handlePluginCallback rejects every
	// request with 401 (no token can verify).
	CallbackSecret []byte
	// CallbackLimiter caps per-run callback throughput. nil => no limiting
	// (bootstrap always sets one).
	CallbackLimiter *plugincallback.Limiter
	// CallbackGate 可选；非 nil 时 sidecar callback 限流走它（key=runID）。nil 时回退 CallbackLimiter。
	CallbackGate func(runID string) bool
	// CallbackSigner / CallbackPublicBase / CallbackTTL configure
	// callback_urls.event injection into sidecar invoke context. When any
	// piece is missing the URL is omitted (fail-open). Set by bootstrap.
	CallbackSigner     httpplugin.CallbackSigner
	CallbackPublicBase string
	CallbackTTL        time.Duration

	// Weixin channel settings API (login / logout / settings). Optional;
	// nil ILink means routes return 503. Set by bootstrap or tests (Fake).
	WeixinILink    weixin.ILink
	WeixinChannel  *weixin.Channel
	WeixinRuntime  *channel.Runtime
	WeixinCredsDir string          // default ./data/channels/weixin
	WeixinRunCtx   context.Context // long-lived ctx for Channel.Start; nil => Background
	weixinMu       sync.Mutex

	mux *http.ServeMux
}

func NewServer(st store.Store, reg *tool.Registry, runner Runner) *Server {
	s := &Server{
		Store:            st,
		Registry:         reg,
		Runner:           runner,
		Identities:       identity.NewMemoryStore(),
		MCPExportEnabled: true,
		mux:              http.NewServeMux(),
	}
	s.routes()
	return s
}

// inboxLimiter returns the configured limiter, lazily creating a default once
// when InboxLimiter was left nil (bootstrap normally sets one).
func (s *Server) inboxLimiter() *inbox.RateLimiter {
	s.inboxLimiterOnce.Do(func() {
		if s.InboxLimiter == nil {
			s.InboxLimiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)
		}
	})
	return s.InboxLimiter
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
	tok := s.gateTokens()
	if !tok.Enabled() {
		return r, true
	}
	min := controlplane.MinRole(r.Method, r.URL.Path)
	if min == controlplane.RoleNone {
		return r, true
	}
	principal, ok := controlplane.AuthenticatePrincipal(r.Header.Get("Authorization"), tok)
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "需要控制面口令")
		return nil, false
	}
	if !principal.Role.AtLeast(min) {
		writeError(w, http.StatusForbidden, "forbidden", "需要管理员口令")
		return nil, false
	}
	// 控制面口令只用于过门禁。门开着且鉴权成功后，从交给 mux 的 request 上
	// 删掉 Authorization，避免 passthrough 把它 PickHeaders 进 Run 的
	// passthrough_json（SQLite），进而被机器路径打到下游 API。门关着时
	// authorize 在上面已提前 return，不会执行到这里，故不影响现有 passthrough。
	r.Header.Del("Authorization")
	ctx := controlplane.WithRole(r.Context(), principal.Role)
	if principal.OperatorID != "" {
		ctx = controlplane.WithOperatorID(ctx, principal.OperatorID)
	}
	return r.WithContext(ctx), true
}

func (s *Server) gateTokens() controlplane.Tokens {
	return controlplane.Tokens{
		Operator:  s.OperatorToken,
		Admin:     s.AdminToken,
		Operators: s.Operators,
	}
}

func (s *Server) metaStore() conversation.MetaStore {
	if ms, ok := s.Messages.(conversation.MetaStore); ok {
		return ms
	}
	return nil
}

func (s *Server) principalFrom(ctx context.Context) controlplane.Principal {
	return controlplane.Principal{
		Role:       controlplane.RoleFrom(ctx),
		OperatorID: controlplane.OperatorIDFrom(ctx),
	}
}

var errConversationForbidden = errors.New("无权访问该会话")

// requireConversationAccess enforces owner checks when the gate is on.
// Missing meta is allowed for admin (legacy rows) and denied for operators.
// Store/DB errors surface as HTTP 500.
func (s *Server) requireConversationAccess(w http.ResponseWriter, r *http.Request, convID string) bool {
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return false
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return false
	}
	return true
}

func (s *Server) checkConversationAccess(ctx context.Context, convID string) error {
	if !s.gateTokens().Enabled() || convID == "" {
		return nil
	}
	ms := s.metaStore()
	if ms == nil {
		return nil
	}
	p := s.principalFrom(ctx)
	meta, err := ms.GetMeta(convID)
	if errors.Is(err, conversation.ErrMetaNotFound) {
		if p.Role == controlplane.RoleAdmin {
			return nil
		}
		return errConversationForbidden
	}
	if err != nil {
		return err
	}
	if !conversation.CanAccess(p, meta) {
		return errConversationForbidden
	}
	return nil
}

// ensureConversationMeta creates ownership on first UI use, or checks access if meta exists.
// Returns false after writing an error response.
func (s *Server) ensureConversationMeta(w http.ResponseWriter, r *http.Request, convID string) bool {
	if err := s.prepareConversationMeta(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return false
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return false
	}
	return true
}

func (s *Server) prepareConversationMeta(ctx context.Context, convID string) error {
	if convID == "" {
		return nil
	}
	ms := s.metaStore()
	if ms == nil {
		return nil
	}
	meta, err := ms.GetMeta(convID)
	if err == nil {
		if !s.gateTokens().Enabled() {
			return nil
		}
		if !conversation.CanAccess(s.principalFrom(ctx), meta) {
			return errConversationForbidden
		}
		return nil
	}
	if !errors.Is(err, conversation.ErrMetaNotFound) {
		return err
	}
	owner := "local-dev"
	if s.gateTokens().Enabled() {
		if id := controlplane.OperatorIDFrom(ctx); id != "" {
			owner = id
		} else if controlplane.RoleFrom(ctx) == controlplane.RoleAdmin {
			owner = "admin"
		}
	}
	return ms.EnsureMeta(conversation.Meta{
		ID:        convID,
		OwnerID:   owner,
		Source:    "ui",
		UpdatedAt: time.Now().UTC(),
	})
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
	s.mux.HandleFunc("DELETE /v0/connectors/{id}", s.handleDeleteConnector)
	s.mux.HandleFunc("GET /v0/skills", s.handleListSkills)
	s.mux.HandleFunc("GET /v0/skills/{id}", s.handleGetSkill)
	s.mux.HandleFunc("POST /v0/skills", s.handlePostSkill)
	s.mux.HandleFunc("DELETE /v0/skills/{id}", s.handleDeleteSkill)
	s.mux.HandleFunc("POST /v0/runs", s.handlePostRun)
	s.mux.HandleFunc("POST /v0/inbox/{channel_id}", s.handlePostInbox)
	s.mux.HandleFunc("POST /v0/runs/{id}/resume", s.handlePostResume)
	s.mux.HandleFunc("POST /v0/runs/{id}/cancel", s.handlePostCancel)
	s.mux.HandleFunc("POST /v0/runs/{id}/plugin-callbacks", s.handlePluginCallback)
	s.mux.HandleFunc("GET /v0/runs/{id}/events", s.handleGetEvents)
	s.mux.HandleFunc("GET /v0/runs/{id}/stream", s.handleRunStream)
	s.mux.HandleFunc("GET /v0/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("GET /v0/artifacts/{id}", s.handleGetArtifact)
	s.mux.HandleFunc("GET /v0/conversations/{id}/identities", s.handleListIdentities)
	s.mux.HandleFunc("POST /v0/conversations/{id}/identities", s.handlePostIdentity)
	s.mux.HandleFunc("POST /v0/conversations/{id}/identities/{iid}/default", s.handleSetDefaultIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities/{iid}", s.handleDeleteIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities", s.handleClearIdentities)
	s.mux.HandleFunc("GET /v0/conversations", s.handleListConversations)
	s.mux.HandleFunc("GET /v0/conversations/{id}/messages", s.handleListMessages)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/messages", s.handleClearMessages)
	s.mux.HandleFunc("POST /v0/conversations/{id}/messages/{message_id}/rollback", s.handleRollbackMessages)
	s.mux.HandleFunc("POST /v0/conversations/{id}/fork", s.handleForkConversation)
	s.mux.HandleFunc("GET /v0/settings/events-webhook", s.handleGetEventsWebhook)
	s.mux.HandleFunc("PUT /v0/settings/events-webhook", s.handlePutEventsWebhook)
	s.mux.HandleFunc("POST /v0/settings/events-webhook/test", s.handlePostEventsWebhookTest)
	s.mux.HandleFunc("GET /v0/settings/events-webhook/deliveries", s.handleGetEventsWebhookDeliveries)
	s.mux.HandleFunc("POST /v0/settings/events-webhook/deliveries/{id}/retry", s.handlePostEventsWebhookDeliveryRetry)
	s.mux.HandleFunc("GET /v0/settings/inbox-channels", s.handleGetInboxChannels)
	s.mux.HandleFunc("PUT /v0/settings/inbox-channels", s.handlePutInboxChannels)
	s.mux.HandleFunc("POST /v0/settings/inbox-channels/{id}/rotate-secret", s.handlePostInboxRotateSecret)
	s.mux.HandleFunc("POST /v0/settings/inbox-channels/{id}/test", s.handlePostInboxTest)
	s.mux.HandleFunc("GET /v0/settings/store", s.handleGetStoreSettings)
	s.mux.HandleFunc("PUT /v0/settings/store", s.handlePutStoreSettings)
	s.mux.HandleFunc("POST /v0/settings/store/restart", s.handlePostStoreRestart)
	s.mux.HandleFunc("POST /v0/settings/channels/weixin/login/start", s.handleWeixinLoginStart)
	s.mux.HandleFunc("GET /v0/settings/channels/weixin/login/status", s.handleWeixinLoginStatus)
	s.mux.HandleFunc("POST /v0/settings/channels/weixin/logout", s.handleWeixinLogout)
	s.mux.HandleFunc("GET /v0/settings/channels/weixin", s.handleGetWeixinSettings)
	s.mux.HandleFunc("PUT /v0/settings/channels/weixin", s.handlePutWeixinSettings)

	s.mux.Handle("/v0/mcp/export", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mcpExportHTTP().ServeHTTP(w, r)
	}))
	s.mux.Handle("/v0/mcp/export/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/v0/mcp/export", s.mcpExportHTTP()).ServeHTTP(w, r)
	}))
	s.mux.HandleFunc("GET /v0/settings/mcp-export", s.handleGetMCPExportSettings)
	s.mux.HandleFunc("GET /v0/settings/mcp-export/identities", s.handleListMCPExportIdentities)
	s.mux.HandleFunc("POST /v0/settings/mcp-export/identities", s.handlePostMCPExportIdentity)
	s.mux.HandleFunc("GET /v0/settings/mcp-export/identities/{id}", s.handleGetMCPExportIdentity)
	s.mux.HandleFunc("PATCH /v0/settings/mcp-export/identities/{id}", s.handlePatchMCPExportIdentity)
	s.mux.HandleFunc("DELETE /v0/settings/mcp-export/identities/{id}", s.handleDeleteMCPExportIdentity)
	s.mux.HandleFunc("GET /v0/settings/mcp-export/keys", s.handleListMCPExportKeys)
	s.mux.HandleFunc("POST /v0/settings/mcp-export/keys", s.handlePostMCPExportKey)
	s.mux.HandleFunc("DELETE /v0/settings/mcp-export/keys/{id}", s.handleDeleteMCPExportKey)
	s.mux.HandleFunc("GET /v0/settings/models", s.handleListModelProfiles)
	s.mux.HandleFunc("POST /v0/settings/models", s.handlePostModelProfile)
	s.mux.HandleFunc("PATCH /v0/settings/models/{id}", s.handlePatchModelProfile)
	s.mux.HandleFunc("DELETE /v0/settings/models/{id}", s.handleDeleteModelProfile)
	s.mux.HandleFunc("POST /v0/settings/models/{id}/default", s.handleSetDefaultModelProfile)
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
	AgentID        string `json:"agent_id"`
	GateEnabled    bool   `json:"gate_enabled"`
	SupportsVision bool   `json:"supports_vision"`
}

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, uiConfig{
		AgentID:        s.DefaultAgentID,
		GateEnabled:    s.gateTokens().Enabled(),
		SupportsVision: s.supportsVision(),
	})
}

// supportsVision reports whether the active LLM provider can accept image
// parts. When no provider is configured, vision is unavailable so image
// attachments are rejected up-front rather than silently dropped.
func (s *Server) supportsVision() bool {
	if s.LLM == nil {
		return false
	}
	return s.LLM.SupportsVision()
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"role":         string(controlplane.RoleFrom(r.Context())),
		"operator_id":  controlplane.OperatorIDFrom(r.Context()),
		"gate_enabled": s.gateTokens().Enabled(),
	})
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
		Type                 string          `json:"type"`
		Spec                 string          `json:"spec"`
		SpecContent          string          `json:"spec_content"`
		SpecURL              string          `json:"spec_url"`
		ImportFormat         string          `json:"import_format"`
		BaseURL              string          `json:"base_url"`
		ExecutionCallbackURL string          `json:"execution_callback_url"`
		RequireApproval      []string        `json:"require_approval"`
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
		if strings.TrimSpace(s.DataDir) == "" {
			writeError(w, http.StatusInternalServerError, "internal_error", "data directory is not configured")
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
		relPath, err := specstore.Write(s.DataDir, id, content, normalized)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		specPath = filepath.Join(s.DataDir, relPath)
		importFormatDetected = detected
	} else if body.Type == "openapi" && specPath == "" {
		existing, err := s.Store.GetConnector(id)
		if err != nil || strings.TrimSpace(existing.Spec) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "spec is required")
			return
		}
		if _, err := os.Stat(existing.Spec); err != nil {
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
		Store:                s.Store,
		Registry:             s.Registry,
		Identities:           s.Identities,
		ID:                   id,
		Type:                 body.Type,
		Spec:                 specPath,
		ImportFormat:         importFormatDetected,
		BaseURL:              body.BaseURL,
		ExecutionCallbackURL: body.ExecutionCallbackURL,
		RequireApproval:      body.RequireApproval,
		RequireLogin:         body.RequireLogin,
		Auth:                 connectorAuth,
		MCP:                  body.MCP,
		CallbackSigner:       s.CallbackSigner,
		CallbackSecret:       s.CallbackSecret,
		CallbackPublicBase:   s.CallbackPublicBase,
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
		"require_approval":       c.RequireApproval,
		"require_login":          c.RequireLogin,
		"auth":                   c.Auth,
		"tools":                  infos,
	}
	if c.Type == "mcp" {
		resp["mcp"] = c.MCP
	}
	writeJSON(w, http.StatusOK, resp)
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
		"id":                     c.ID,
		"type":                   c.Type,
		"spec":                   c.Spec,
		"import_format_detected": c.ImportFormat,
		"base_url":               c.BaseURL,
		"execution_callback_url": c.ExecutionCallbackURL,
		"require_approval":       c.RequireApproval,
		"require_login":          c.RequireLogin,
		"auth":                   c.Auth,
		"tools":                  tools,
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

func (s *Server) handleGetEventsWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	statusParam := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	var statuses []store.WebhookOutboxStatus
	if statusParam == "" {
		statuses = []store.WebhookOutboxStatus{store.WebhookOutboxDead, store.WebhookOutboxPending}
	} else {
		for _, part := range strings.Split(statusParam, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			statuses = append(statuses, store.WebhookOutboxStatus(part))
		}
	}
	entries, err := s.Store.ListWebhookOutbox(statuses, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	type row struct {
		ID          string `json:"id"`
		RunID       string `json:"run_id"`
		Kind        string `json:"kind"`
		EventIndex  int    `json:"event_index"`
		Status      string `json:"status"`
		Attempt     int    `json:"attempt"`
		MaxAttempts int    `json:"max_attempts"`
		LastError   string `json:"last_error,omitempty"`
		NextRetryAt string `json:"next_retry_at"`
		CreatedAt   string `json:"created_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		out = append(out, row{
			ID:          e.ID,
			RunID:       e.RunID,
			Kind:        string(e.Kind),
			EventIndex:  e.EventIndex,
			Status:      string(e.Status),
			Attempt:     e.Attempt,
			MaxAttempts: e.MaxAttempts,
			LastError:   e.LastError,
			NextRetryAt: e.NextRetryAt.UTC().Format(time.RFC3339Nano),
			CreatedAt:   e.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:   e.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

func (s *Server) handlePostEventsWebhookDeliveryRetry(w http.ResponseWriter, r *http.Request) {
	if s.Webhook == nil {
		writeError(w, http.StatusServiceUnavailable, "webhook_unavailable", "webhook dispatcher is not configured")
		return
	}
	id := r.PathValue("id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing delivery id")
		return
	}
	if err := s.Webhook.RetryDelivery(id); err != nil {
		if errors.Is(err, store.ErrWebhookOutboxNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "delivery not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
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
		Export       *string `json:"export"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Enabled == nil && body.RequireLogin == nil && body.Title == nil && body.Description == nil && body.Export == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "at least one of enabled, require_login, title, description, or export is required")
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
	// mutating HITL. Toggling only require_login on an already-enabled row is
	// served by Registry.SetRequireLogin so we don't drop the baked-in
	// RequireApproval flag (RegisterOneFromConnector does not know
	// RequireApprovalMutating and would otherwise lose mutating HITL).
	// Export-only patches update the store catalog and never re-register.
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
	return connector.RegisterOneFromConnector(s.Store, s.Registry, s.Identities, c, t, connector.CallbackConfig{
		Signer:     s.CallbackSigner,
		Secret:     s.CallbackSecret,
		PublicBase: s.CallbackPublicBase,
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
	s.Registry.UnregisterConnector(id)
	if err := s.Store.DeleteConnector(id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
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
		AgentID        string                `json:"agent_id"`
		Input          string                `json:"input"`
		ConversationID string                `json:"conversation_id"`
		IdentityID     string                `json:"identity_id"`
		SessionToken   string                `json:"session_token"`
		WebhookURL     string                `json:"webhook_url"`
		WebhookHeaders map[string]string     `json:"webhook_headers"`
		Skills         []string              `json:"skills"`
		Attachments    []attach.AttachmentIn `json:"attachments"`
		ModelProfileID string                `json:"model_profile_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if strings.TrimSpace(body.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}

	if _, err := s.Store.GetAgent(body.AgentID); err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}

	// Omitting conversation_id keeps the machine path (default connector
	// headers apply; require_login is not enforced). Chat always sends an id.
	conv := strings.TrimSpace(body.ConversationID)
	identityID := strings.TrimSpace(body.IdentityID)
	runInput := body.Input
	if conv != "" && s.Identities != nil {
		cleaned, sessionID, err := identity.PrepareSessionAuth(s.Identities, conv, runInput, body.SessionToken)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		runInput = cleaned
		if identityID == "" && sessionID != "" {
			identityID = sessionID
		}
	}

	// Attachments: extract text + images up-front so size/type/vision failures
	// reject the request before any run is created. Image bytes never enter
	// SQLite; only filenames surface in the persisted user bubble.
	textExts, imageExts, err := attach.Process(body.Attachments, attach.DefaultOptions())
	if err != nil {
		s.writeAttachmentError(w, err)
		return
	}
	if len(imageExts) > 0 && !s.supportsVision() {
		writeError(w, http.StatusBadRequest, "vision_unsupported",
			"model does not support vision; remove image attachments or switch to a vision-capable model")
		return
	}

	// Skill mentions (@id / /id) are stripped from the user text and merged
	// with body.skills. Omitting skills with no mentions keeps agent defaults;
	// any explicit input (non-nil body.skills or a mention) overrides for this
	// run, with an empty merged set meaning "no default skill active".
	cleanedInput, mentionIDs := skillparse.Parse(runInput)
	var runSkills []string
	if body.Skills != nil || len(mentionIDs) > 0 {
		merged := mergeSkillIDs(mentionIDs, body.Skills)
		if s.SkillCatalog != nil {
			var unknown []string
			for _, id := range merged {
				if _, ok := s.SkillCatalog.Get(id); !ok {
					unknown = append(unknown, id)
				}
			}
			if len(unknown) > 0 {
				writeError(w, http.StatusBadRequest, "unknown_skill",
					"unknown skill id: "+strings.Join(unknown, ", "))
				return
			}
		}
		runSkills = merged
	}

	// Build the LLM-bound user content (cleaned text + text-attachment blocks)
	// and the multimodal Parts payload when any attachments are present. The
	// persisted bubble stores only the visible text + filenames (no base64).
	llmText := cleanedInput
	var userParts []llm.ContentPart
	if len(textExts) > 0 || len(imageExts) > 0 {
		var b strings.Builder
		b.WriteString(cleanedInput)
		for _, t := range textExts {
			b.WriteString("\n\n【附件: ")
			b.WriteString(t.Filename)
			b.WriteString("】\n")
			b.WriteString(t.Text)
		}
		llmText = b.String()
		userParts = append(userParts, llm.ContentPart{Type: "text", Text: llmText})
		for _, img := range imageExts {
			userParts = append(userParts, llm.ContentPart{
				Type:       "image",
				ImageMIME:  img.ImageMIME,
				ImageBytes: img.ImageBytes,
			})
		}
	}
	displayText := cleanedInput
	if n := len(textExts) + len(imageExts); n > 0 {
		names := make([]string, 0, n)
		for _, e := range textExts {
			names = append(names, e.Filename)
		}
		for _, e := range imageExts {
			names = append(names, e.Filename)
		}
		displayText = strings.TrimSpace(cleanedInput) + "（附件：" + strings.Join(names, ", ") + "）"
	}

	var passthrough map[string]string
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		passthrough = authcred.PickHeaders(r.Header, s.AuthWhitelist)
	}
	var webhookCfg *store.WebhookConfig
	if u := strings.TrimSpace(body.WebhookURL); u != "" || len(body.WebhookHeaders) > 0 {
		webhookCfg = &store.WebhookConfig{
			URL:     u,
			Headers: body.WebhookHeaders,
		}
	}

	updated, err := s.startRun(r.Context(), startRunInput{
		AgentID:        body.AgentID,
		Input:          displayText,
		ConversationID: conv,
		IdentityID:     identityID,
		Skills:         runSkills,
		Webhook:        webhookCfg,
		Passthrough:    passthrough,
		UserParts:      userParts,
		ModelProfileID: strings.TrimSpace(body.ModelProfileID),
	})
	if err != nil {
		if err.Error() == "无权访问该会话" {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":          updated.ID,
		"status":          updated.Status,
		"conversation_id": conv,
	})
}

// runExecute dispatches to a RunWithOptions runner when available so per-run
// skills and multimodal user parts are honored; otherwise it falls back to the
// plain Execute path. Test fakes that only implement Runner still work.
func (s *Server) runExecute(ctx context.Context, runID string, def agent.Def, input string, opts run.RunOptions) error {
	if ex, ok := s.Runner.(RunWithOptions); ok {
		return ex.ExecuteWithOpts(ctx, runID, def, input, opts)
	}
	return s.Runner.Execute(ctx, runID, def, input)
}

// ExecuteJob runs a queued run under a worker lease with idempotency gating.
// It implements middleware.Executor: queue workers and the local fallback
// goroutine both funnel through it. Terminal or HITL-waiting runs are
// ack-skipped (resume is a separate entry point); a live lease held by
// another worker is also ack-skipped.
func (s *Server) ExecuteJob(ctx context.Context, job middleware.Job) error {
	if job.RunID == "" {
		return nil
	}
	cur, err := s.Store.GetRun(job.RunID)
	if err != nil || cur == nil {
		return err
	}
	switch cur.Status {
	case store.StatusSucceeded, store.StatusFailed, store.StatusCancelled, store.StatusWaitingHuman:
		return nil
	}

	ttl := s.LeaseTTL
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	acquired, err := s.Store.LeaseRun(job.RunID, ttl)
	if err != nil {
		return err
	}
	if !acquired {
		return nil // another worker holds the lease
	}
	defer func() { _ = s.Store.ClearRunLease(job.RunID) }()

	hbCtx, stopHB := context.WithCancel(ctx)
	defer stopHB()
	go s.leaseHeartbeat(hbCtx, job.RunID, ttl)

	def, input, opts, err := s.resolveJob(job, cur)
	if err != nil {
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return nil
	}
	if err := s.runExecute(ctx, job.RunID, def, input, opts); err != nil {
		s.finalizeRunError(job.RunID, cur.ConversationID, err)
		return err
	}
	return nil
}

// leaseHeartbeat renews the worker lease roughly every ttl/3 until ctx ends.
func (s *Server) leaseHeartbeat(ctx context.Context, runID string, ttl time.Duration) {
	t := time.NewTicker(ttl / 3)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.Store.HeartbeatRun(runID, ttl)
		}
	}
}

// resolveJob rebuilds the agent def, input, and run options for a queued job,
// falling back to the persisted run row when the job omits a field.
func (s *Server) resolveJob(job middleware.Job, cur *store.Run) (agent.Def, string, run.RunOptions, error) {
	agentID := strings.TrimSpace(job.AgentID)
	if agentID == "" {
		agentID = cur.AgentID
	}
	ag, err := s.Store.GetAgent(agentID)
	if err != nil {
		return agent.Def{}, "", run.RunOptions{}, err
	}
	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	input := job.Input
	if strings.TrimSpace(input) == "" {
		input = cur.Input
	}
	opts := run.RunOptions{Skills: job.Skills, UserParts: partsFromMiddleware(job.UserParts)}
	return def, input, opts, nil
}

// finalizeRunError mirrors the legacy background-goroutine failure handling:
// re-read the run, and only when it is still queued/running mark it failed,
// append an LLM error event, and leave a system note in the conversation.
func (s *Server) finalizeRunError(runID, convID string, runErr error) {
	cur, _ := s.Store.GetRun(runID)
	if cur == nil {
		return
	}
	if cur.Status != store.StatusRunning && cur.Status != store.StatusQueued {
		return
	}
	_ = s.Store.UpdateRun(runID, store.StatusFailed, "", runErr.Error())
	_ = s.Store.AppendEvent(runID, store.Event{
		Type: run.EventLLMError,
		Data: map[string]any{"error": runErr.Error()},
	})
	if s.Messages != nil && convID != "" {
		note := strings.TrimSpace(runErr.Error())
		if note == "" {
			note = "运行失败"
		} else {
			note = "运行失败：" + note
		}
		_, _ = s.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleSystemNote,
			Content: note,
			RunID:   runID,
		})
	}
}

// Dispatch enqueues a job when a Queue is configured, falling back to a local
// goroutine (legacy behavior) on nil queue or enqueue failure.
func (s *Server) Dispatch(ctx context.Context, job middleware.Job) {
	if job.EnqueuedAt.IsZero() {
		job.EnqueuedAt = time.Now().UTC()
	}
	if s.Queue != nil {
		if err := s.Queue.Enqueue(ctx, job); err == nil {
			return
		}
	}
	go func() { _ = s.ExecuteJob(context.Background(), job) }()
}

// writeAttachmentError maps an attach.Process error to the spec's API error
// codes. Unsupported MIME, size/count limits, and empty-PDF each get a distinct
// code; everything else (e.g. bad base64) is reported as invalid_attachment.
func (s *Server) writeAttachmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attach.ErrUnsupported):
		writeError(w, http.StatusBadRequest, "unsupported_attachment", err.Error())
	case errors.Is(err, attach.ErrTooLarge):
		writeError(w, http.StatusBadRequest, "attachment_too_large", err.Error())
	case errors.Is(err, attach.ErrTooMany):
		writeError(w, http.StatusBadRequest, "too_many_attachments", err.Error())
	case errors.Is(err, attach.ErrEmptyPDFText):
		writeError(w, http.StatusBadRequest, "empty_pdf_text", err.Error())
	default:
		writeError(w, http.StatusBadRequest, "invalid_attachment", err.Error())
	}
}

// mergeSkillIDs concatenates mention IDs and body.skills, dropping empties and
// preserving first-seen order. It always returns a non-nil slice (empty when
// both inputs are empty) so callers can distinguish "explicit override with an
// empty set" from "omit / use agent defaults".
func mergeSkillIDs(mentionIDs, bodySkills []string) []string {
	seen := make(map[string]struct{}, len(mentionIDs)+len(bodySkills))
	out := make([]string, 0, len(mentionIDs)+len(bodySkills))
	for _, id := range mentionIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range bodySkills {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

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

func (s *Server) handlePostCancel(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing run id")
		return
	}
	runRec, err := s.Store.GetRun(id)
	if err != nil || runRec == nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	switch runRec.Status {
	case store.StatusQueued, store.StatusRunning, store.StatusWaitingHuman:
	default:
		writeError(w, http.StatusConflict, "not_active", "run is not active")
		return
	}
	canceller, ok := s.Runner.(RunCanceller)
	if !ok {
		writeError(w, http.StatusNotImplemented, "cancel_unavailable", "runner does not support cancel")
		return
	}
	if err := canceller.Cancel(id); err != nil {
		writeError(w, http.StatusConflict, "cancel_failed", err.Error())
		return
	}
	updated, err := s.Store.GetRun(id)
	if err != nil || updated == nil {
		writeJSON(w, http.StatusOK, map[string]string{"run_id": id, "status": string(store.StatusCancelled)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"run_id": updated.ID,
		"status": string(updated.Status),
	})
}

func (s *Server) handlePostResume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
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
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
		return
	}
	writeJSON(w, http.StatusOK, runRec)
}

// maxPluginCallbackPayloadBytes is the upper bound on the serialized payload
// field of a plugin callback body (spec §3.4: 64KiB). The whole body is
// bounded by http.MaxBytesReader; the payload itself is checked after decode.
const maxPluginCallbackPayloadBytes = 64 * 1024

// handlePluginCallback receives a sidecar plugin event for a specific Run.
// Auth is by HMAC token only (ACL: RoleNone); the control-plane gate never
// applies. Flow: verify token → run exists → rate limiter → decode → size
// check → append event → 204.
func (s *Server) handlePluginCallback(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("id")
	if runID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing run id")
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" || len(s.CallbackSecret) == 0 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing callback token")
		return
	}
	if err := plugincallback.Verify(s.CallbackSecret, runID, token, time.Now()); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid callback token")
		return
	}
	if _, err := s.Store.GetRun(runID); err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if s.CallbackGate != nil {
		if !s.CallbackGate(runID) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "callback budget exhausted for this run")
			return
		}
	} else if s.CallbackLimiter != nil && !s.CallbackLimiter.Allow(runID, time.Now()) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "callback budget exhausted for this run")
		return
	}

	var body struct {
		Type    string         `json:"type"`
		Name    string         `json:"name"`
		Payload map[string]any `json:"payload"`
	}
	// Cap the whole request body at 1MiB so a hostile sidecar cannot stream
	// gigabytes into the decoder; the payload field is size-checked after.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing name")
		return
	}
	var payloadBytes int
	if body.Payload != nil {
		raw, err := json.Marshal(body.Payload)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "payload not serializable")
			return
		}
		payloadBytes = len(raw)
	}
	if payloadBytes > maxPluginCallbackPayloadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "payload exceeds 64KiB")
		return
	}

	// Event.Type is the fixed classification; the sidecar's body.type is the
	// event's own subtype, preserved as data.type for downstream consumers.
	data := map[string]any{
		"name":    body.Name,
		"payload": body.Payload,
	}
	if t := strings.TrimSpace(body.Type); t != "" {
		data["type"] = t
	}
	if err := s.Store.AppendEvent(runID, store.Event{
		Type: "plugin.callback",
		Data: data,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "append event failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	if s.Artifacts == nil {
		writeError(w, http.StatusNotFound, "artifact_not_found", "artifact not found")
		return
	}
	id := r.PathValue("id")
	html, runID, err := s.Artifacts.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "artifact_not_found", "artifact not found")
		return
	}
	if _, err := s.Store.GetRun(runID); err != nil {
		writeError(w, http.StatusNotFound, "artifact_not_found", "artifact not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
		return
	}
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

// ssePollInterval is the SSE fallback poll period when Hub nudges are missing
// (e.g. cross-replica AppendEvent). Tests may shorten it via t.Cleanup restore.
var ssePollInterval = 3 * time.Second

func (s *Server) handleRunStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if !s.requireConversationAccess(w, r, runRec.ConversationID) {
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
	poll := time.NewTicker(ssePollInterval)
	defer poll.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case <-poll.C:
			if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
				return
			} else if catchUpTerminal {
				_ = writeSSEEnded(w, rc, status)
				return
			}
		case ev, ok := <-sub.Events:
			if !ok {
				return
			}
			if ev.Event.Type == "external.nudge" {
				if catchUpTerminal, status, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
					return
				} else if catchUpTerminal {
					_ = writeSSEEnded(w, rc, status)
					return
				}
				continue
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
			// final AppendEvent is not lost when select picks Ended. Then
			// catch up from store: channel may only hold external.nudge
			// placeholders while real events live in the store.
			lastSent, drainErr = drainSubEvents(w, rc, sub, lastSent)
			if drainErr != nil {
				return
			}
			if _, _, err := catchUpRunStream(w, rc, s.Store, id, &lastSent); err != nil {
				return
			}
			_ = writeSSEEnded(w, rc, stt)
			return
		}
	}
}

// drainSubEvents non-blocking writes any buffered subscription events with index > lastSent.
// external.nudge placeholders are skipped (no write / no lastSent advance); callers
// catch up from the store via catchUpRunStream.
func drainSubEvents(w http.ResponseWriter, rc *http.ResponseController, sub *eventbus.Subscription, lastSent int) (int, error) {
	for {
		select {
		case ev, ok := <-sub.Events:
			if !ok {
				return lastSent, nil
			}
			if ev.Event.Type == "external.nudge" {
				continue
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

type executeRunOpts struct {
	skipUserAppend bool
	rebindUserID   string
	// runOpts carries per-run skill overrides and multimodal user parts. The
	// regenerate path leaves this zero-valued (reuse agent defaults, no parts)
	// since attachments are not re-sent on regenerate.
	runOpts run.RunOptions
}

// createAndExecuteRun creates a run, appends the user message, and starts Execute in background.
func (s *Server) createAndExecuteRun(r *http.Request, agentID, input, convID, identityID string, opts executeRunOpts) (*store.Run, error) {
	ag, err := s.Store.GetAgent(agentID)
	if err != nil {
		return nil, fmt.Errorf("unknown agent")
	}
	if err := s.prepareConversationMeta(r.Context(), convID); err != nil {
		return nil, err
	}
	createIn := store.CreateRunInput{
		AgentID:        agentID,
		Input:          input,
		ConversationID: convID,
		IdentityID:     strings.TrimSpace(identityID),
	}
	if authcred.NormalizeMode(s.AuthMode) == authcred.ModePassthrough {
		createIn.PassthroughHeaders = authcred.PickHeaders(r.Header, s.AuthWhitelist)
	}
	runRec, err := s.Store.CreateRun(createIn)
	if err != nil {
		return nil, err
	}
	if opts.skipUserAppend && opts.rebindUserID != "" {
		if s.Messages == nil {
			return nil, fmt.Errorf("message store not configured")
		}
		if err := s.Messages.SetRunID(opts.rebindUserID, runRec.ID); err != nil {
			return nil, err
		}
	} else if s.Messages != nil && convID != "" {
		_, _ = s.Messages.Append(convID, conversation.Message{
			Role:    conversation.RoleUser,
			Content: input,
			RunID:   runRec.ID,
		})
	}
	def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
	_ = s.Store.AppendEvent(runRec.ID, store.Event{Type: run.EventRunStarted})
	s.Dispatch(r.Context(), middleware.Job{
		RunID:     runRec.ID,
		Kind:      middleware.KindRun,
		AgentID:   def.ID,
		Input:     input,
		Skills:    opts.runOpts.Skills,
		UserParts: PartsToMiddleware(opts.runOpts.UserParts),
	})
	return runRec, nil
}
