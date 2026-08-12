package bootstrap

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
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
	closer := storeCloser(st)
	reg := tool.NewRegistry()

	st.UpsertAgent(store.Agent{ID: cfg.Agent.ID, System: cfg.Agent.System})

	messages, identities, err := openConversationAndIdentities(st, cfg)
	if err != nil {
		_ = closer.Close()
		return nil, nil, fmt.Errorf("open conversation/identity stores: %w", err)
	}

	if err := registerConnector(st, reg, cfg, identities); err != nil {
		_ = closer.Close()
		return nil, nil, err
	}

	engine := &run.Engine{
		Store:       st,
		LLM:         provider,
		Tools:       reg,
		Gate:        run.NewGate(),
		MaxSteps:    cfg.Run.MaxSteps,
		Messages:    messages,
		MaxMessages: cfg.Conversation.MaxMessages,
	}
	srv := api.NewServer(st, reg, engine)
	srv.Identities = identities
	srv.Messages = messages
	srv.DefaultAgentID = cfg.Agent.ID
	return srv, closer, nil
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

func registerConnector(st store.Store, reg *tool.Registry, cfg config.Config, identities identity.Store) error {
	typ := cfg.Connector.Type
	if typ == "" {
		typ = "openapi"
	}
	if cfg.Connector.Spec == "" {
		return fmt.Errorf("connector.spec is required")
	}
	approval := cfg.Connector.RequireApproval
	capture := identity.CaptureConfig{
		ToolNameGlob:   cfg.Connector.Auth.Capture.ToolNameGlob,
		TokenJSONPaths: cfg.Connector.Auth.Capture.TokenJSONPaths,
		LabelJSONPaths: cfg.Connector.Auth.Capture.LabelJSONPaths,
		HeaderTemplate: cfg.Connector.Auth.Capture.HeaderTemplate,
		DefaultScheme:  cfg.Connector.Auth.Capture.DefaultScheme,
	}
	capture = withCaptureDefaults(capture)
	if capture.DefaultScheme == "" {
		if routes, err := openapi.LoadTools(cfg.Connector.Spec); err == nil {
			capture.DefaultScheme = uniqueSecurityScheme(routes)
		}
	}
	_, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID:                      cfg.Connector.ID,
		Type:                    typ,
		SpecPath:                cfg.Connector.Spec,
		BaseURL:                 cfg.Connector.BaseURL,
		RequireApproval:         approval,
		RequireApprovalMutating: cfg.Connector.RequireApprovalMutating,
		Headers:                 resolveBearerHeaders(cfg.Connector.Auth.BearerEnv),
		Identities:              identities,
		Resolver:                authresolve.OpenAPISecurityResolver{},
		Capture:                 capture,
	})
	return err
}

// withCaptureDefaults fills open-box login capture when config omits capture.
// default.local.yaml often only sets bearer_env; without defaults, login never persists.
// To disable capture, set tool_name_glob to a non-matching pattern (e.g. "__none__").
func withCaptureDefaults(c identity.CaptureConfig) identity.CaptureConfig {
	if strings.TrimSpace(c.ToolNameGlob) != "" {
		return c
	}
	c.ToolNameGlob = "*login*"
	if len(c.TokenJSONPaths) == 0 {
		c.TokenJSONPaths = []string{"accessToken", "data.accessToken", "data.token"}
	}
	if len(c.LabelJSONPaths) == 0 {
		c.LabelJSONPaths = []string{"email", "data.email"}
	}
	if strings.TrimSpace(c.HeaderTemplate) == "" {
		c.HeaderTemplate = "Bearer {{token}}"
	}
	return c
}

// resolveBearerHeaders builds connector default Headers from bearer_env (fallback).
func resolveBearerHeaders(bearerEnv string) map[string]string {
	if strings.TrimSpace(bearerEnv) == "" {
		return nil
	}
	v := strings.TrimSpace(os.Getenv(bearerEnv))
	if v == "" {
		return nil
	}
	if !strings.HasPrefix(strings.ToLower(v), "bearer ") {
		v = "Bearer " + v
	}
	return map[string]string{"Authorization": v}
}

// uniqueSecurityScheme returns the sole security scheme name across routes, or "".
func uniqueSecurityScheme(routes []openapi.ToolRoute) string {
	seen := map[string]struct{}{}
	for _, r := range routes {
		for _, s := range r.Security {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			seen[s] = struct{}{}
		}
	}
	if len(seen) != 1 {
		return ""
	}
	for s := range seen {
		return s
	}
	return ""
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
