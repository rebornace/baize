package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/analysis"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channel/weixin"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/webhook"
)

// Run starts Runtime (and optionally mock-ticket) in-process and blocks on the API server.
func Run(cfg config.Config, configPath string) error {
	ticketListen := strings.TrimSpace(cfg.MockTicket.Listen)
	useMockTicket := true
	switch strings.ToLower(ticketListen) {
	case "off", "false", "none", "-":
		useMockTicket = false
	case "":
		ticketListen = ":18080"
	}

	var ticketBase string
	if useMockTicket {
		go func() {
			log.Printf("mock-ticket listening on %s", ticketListen)
			if err := http.ListenAndServe(ticketListen, mockticket.NewHandler()); err != nil {
				log.Printf("mock-ticket server error: %v", err)
			}
		}()
		ticketBase = localHTTPBase(ticketListen)
		cfg.Connector.BaseURL = ticketBase
		if err := waitHealthy(ticketBase+"/healthz", 5*time.Second); err != nil {
			return fmt.Errorf("mock-ticket health check failed: %w", err)
		}
	} else {
		ticketBase = cfg.Connector.BaseURL
	}

	srv, closer, err := newAPIServer(cfg, configPath)
	if err != nil {
		return err
	}

	listen := cfg.Listen
	if listen == "" {
		listen = ":8080"
	}
	runtimeBase := localHTTPBase(listen)
	printCurlHints(runtimeBase, cfg.Agent.ID, ticketBase)
	logUIHint(cfg, runtimeBase)
	log.Printf("baize runtime listening on %s", listen)
	httpSrv := &http.Server{Addr: listen, Handler: srv.Handler()}
	srv.Shutdown = httpSrv.Shutdown
	srv.RestartProcess = Reexec
	err = httpSrv.ListenAndServe()
	_ = closer.Close()
	return err
}

// Serve starts Runtime only (no mock-ticket) and blocks on the API server.
func Serve(cfg config.Config, configPath string) error {
	srv, closer, err := newAPIServer(cfg, configPath)
	if err != nil {
		return err
	}
	listen := cfg.Listen
	if listen == "" {
		listen = ":8080"
	}
	runtimeBase := localHTTPBase(listen)
	printCurlHints(runtimeBase, cfg.Agent.ID, cfg.Connector.BaseURL)
	logUIHint(cfg, runtimeBase)
	log.Printf("baize runtime listening on %s", listen)
	httpSrv := &http.Server{Addr: listen, Handler: srv.Handler()}
	srv.Shutdown = httpSrv.Shutdown
	srv.RestartProcess = Reexec
	err = httpSrv.ListenAndServe()
	_ = closer.Close()
	return err
}

// StartForTest starts mock-ticket and Runtime on ephemeral ports for integration tests.
// cfg.Connector.BaseURL is overwritten with the actual ticket URL.
func StartForTest(t testing.TB, cfg config.Config) (runtimeURL, ticketURL string, shutdown func()) {
	t.Helper()

	ticketLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen mock-ticket: %v", err)
	}
	ticketURL = "http://" + ticketLn.Addr().String()
	ticketSrv := &http.Server{Handler: mockticket.NewHandler()}
	go func() { _ = ticketSrv.Serve(ticketLn) }()

	if err := waitHealthy(ticketURL+"/healthz", 5*time.Second); err != nil {
		_ = ticketSrv.Close()
		t.Fatalf("mock-ticket health check failed: %v", err)
	}

	cfg.Connector.BaseURL = ticketURL
	if cfg.Run.MaxSteps <= 0 {
		cfg.Run.MaxSteps = 16
	}

	apiSrv, storeClose, err := newAPIServer(cfg, "")
	if err != nil {
		_ = ticketSrv.Close()
		t.Fatalf("new api server: %v", err)
	}

	runtimeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = storeClose.Close()
		_ = ticketSrv.Close()
		t.Fatalf("listen runtime: %v", err)
	}
	runtimeURL = "http://" + runtimeLn.Addr().String()
	httpSrv := &http.Server{Handler: apiSrv.Handler()}
	go func() { _ = httpSrv.Serve(runtimeLn) }()

	if err := waitHealthy(runtimeURL+"/healthz", 5*time.Second); err != nil {
		_ = httpSrv.Close()
		_ = storeClose.Close()
		_ = ticketSrv.Close()
		t.Fatalf("runtime health check failed: %v", err)
	}

	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		_ = ticketSrv.Shutdown(ctx)
		_ = storeClose.Close()
	}
	return runtimeURL, ticketURL, shutdown
}

