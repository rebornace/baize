package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	"github.com/rebornace/baize/internal/connector/mcpoauth"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/middleware"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/runtimecfg"
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

// UploadSaver persists chat attachments into the per-conversation file
// workspace. SaveUpload stores extracted text; SaveUploadBytes stores image
// bytes. Both return the workspace-relative logical path. Failures are
// non-fatal: callers log and continue (the turn still proceeds inline).
type UploadSaver interface {
	SaveUpload(ctx context.Context, conversationID, filename, text string) (string, error)
	SaveUploadBytes(ctx context.Context, conversationID, filename string, data []byte, mime string) (string, error)
}

// ChannelMediaOpener serves a previously persisted inbound channel attachment
// (an inline image or a downloadable file). found=false (with nil error) means
// the object does not exist; it is distinct from a backend failure. Implemented
// by internal/channelmedia.Store.
type ChannelMediaOpener interface {
	OpenMedia(ctx context.Context, conversationID, object string) (data []byte, mime string, found bool, err error)
}

// ChatMediaSaver persists chat-uploaded attachments (images and files) to a
// blob-backed namespace and returns browser-reachable relative URLs, so a
// web-uploaded image renders inline and a document downloads from the user
// bubble — the same mechanism inbound IM channels use. Implemented by
// internal/channelmedia.Store.
type ChatMediaSaver interface {
	SaveInboundImage(ctx context.Context, conversationID, filename, mime string, data []byte) (url string, object string, err error)
	SaveInboundFile(ctx context.Context, conversationID, filename, mime string, data []byte) (url string, object string, err error)
}

type Server struct {
	Store     store.Store
	Registry  *tool.Registry
	Artifacts artifact.Store // optional; nil = artifact routes unavailable
	// ChannelMedia optionally serves inbound channel images (e.g. WeChat) for
	// inline display. nil = the media route returns 404.
	ChannelMedia ChannelMediaOpener
	// ChatMedia optionally persists web-uploaded chat attachments (images and
	// files) so they render inline / download from the user bubble. nil = web
	// uploads stay model-only (no durable preview/download).
	ChatMedia ChatMediaSaver
	// Workspace optionally persists chat attachments to the per-conversation
	// file workspace. nil = attachments are not persisted (in-turn only).
	Workspace    UploadSaver
	Runner       Runner
	SkillCatalog *skill.Catalog
	Identities   identity.Store
	Messages     conversation.Store // optional; nil = no message persistence
	// Memory is the account-scoped fact store (P6 settings CRUD). nil = routes unavailable.
	Memory         memory.Store
	Hub            *eventbus.Hub // optional; nil = SSE replay only (no live fan-out)
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
	OperatorToken string
	AdminToken    string
	Operators     []controlplane.Operator
	// Settings optionally supplies hot-reloadable engine knobs and control-plane
	// credentials. nil = use the static OperatorToken/AdminToken/Operators
	// fields above (legacy behavior; existing tests leave it nil).
	Settings *runtimecfg.Holder
	// TierAdvisor optionally arbitrates light/power (DP-4) when Auto routing
	// lands on the standard tier. nil = pure heuristic routing (existing tests).
	TierAdvisor      llm.TierAdvisor
	Webhook          *webhook.Dispatcher // optional; nil = no outbound webhook delivery
	Inbox            *inbox.Registry     // optional; nil = inbox routes unavailable
	InboxLimiter     *inbox.RateLimiter  // optional; nil => lazy default via inboxLimiter()
	inboxLimiterOnce sync.Once
	// InboxGate 可选；非 nil 时 inbox 入站限流走它（key=channelID）。nil 时回退 InboxLimiter。
	InboxGate func(channelID string) bool
	// MCPExportEnabled gates /v0/mcp/export (default true when set by bootstrap).
	MCPExportEnabled bool
	DataDir          string // parent dir for sqlite / legacy paths; not used for connector specs
	Blobs            blob.Store
	ConfigPath       string
	Config           *config.Config
	Shutdown         func(context.Context) error
	RestartProcess   func() error
	// HotSwapStore optionally opens a new store in-process (F-HOT). nil = PUT
	// without restart only writes overlay.
	HotSwapStore func(config.StoreOverlay) error
	// ReloadConfig re-reads layered YAML into runtimecfg baseline (SIGHUP /
	// POST /v0/settings/reload). nil = reload endpoint unavailable.
	ReloadConfig func() error
	// EffectiveStoreDriver / StoreConfigMismatch feed GET /settings/store.
	EffectiveStoreDriver func() string
	StoreConfigMismatch  func() bool
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
	// CallbackPublicBase is also kept in sync with Settings.PublicBaseURL()
	// after PATCH /v0/settings/runtime (see publicBaseURL).
	CallbackSigner     httpplugin.CallbackSigner
	CallbackPublicBase string
	CallbackTTL        time.Duration

	// OAuthSessions holds in-flight MCP OAuth PKCE state (single-process).
	OAuthSessions *mcpoauth.SessionStore

	// Outbound is the channel used to deliver mirrored UI/API user turns and
	// succeeded assistant replies to channel peers. It is normally a
	// *channel.Router that dispatches by conversation meta.Source (tests may
	// pass a concrete channel.Channel). nil = no mirroring.
	Outbound       channel.Channel
	OutboundExtras func(conversationID string) map[string]string

	// channels is the per-channel runtime handle table, keyed by channel name
	// (e.g. "weixin"). The generic management plane looks handles up by name
	// and type-asserts channel.ManagedChannel. Guarded by channelsMu; only map
	// reads/writes happen under the lock (never network/long operations).
	channelsMu sync.RWMutex
	channels   map[string]*ChannelHandle

	mux *http.ServeMux
}

