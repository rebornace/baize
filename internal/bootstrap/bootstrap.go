package bootstrap

import (
	"context"
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
	"github.com/rebornace/baize/internal/analysis"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/webhook"
)

// Run starts Runtime (and optionally mock-ticket) in-process and blocks on the API server.
func Run(cfg config.Config) error {
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

	srv, _, err := newAPIServer(cfg)
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
	return http.ListenAndServe(listen, srv.Handler())
}

// Serve starts Runtime only (no mock-ticket) and blocks on the API server.
func Serve(cfg config.Config) error {
	srv, _, err := newAPIServer(cfg)
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
	return http.ListenAndServe(listen, srv.Handler())
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

	apiSrv, storeClose, err := newAPIServer(cfg)
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

func newAPIServer(cfg config.Config) (*api.Server, io.Closer, error) {
	provider, err := newLLM(cfg)
	if err != nil {
		return nil, nil, err
	}

	st, err := store.Open(cfg.Store.Driver, cfg.Store.SQLitePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	closer := &storeAndMCPCloser{inner: storeCloser(st)}
	reg := tool.NewRegistry()

	skillCat, err := skill.LoadCatalog(cfg.Skills.BuiltinDir, cfg.Skills.UserDir)
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

	if err := registerConnector(st, reg, cfg, identities); err != nil {
		_ = closer.Close()
		return nil, nil, err
	}
	loadStoredConnectors(st, reg, cfg, identities)
	if n := len(st.ListConnectors()); n > 0 {
		ids := make([]string, 0, n)
		for _, c := range st.ListConnectors() {
			ids = append(ids, c.ID)
		}
		log.Printf("persisted connectors restored from store: %v", ids)
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

	engine := &run.Engine{
		Store:       st,
		LLM:         provider,
		Tools:       reg,
		Gate:        run.NewGate(),
		MaxSteps:    cfg.Run.MaxSteps,
		Messages:    messages,
		MaxMessages: cfg.Conversation.MaxMessages,
		Identities:  identities,
		Skills:      skillCat,
	}
	srv := api.NewServer(st, reg, engine)
	srv.Hub = hub
	srv.Webhook = dispatcher
	srv.SkillCatalog = skillCat
	srv.Identities = identities
	srv.Messages = messages
	srv.DefaultAgentID = cfg.Agent.ID

	if sqlite, ok := st.(*store.SQLite); ok && strings.TrimSpace(cfg.Store.SQLitePath) != "" {
		artDir := filepath.Join(filepath.Dir(cfg.Store.SQLitePath), "artifacts")
		artStore, err := artifact.NewFileStore(artDir, sqlite)
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
	srv.OperatorToken = op
	srv.AdminToken = adm
	if strings.TrimSpace(cfg.Store.SQLitePath) != "" {
		srv.DataDir = filepath.Dir(cfg.Store.SQLitePath)
	}
	return srv, closer, nil
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

// openConversationAndIdentities builds the conversation message store and the
// identity store based on the configured driver.
//
//   - memory driver: both stores are in-memory (PersistIdentities is ignored).
//   - sqlite driver: messages use a SQLite table on the shared *sql.DB;
//     identities use SQLite when PersistIdentities is nil (default true) or
//     explicitly true, and fall back to an in-memory store when explicitly
//     set to false.
func openConversationAndIdentities(st store.Store, cfg config.Config) (conversation.Store, identity.Store, error) {
	driver := strings.ToLower(cfg.Store.Driver)
	if driver == "" {
		driver = "memory"
	}

	if driver != "sqlite" {
		return conversation.NewMemoryStore(), identity.NewMemoryStore(), nil
	}

	sqlite, ok := st.(*store.SQLite)
	if !ok {
		// Defensive: store.Open("sqlite", ...) always returns *SQLite.
		return conversation.NewMemoryStore(), identity.NewMemoryStore(), nil
	}
	db := sqlite.DB()

	msgs, err := conversation.OpenSQLite(db)
	if err != nil {
		return nil, nil, err
	}

	persist := cfg.Conversation.PersistIdentities
	if persist != nil && !*persist {
		identities := identity.NewMemoryStore()
		return msgs, identities, nil
	}
	identities, err := identity.OpenSQLite(db)
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
}

func (c *storeAndMCPCloser) Close() error {
	connector.CloseAllMCPSessions()
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

func registerConnector(st store.Store, reg *tool.Registry, cfg config.Config, identities identity.Store) error {
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
	})
	return err
}

// loadStoredConnectors re-applies every connector persisted in the Store other
// than the YAML-configured one. Apply re-merges the persisted catalog (so
// disabled rows and extra rows survive a restart) and re-registers only the
// enabled rows. Errors are logged but do not abort startup so a single bad
// connector cannot brick the runtime; the YAML connector is skipped because it
// was just registered by registerConnector.
func loadStoredConnectors(st store.Store, reg *tool.Registry, cfg config.Config, identities identity.Store) {
	for _, c := range st.ListConnectors() {
		if c.ID == cfg.Connector.ID {
			continue
		}
		_, _, err := connector.Apply(connector.ApplyInput{
			Store:           st,
			Registry:        reg,
			Identities:      identities,
			ID:              c.ID,
			Type:            c.Type,
			Spec:            c.Spec,
			BaseURL:              c.BaseURL,
			ExecutionCallbackURL: c.ExecutionCallbackURL,
			Auth:                 c.Auth,
			MCP:             c.MCP,
			RequireApproval: c.RequireApproval,
			RequireLogin:    nil, // preserve persisted per-tool require_login
		})
		if err != nil {
			log.Printf("loadStoredConnectors: %s: %v", c.ID, err)
		}
	}
}

func newLLM(cfg config.Config) (llm.Provider, error) {
	switch strings.ToLower(cfg.LLM.Provider) {
	case "", "mock":
		return llm.NewMock(), nil
	case "openai_compatible":
		env := cfg.LLM.APIKeyEnv
		if env == "" {
			env = "BAIZE_API_KEY"
		}
		p := llm.NewOpenAI(cfg.LLM.BaseURL, os.Getenv(env), cfg.LLM.Model)
		p.DisableThinking = cfg.LLM.DisableThinking
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