func newAPIServer(cfg config.Config, configPath string) (*api.Server, io.Closer, error) {
	// newLLM validates the YAML llm section at startup and provides the
	// provider for the mock/demo path. Real deployments replace it with the
	// hot-reloadable Switch once the store (and its model profiles) is open.
	baseProvider, err := newLLM(cfg)
	if err != nil {
		return nil, nil, err
	}

	// Resolve sidecar callback wiring (Phase 2). An empty secret triggers an
	// ephemeral key so single-process dev still works; an explicit secret is
	// required for multi-process deployments where the API and the sidecar
	// invoke path must share the key.
	callbackSecret, secretEphemeral := resolveCallbackSecret(cfg.Runtime.CallbackHMACSecret)
	if secretEphemeral {
		log.Printf("runtime.callback_hmac_secret empty: generated ephemeral secret (not valid across restarts)")
	}
	callbackTTL := time.Duration(cfg.Runtime.CallbackTokenTTLSec) * time.Second
	if callbackTTL <= 0 {
		callbackTTL = time.Duration(config.DefaultCallbackTokenTTLSec) * time.Second
	}
	callbackPublicBase := strings.TrimSpace(cfg.Runtime.PublicBaseURL)
	callbackSigner := httpplugin.CallbackSigner(plugincallback.Issue)
	callbackLimiter := plugincallback.NewLimiter(plugincallback.DefaultBudget, plugincallback.DefaultWindow)
	callbackCfg := connector.CallbackConfig{
		Signer:     callbackSigner,
		Secret:     callbackSecret,
		PublicBase: callbackPublicBase,
		TTL:        callbackTTL,
	}

	st, err := store.OpenWithOptions(cfg.Store.Driver, store.OpenOptions{
		SQLitePath: cfg.Store.SQLitePath,
		DSN:        cfg.Store.DSN,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	closer := &storeAndMCPCloser{inner: storeCloser(st)}

	// Resolve the active LLM provider. The mock/demo path (provider unset or
	// "mock", matching newLLM) keeps the YAML-built provider and neither seeds
	// profiles nor builds a Switch. Real deployments seed a default profile
	// from the YAML llm section on first boot and route through the Switch,
	// which resolves providers from stored profiles and hot-reloads on edit.
	provider := baseProvider
	switch strings.ToLower(cfg.LLM.Provider) {
	case "", "mock":
		// demo/test path: keep baseProvider as-is.
	default:
		if err := seedModelProfile(st, cfg); err != nil {
			_ = closer.Close()
			return nil, nil, err
		}
		provider = llm.NewSwitch(&llm.StoreProfileSource{Store: st})
	}

	reg := tool.NewRegistry()

	skillCat, err := skill.LoadCatalog(cfg.SkillBuiltinDirs(), cfg.Skills.UserDir)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("load skill catalog: %w", err)
	}

	st.UpsertAgent(store.Agent{
		ID:     cfg.Agent.ID,
		System: cfg.Agent.System,
		Skills: append([]string(nil), cfg.Agent.Skills...),
	})

	messages, identities, err := openConversationAndIdentities(st, cfg)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("open conversation/identity stores: %w", err)
	}

	if err := registerConnector(st, reg, cfg, identities, callbackCfg); err != nil {
		_ = closer.Close()
		return nil, nil, err
	}
	loadStoredConnectors(st, reg, cfg, identities, callbackCfg)
	if n := len(st.ListConnectors()); n > 0 {
		ids := make([]string, 0, n)
		for _, c := range st.ListConnectors() {
			ids = append(ids, c.ID)
		}
		log.Printf("persisted connectors restored from store: %v", ids)
	}

	// Artifact store needs the underlying SQL backend; eventbus.Notify wraps st
	// and would break a direct type assertion below.
	var sqlBackend store.SQLBackend
	if sb, ok := st.(store.SQLBackend); ok {
		sqlBackend = sb
	}

	hub := eventbus.NewHub()
	st = eventbus.Notify(st, hub)

	webhookCfg, err := loadEventsWebhook(st, cfg)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("load events webhook: %w", err)
	}
	dispatcher := webhook.NewDispatcher(st, webhookCfg)
	dispatcher.Attach(hub)
	webhookWorkerCtx, webhookWorkerCancel := context.WithCancel(context.Background())
	go dispatcher.StartWorker(webhookWorkerCtx)
	closer.stops = append(closer.stops, webhookWorkerCancel)

	inboxReg := inbox.NewRegistry()
	if err := seedInboxChannels(cfg, st, inboxReg); err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("seed inbox channels: %w", err)
	}

	engine := &run.Engine{
		Store:       st,
		LLM:         provider,
		Tools:       reg,
		Gate:        run.NewGate(),
		MaxSteps:    cfg.Run.MaxSteps,
		ToolTimeout: time.Duration(cfg.Run.ToolTimeoutSec) * time.Second,
		Messages:    messages,
		MaxMessages: cfg.Conversation.MaxMessages,
		Identities:  identities,
		Skills:      skillCat,
	}
	srv := api.NewServer(st, reg, engine)
	cfgCopy := cfg
	srv.Config = &cfgCopy
	srv.ConfigPath = configPath
	srv.Hub = hub
	srv.Webhook = dispatcher
	srv.Inbox = inboxReg
	srv.InboxLimiter = inbox.NewRateLimiter(inbox.DefaultRateLimit, inbox.DefaultRateWindow)
	srv.MCPExportEnabled = cfg.MCPExportEnabled()
	srv.SkillCatalog = skillCat
	srv.Identities = identities
	srv.Messages = messages
	srv.DefaultAgentID = cfg.Agent.ID
	srv.LLM = provider
	srv.CallbackSecret = callbackSecret
	srv.CallbackLimiter = callbackLimiter
	srv.CallbackSigner = callbackSigner
	srv.CallbackPublicBase = callbackPublicBase
	srv.CallbackTTL = callbackTTL

	if sqlBackend != nil {
		artDir := artifactDataDir(cfg)
		artStore, err := artifact.NewFileStore(artDir, sqlBackend)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("open artifact store: %w", err)
		}
		reg.RegisterSpec(analysis.ToolSpec(), analysis.Invoker(artStore))
		srv.Artifacts = artStore
	}
	srv.AuthMode = authcred.NormalizeMode(cfg.Connector.Auth.Mode)
	srv.AuthWhitelist = cfg.Connector.Auth.Passthrough.Headers

	op, err := controlplane.ResolveSecret(cfg.ControlPlane.OperatorToken)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("control_plane.operator_token: %w", err)
	}
	adm, err := controlplane.ResolveSecret(cfg.ControlPlane.AdminToken)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("control_plane.admin_token: %w", err)
	}
	var operators []controlplane.Operator
	for _, entry := range cfg.ControlPlane.Operators {
		token, err := controlplane.ResolveSecret(entry.Token)
		if err != nil {
			_ = closer.Close()
			return nil, nil, fmt.Errorf("control_plane.operators[%s]: %w", entry.ID, err)
		}
		if token == "" {
			continue
		}
		operators = append(operators, controlplane.Operator{ID: entry.ID, Token: token})
	}
	srv.OperatorToken = op
	srv.AdminToken = adm
	srv.Operators = operators
	if dir := dataDir(cfg); dir != "" {
		srv.DataDir = dir
	}
	if err := wireWeixinChannel(srv, st, engine, messages, provider, closer); err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("wire weixin channel: %w", err)
	}
	return srv, closer, nil
}