func NewServer(st store.Store, reg *tool.Registry, runner Runner) *Server {
	s := &Server{
		Store:            st,
		Registry:         reg,
		Runner:           runner,
		Identities:       identity.NewMemoryStore(),
		MCPExportEnabled: true,
		OAuthSessions:    mcpoauth.NewSessionStore(),
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

// publicBaseURL returns the advertised Runtime root for OAuth redirects and
// plugin callback_urls. Prefer the hot-reloadable Settings holder when wired
// (tests that only set CallbackPublicBase still work).
func (s *Server) publicBaseURL() string {
	if s.Settings != nil {
		return strings.TrimSpace(s.Settings.PublicBaseURL())
	}
	return strings.TrimSpace(s.CallbackPublicBase)
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
	if s.Settings != nil {
		c := s.Settings.Credentials()
		return controlplane.Tokens{
			Operator:  c.OperatorToken,
			Admin:     c.AdminToken,
			Operators: c.Operators,
		}
	}
	return controlplane.Tokens{
		Operator:  s.OperatorToken,
		Admin:     s.AdminToken,
		Operators: s.Operators,
	}
}

// GateTokensForTest exposes the effective gate tokens for tests.
func (s *Server) GateTokensForTest() controlplane.Tokens { return s.gateTokens() }

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
	s.mux.HandleFunc("GET /v0/connectors", s.handleListConnectors)
	s.mux.HandleFunc("GET /v0/connectors/{id}", s.handleGetConnector)
	s.mux.HandleFunc("POST /v0/connectors/{id}/mcp/oauth/start", s.handleMCPOAuthStart)
	s.mux.HandleFunc("GET /v0/connectors/{id}/mcp/oauth/callback", s.handleMCPOAuthCallback)
	s.mux.HandleFunc("POST /v0/connectors/{id}/mcp/oauth/disconnect", s.handleMCPOAuthDisconnect)
	s.mux.HandleFunc("GET /v0/connectors/{id}/mcp/oauth/status", s.handleMCPOAuthStatus)
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
	s.mux.HandleFunc("GET /v0/channels/media/{conv}/{object}", s.handleChannelMedia)
	s.mux.HandleFunc("GET /v0/conversations/{id}/identities", s.handleListIdentities)
	s.mux.HandleFunc("POST /v0/conversations/{id}/identities", s.handlePostIdentity)
	s.mux.HandleFunc("POST /v0/conversations/{id}/identities/{iid}/default", s.handleSetDefaultIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities/{iid}", s.handleDeleteIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities", s.handleClearIdentities)
	s.mux.HandleFunc("GET /v0/conversations", s.handleListConversations)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}", s.handleDeleteConversation)
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
	s.mux.HandleFunc("POST /v0/settings/reload", s.handlePostSettingsReload)
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/login/start", s.handleChannelLoginStart)
	s.mux.HandleFunc("GET /v0/settings/channels/{name}/login/status", s.handleChannelLoginStatus)
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/logout", s.handleChannelLogout)
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/process/start", s.handleChannelProcessStart)
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/process/stop", s.handleChannelProcessStop)
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/process/restart", s.handleChannelProcessRestart)
	s.mux.HandleFunc("GET /v0/settings/channels/{name}/outbound-deliveries", s.handleGetChannelOutboundDeliveries)
	s.mux.HandleFunc("POST /v0/settings/channels/{name}/outbound-deliveries/{id}/retry", s.handlePostChannelOutboundDeliveryRetry)
	s.mux.HandleFunc("GET /v0/settings/channels/{name}", s.handleGetChannelSettings)
	s.mux.HandleFunc("PUT /v0/settings/channels/{name}", s.handlePutChannelSettings)
	s.mux.HandleFunc("GET /v0/settings/runtime", s.handleGetRuntimeSettings)
	s.mux.HandleFunc("PATCH /v0/settings/runtime", s.handlePatchRuntimeSettings)
	s.mux.HandleFunc("GET /v0/settings/credentials", s.handleGetCredentials)
	s.mux.HandleFunc("PATCH /v0/settings/credentials", s.handlePatchCredentials)

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
	s.mux.HandleFunc("POST /v0/settings/models/discover", s.handleDiscoverModels)
	s.mux.HandleFunc("POST /v0/settings/models/batch", s.handleBatchImportModels)
	s.mux.HandleFunc("PATCH /v0/settings/models/{id}", s.handlePatchModelProfile)
	s.mux.HandleFunc("DELETE /v0/settings/models/{id}", s.handleDeleteModelProfile)
	s.mux.HandleFunc("GET /v0/settings/memory", s.handleListMemory)
	s.mux.HandleFunc("POST /v0/settings/memory", s.handlePostMemory)
	s.mux.HandleFunc("PATCH /v0/settings/memory/{id}", s.handlePatchMemory)
	s.mux.HandleFunc("DELETE /v0/settings/memory/{id}", s.handleDeleteMemory)
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

// nonNilStrings guarantees an empty gate list serializes as [] rather than
// null (SQLite omits empty lists, so a freshly cleared connector loads nil).
func nonNilStrings(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
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

// routingProfiles returns the current model profiles in the compact form the
// llm router needs. Read live so adding/editing a model in Settings takes
// effect without a restart.
func (s *Server) routingProfiles() []llm.RoutingProfile {
	if s.Store == nil {
		return nil
	}
	list, err := s.Store.ListModelProfiles()
	if err != nil {
		return nil
	}
	return llm.RoutingProfilesFrom(list)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"role":         string(controlplane.RoleFrom(r.Context())),
		"operator_id":  controlplane.OperatorIDFrom(r.Context()),
		"gate_enabled": s.gateTokens().Enabled(),
	})
}
