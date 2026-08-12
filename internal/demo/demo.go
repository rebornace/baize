package demo

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// Run starts mock-ticket + Runtime in-process and blocks on the API server.
func Run(cfg config.Config) error {
	ticketListen := cfg.Demo.TicketListen
	if ticketListen == "" {
		ticketListen = ":18080"
	}

	go func() {
		log.Printf("mock-ticket listening on %s", ticketListen)
		if err := http.ListenAndServe(ticketListen, mockticket.NewHandler()); err != nil {
			log.Printf("mock-ticket server error: %v", err)
		}
	}()

	ticketBase := localHTTPBase(ticketListen)
	if err := waitHealthy(ticketBase+"/healthz", 5*time.Second); err != nil {
		return fmt.Errorf("mock-ticket health check failed: %w", err)
	}

	srv, err := newAPIServer(cfg)
	if err != nil {
		return err
	}

	listen := cfg.Listen
	if listen == "" {
		listen = ":8080"
	}
	printCurlHints(localHTTPBase(listen), cfg.Agent.ID)
	log.Printf("baize runtime listening on %s", listen)
	return http.ListenAndServe(listen, srv.Handler())
}

// Serve starts Runtime only (no mock-ticket) and blocks on the API server.
func Serve(cfg config.Config) error {
	srv, err := newAPIServer(cfg)
	if err != nil {
		return err
	}
	listen := cfg.Listen
	if listen == "" {
		listen = ":8080"
	}
	printCurlHints(localHTTPBase(listen), cfg.Agent.ID)
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
		cfg.Run.MaxSteps = 8
	}

	apiSrv, err := newAPIServer(cfg)
	if err != nil {
		_ = ticketSrv.Close()
		t.Fatalf("new api server: %v", err)
	}

	runtimeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = ticketSrv.Close()
		t.Fatalf("listen runtime: %v", err)
	}
	runtimeURL = "http://" + runtimeLn.Addr().String()
	httpSrv := &http.Server{Handler: apiSrv.Handler()}
	go func() { _ = httpSrv.Serve(runtimeLn) }()

	if err := waitHealthy(runtimeURL+"/healthz", 5*time.Second); err != nil {
		_ = httpSrv.Close()
		_ = ticketSrv.Close()
		t.Fatalf("runtime health check failed: %v", err)
	}

	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		_ = ticketSrv.Shutdown(ctx)
	}
	return runtimeURL, ticketURL, shutdown
}

func newAPIServer(cfg config.Config) (*api.Server, error) {
	provider, err := newLLM(cfg)
	if err != nil {
		return nil, err
	}

	st := store.New()
	reg := tool.NewRegistry()

	st.UpsertAgent(store.Agent{ID: cfg.Agent.ID, System: cfg.Agent.System})

	if err := registerConnector(st, reg, cfg); err != nil {
		return nil, err
	}

	engine := &run.Engine{
		Store:    st,
		LLM:      provider,
		Tools:    reg,
		MaxSteps: cfg.Run.MaxSteps,
	}
	return api.NewServer(st, reg, engine), nil
}

func registerConnector(st *store.Store, reg *tool.Registry, cfg config.Config) error {
	typ := cfg.Connector.Type
	if typ == "" {
		typ = "openapi"
	}
	if typ != "openapi" {
		return fmt.Errorf("unsupported connector type: %s", typ)
	}
	if cfg.Connector.Spec == "" {
		return fmt.Errorf("connector.spec is required")
	}

	routes, err := openapi.LoadTools(cfg.Connector.Spec)
	if err != nil {
		return fmt.Errorf("load openapi tools: %w", err)
	}

	st.UpsertConnector(store.Connector{
		ID:      cfg.Connector.ID,
		Type:    typ,
		Spec:    cfg.Connector.Spec,
		BaseURL: cfg.Connector.BaseURL,
	})

	inv := &openapi.Invoker{BaseURL: cfg.Connector.BaseURL, Tools: routes}
	for _, route := range routes {
		route := route
		name := route.Name
		reg.RegisterSpec(llm.ToolSpec{
			Name:        route.Name,
			Description: route.Description,
			InputSchema: route.InputSchema,
		}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			res, err := inv.Invoke(ctx, name, args)
			if err != nil {
				return nil, true, err
			}
			return res.Content, res.IsError, nil
		})
	}
	return nil
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
		return llm.NewOpenAI(cfg.LLM.BaseURL, os.Getenv(env), cfg.LLM.Model), nil
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

func printCurlHints(runtimeBase, agentID string) {
	if agentID == "" {
		agentID = "ticket-agent"
	}
	log.Printf("demo ready. try:")
	log.Printf(`  curl -s -X POST %s/v0/runs -H "Content-Type: application/json" -d "{\"agent_id\":\"%s\",\"input\":\"创建一个紧急工单：VPN 挂了\"}"`, runtimeBase, agentID)
	log.Printf(`  curl -s %s/tickets`, "http://127.0.0.1:18080")
}