func wireWeixinChannel(srv *api.Server, st store.Store, engine *run.Engine, messages conversation.Store, provider llm.Provider, closer *storeAndMCPCloser) error {
	credsDir := api.DefaultWeixinCredsDir
	settings, err := api.LoadWeixinChannelSettings(credsDir)
	if err != nil {
		return err
	}

	assignee := strings.TrimSpace(settings.Assignee)
	if assignee == "" {
		assignee = "channel:weixin"
	}
	agentID := strings.TrimSpace(settings.AgentID)
	if agentID == "" {
		agentID = srv.DefaultAgentID
	}

	meta, ok := messages.(conversation.MetaStore)
	if !ok {
		return fmt.Errorf("conversation store does not support meta")
	}

	supportsVision := false
	if provider != nil {
		supportsVision = provider.SupportsVision()
	}

	rt := &channel.Runtime{
		Runs:           st,
		Meta:           meta,
		Messages:       messages,
		Assignee:       assignee,
		DefaultAgentID: agentID,
		SupportsVision: supportsVision,
		AfterCreateRun: func(ctx context.Context, runRec *store.Run, userParts []llm.ContentPart) error {
			ag, err := st.GetAgent(runRec.AgentID)
			if err != nil {
				return err
			}
			def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
			opts := run.RunOptions{UserParts: userParts}
			go func() {
				_ = engine.ExecuteWithOpts(context.Background(), runRec.ID, def, runRec.Input, opts)
			}()
			return nil
		},
		ResumeHITL: func(ctx context.Context, runID string, approve bool, comment string) error {
			return engine.ContinueFromHITL(ctx, runID, run.Decision{Approve: approve, Comment: comment})
		},
	}

	accountID, token, credErr := weixin.LoadCreds(credsDir)
	if credErr != nil {
		accountID, token = "", ""
	}
	ilink := weixin.NewClient("", nil)
	ch := weixin.New(ilink, rt, accountID, token)
	ch.SetCredsDir(credsDir)

	runCtx, cancel := context.WithCancel(context.Background())
	closer.stops = append(closer.stops, func() {
		cancel()
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = ch.Stop(stopCtx)
	})

	srv.WeixinILink = ilink
	srv.WeixinChannel = ch
	srv.WeixinRuntime = rt
	srv.WeixinCredsDir = credsDir
	srv.WeixinRunCtx = runCtx

	engine.Meta = meta
	engine.Outbound = ch
	engine.OutboundExtras = rt.OutboundExtras

	if credErr == nil && settings.Enabled {
		if err := ch.Start(runCtx); err != nil {
			log.Printf("weixin channel: start skipped: %v", err)
		} else {
			log.Printf("weixin channel: started (creds_dir=%s)", credsDir)
		}
	}
	return nil
}

func dataDir(cfg config.Config) string {
	switch strings.ToLower(cfg.Store.Driver) {
	case "sqlite":
		if strings.TrimSpace(cfg.Store.SQLitePath) != "" {
			return filepath.Dir(cfg.Store.SQLitePath)
		}
	case "postgres":
		return "./data"
	}
	return ""
}

func artifactDataDir(cfg config.Config) string {
	if dir := dataDir(cfg); dir != "" {
		return filepath.Join(dir, "artifacts")
	}
	return "./data/artifacts"
}

// loadEventsWebhook returns the persisted events webhook config, seeding from
// YAML defaults when the settings KV is empty.
func loadEventsWebhook(st store.Store, cfg config.Config) (webhook.Config, error) {
	raw, ok, err := st.GetSetting(store.SettingKeyEventsWebhook)
	if err != nil {
		return webhook.Config{}, err
	}
	if ok && len(raw) > 0 {
		var out webhook.Config
		if err := json.Unmarshal(raw, &out); err != nil {
			return webhook.Config{}, err
		}
		return out, nil
	}
	def := webhook.Config{
		URL:     cfg.Events.Webhook.URL,
		Headers: cfg.Events.Webhook.Headers,
	}
	if def.Headers == nil {
		def.Headers = map[string]string{}
	}
	b, err := json.Marshal(def)
	if err != nil {
		return webhook.Config{}, err
	}
	if err := st.UpsertSetting(store.SettingKeyEventsWebhook, b); err != nil {
		return webhook.Config{}, err
	}
	return def, nil
}

// seedInboxChannels loads channel config from the settings KV when present,
// otherwise seeds from YAML and persists generated secrets to the store.
func seedInboxChannels(cfg config.Config, st store.Store, reg *inbox.Registry) error {
	raw, ok, err := st.GetSetting(store.SettingKeyInboxChannels)
	if err != nil {
		return err
	}
	if ok && len(raw) > 0 {
		var channels []inbox.Channel
		if err := json.Unmarshal(raw, &channels); err != nil {
			return err
		}
		reg.Replace(channels)
		return nil
	}

	channels := append([]inbox.Channel(nil), cfg.Inbox.Channels...)
	for i := range channels {
		if strings.TrimSpace(channels[i].Secret) == "" {
			channels[i].Secret = inbox.GenerateSecret()
			log.Printf("warning: inbox channel %q had empty secret; generated and persisted to store", channels[i].ID)
		}
	}
	reg.Replace(channels)
	b, err := json.Marshal(channels)
	if err != nil {
		return err
	}
	return st.UpsertSetting(store.SettingKeyInboxChannels, b)
}

// openConversationAndIdentities builds the conversation message store and the
// identity store based on the configured driver.
//
//   - memory driver: both stores are in-memory (PersistIdentities is ignored).
//   - sqlite/postgres: messages and (by default) identities use the shared SQL DB.
func openConversationAndIdentities(st store.Store, cfg config.Config) (conversation.Store, identity.Store, error) {
	driver := strings.ToLower(cfg.Store.Driver)
	if driver == "" {
		driver = "memory"
	}

	if driver == "memory" {
		return conversation.NewMemoryStore(), identity.NewMemoryStore(), nil
	}

	sqlBackend, ok := st.(store.SQLBackend)
	if !ok {
		return conversation.NewMemoryStore(), identity.NewMemoryStore(), nil
	}
	db := sqlBackend.DB()
	dialect := sqlBackend.Dialect()

	msgs, err := conversation.OpenSQL(db, dialect)
	if err != nil {
		return nil, nil, err
	}

	persist := cfg.Conversation.PersistIdentities
	if persist != nil && !*persist {
		return msgs, identity.NewMemoryStore(), nil
	}
	identities, err := identity.OpenSQL(db, dialect)
	if err != nil {
		return nil, nil, err
	}
	return msgs, identities, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

func storeCloser(st store.Store) io.Closer {
	if c, ok := st.(io.Closer); ok {
		return c
	}
	return nopCloser{}
}

type storeAndMCPCloser struct {
	inner io.Closer
	stops []func()
}

func (c *storeAndMCPCloser) Close() error {
	for i := len(c.stops) - 1; i >= 0; i-- {
		if c.stops[i] != nil {
			c.stops[i]()
		}
	}
	connector.CloseAllMCPSessions()
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

func registerConnector(st store.Store, reg *tool.Registry, cfg config.Config, identities identity.Store, cb connector.CallbackConfig) error {
	if strings.TrimSpace(cfg.Connector.ID) == "" {
		return nil
	}
	// YAML 省略 require_login 时传 nil，让 MergeCatalog 保留行上已持久化的
	// per-tool require_login；仅当 YAML 显式给出名单（含空数组）时才传指针，
	// 否则重启会把设置页勾选的 require_login 冲掉。
	var requireLogin *[]string
	if cfg.Connector.RequireLogin != nil {
		login := cfg.Connector.RequireLogin
		requireLogin = &login
	}
	_, _, err := connector.Apply(connector.ApplyInput{
		Store:                   st,
		Registry:                reg,
		Identities:              identities,
		ID:                      cfg.Connector.ID,
		Type:                    cfg.Connector.Type,
		Spec:                    cfg.Connector.Spec,
		BaseURL:                 cfg.Connector.BaseURL,
		ExecutionCallbackURL:    cfg.Connector.ExecutionCallbackURL,
		RequireApproval:         cfg.Connector.RequireApproval,
		RequireApprovalMutating: cfg.Connector.RequireApprovalMutating,
		RequireLogin:            requireLogin,
		Auth: store.ConnectorAuth{
			Mode:        cfg.Connector.Auth.Mode,
			Static:      store.StaticAuth{Headers: cfg.Connector.Auth.Static.Headers},
			Passthrough: store.PassThruAuth{Headers: cfg.Connector.Auth.Passthrough.Headers},
			VaultRef:    store.VaultRefAuth{Headers: cfg.Connector.Auth.VaultRef.Headers},
			Capture: store.CaptureAuth{
				ToolNameGlob:   cfg.Connector.Auth.Capture.ToolNameGlob,
				TokenJSONPaths: cfg.Connector.Auth.Capture.TokenJSONPaths,
				LabelJSONPaths: cfg.Connector.Auth.Capture.LabelJSONPaths,
				HeaderTemplate: cfg.Connector.Auth.Capture.HeaderTemplate,
				DefaultScheme:  cfg.Connector.Auth.Capture.DefaultScheme,
			},
		},
		CallbackSigner:     cb.Signer,
		CallbackSecret:     cb.Secret,
		CallbackPublicBase: cb.PublicBase,
		CallbackTTL:        cb.TTL,
	})
	return err
}

// loadStoredConnectors re-applies every connector persisted in the Store other
// than the YAML-configured one. Apply re-merges the persisted catalog (so
// disabled rows and extra rows survive a restart) and re-registers only the
// enabled rows. Errors are logged but do not abort startup so a single bad
// connector cannot brick the runtime; the YAML connector is skipped because it
// was just registered by registerConnector.
func loadStoredConnectors(st store.Store, reg *tool.Registry, cfg config.Config, identities identity.Store, cb connector.CallbackConfig) {
	for _, c := range st.ListConnectors() {
		if c.ID == cfg.Connector.ID {
			continue
		}
		_, _, err := connector.Apply(connector.ApplyInput{
			Store:                st,
			Registry:             reg,
			Identities:           identities,
			ID:                   c.ID,
			Type:                 c.Type,
			Spec:                 c.Spec,
			BaseURL:              c.BaseURL,
			ExecutionCallbackURL: c.ExecutionCallbackURL,
			Auth:                 c.Auth,
			MCP:                  c.MCP,
			RequireApproval:      c.RequireApproval,
			RequireLogin:         nil, // preserve persisted per-tool require_login
			CallbackSigner:       cb.Signer,
			CallbackSecret:       cb.Secret,
			CallbackPublicBase:   cb.PublicBase,
			CallbackTTL:          cb.TTL,
		})
		if err != nil {
			log.Printf("loadStoredConnectors: %s: %v", c.ID, err)
		}
	}
}

// seedModelProfile ensures at least one profile exists, seeding a default from
// the YAML llm section on first boot. The API key is resolved from the
// environment (api_key_env) at call time and is never stored; the profile
// keeps api_key_env so the Switch can resolve the key per request.
func seedModelProfile(st store.Store, cfg config.Config) error {
	list, err := st.ListModelProfiles()
	if err != nil {
		return err
	}
	if len(list) > 0 {
		return nil
	}
	env := cfg.LLM.APIKeyEnv
	if env == "" {
		env = "BAIZE_API_KEY"
	}
	if _, err := st.UpsertModelProfile(store.ModelProfile{
		Name:            "默认模型",
		Provider:        "openai_compatible",
		BaseURL:         cfg.LLM.BaseURL,
		Model:           cfg.LLM.Model,
		APIKeyEnv:       env,
		DisableThinking: cfg.LLM.DisableThinking,
		SupportsVision:  cfg.LLM.SupportsVision,
		IsDefault:       true,
	}); err != nil {
		return fmt.Errorf("seed model profile: %w", err)
	}
	return nil
}

func newLLM(cfg config.Config) (llm.Provider, error) {
	switch strings.ToLower(cfg.LLM.Provider) {
	case "", "mock":
		m := llm.NewMock()
		m.VisionSupported = cfg.LLM.SupportsVision
		return m, nil
	case "openai_compatible":
		env := cfg.LLM.APIKeyEnv
		if env == "" {
			env = "BAIZE_API_KEY"
		}
		p := llm.NewOpenAI(cfg.LLM.BaseURL, os.Getenv(env), cfg.LLM.Model)
		p.DisableThinking = cfg.LLM.DisableThinking
		p.VisionSupported = cfg.LLM.SupportsVision
		return p, nil
	default:
		return nil, fmt.Errorf("unknown llm.provider: %s", cfg.LLM.Provider)
	}
}

func waitHealthy(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	var last error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}

func localHTTPBase(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		// listen may be ":8080"
		if strings.HasPrefix(listen, ":") {
			return "http://127.0.0.1" + listen
		}
		return "http://" + listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// resolveCallbackSecret returns the configured HMAC secret, or a freshly
// generated 32-byte ephemeral secret when none is configured. The second
// return is true when the secret was ephemeral so the caller can log it.
// An ephemeral secret is only valid for the lifetime of this process; tokens
// issued before a restart are invalid afterwards, which is acceptable for
// local dev but not for multi-process deployments.
func resolveCallbackSecret(configured string) ([]byte, bool) {
	if s := strings.TrimSpace(configured); s != "" {
		return []byte(s), false
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// rand.Read should never fail on modern systems; fall back to a
		// time-seeded-ish key to keep startup alive (still ephemeral).
		log.Printf("crypto/rand failed (%v); using weak ephemeral secret", err)
		for i := range buf {
			buf[i] = byte(time.Now().UnixNano() >> uint(i%8))
		}
	}
	return buf, true
}

func printCurlHints(runtimeBase, agentID, ticketBase string) {
	if agentID == "" {
		agentID = "ticket-agent"
	}
	if ticketBase == "" {
		ticketBase = "http://127.0.0.1:18080"
	}
	log.Printf("baize ready. try:")
	log.Printf(`  curl -s -X POST %s/v0/runs -H "Content-Type: application/json" -d "{\"agent_id\":\"%s\",\"input\":\"创建一个紧急工单：VPN 挂了\"}"`, runtimeBase, agentID)
	log.Printf(`  curl -s %s/tickets`, ticketBase)
}

func logUIHint(cfg config.Config, runtimeBase string) {
	if cfg.UI.Enabled {
		log.Printf("open %s/ui", runtimeBase)
	}
}
