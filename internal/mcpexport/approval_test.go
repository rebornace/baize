package mcpexport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func connectExportHTTP(
	ctx context.Context,
	endpoint string,
	token string,
	clientOpts *mcp.ClientOptions,
) (*mcp.ClientSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "baize", Version: "dev"}, clientOpts)
	transport := &mcp.StreamableClientTransport{
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Transport: exportTestHeaderRoundTripper{headers: map[string]string{
				"Authorization": "Bearer " + token,
			}},
		},
		DisableStandaloneSSE: true,
	}
	return client.Connect(ctx, transport, nil)
}

type exportTestHeaderRoundTripper struct {
	headers map[string]string
}

func (h exportTestHeaderRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(req)
}

func setupApprovalExportFixture(t *testing.T) (st store.Store, reg *tool.Registry, ids identity.Store, plaintext string, invokeCount *atomic.Int32) {
	t.Helper()
	st = store.NewMemory()
	ids = identity.NewMemoryStore()
	reg = tool.NewRegistry()
	invokeCount = &atomic.Int32{}

	st.UpsertConnector(store.Connector{ID: "c1", Type: "openapi"})
	st.UpsertTool(store.Tool{
		ConnectorID:     "c1",
		Name:            "approved_get",
		Source:          store.ToolSourceSpec,
		Enabled:         true,
		Method:          "GET",
		Description:     "needs approval",
		RequireApproval: true,
		Export:          "default",
	})

	reg.Register("approved_get", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		invokeCount.Add(1)
		return map[string]any{"ok": true}, false, nil
	})

	export := store.MCPExportIdentity{ID: "idt_export_1", Name: "Ops", Scheme: "Bearer"}
	if err := st.UpsertMCPExportIdentity(export); err != nil {
		t.Fatalf("UpsertMCPExportIdentity: %v", err)
	}
	plaintext, hash, prefix, err := mcpexport.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key := store.MCPExportKey{
		ID:           "key_1",
		Name:         "test",
		IdentityID:   export.ID,
		KeyHash:      hash,
		Prefix:       prefix,
		AllowedTools: []string{"approved_get"},
	}
	if err := st.InsertMCPExportKey(key); err != nil {
		t.Fatalf("InsertMCPExportKey: %v", err)
	}
	return st, reg, ids, plaintext, invokeCount
}

// TestSpikeStatelessElicitationDuringToolCall documents whether server-initiated
// Elicit works during tools/call on the export server's Stateless HTTP handler.
func TestSpikeStatelessElicitationDuringToolCall(t *testing.T) {
	t.Helper()
	st := store.NewMemory()
	ids := identity.NewMemoryStore()
	reg := tool.NewRegistry()
	st.UpsertConnector(store.Connector{ID: "c1", Type: "openapi"})
	st.UpsertTool(store.Tool{
		ConnectorID: "c1", Name: "spike_tool", Source: store.ToolSourceSpec,
		Enabled: true, Method: "GET", Export: "default", RequireApproval: true,
	})
	reg.Register("spike_tool", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	export := store.MCPExportIdentity{ID: "idt_1", Name: "Ops"}
	_ = st.UpsertMCPExportIdentity(export)
	pt, hash, prefix, _ := mcpexport.GenerateKey()
	_ = st.InsertMCPExportKey(store.MCPExportKey{
		ID: "k1", IdentityID: export.ID, KeyHash: hash, Prefix: prefix,
		AllowedTools: []string{"spike_tool"},
	})

	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	session, err := connectExportHTTP(ctx, srv.URL, pt, &mcp.ClientOptions{
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := mcpbridge.CallTool(ctx, session, "spike_tool", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected MRTR elicitation path to succeed on stateless export, got error: %s", toolResultText(t, result))
	}
	t.Log("spike: Stateless export + InputRequests elicitation during tools/call succeeds with ElicitationHandler")
}

func TestNeedsExportApprovalMatrix(t *testing.T) {
	base := store.Tool{Enabled: true, Source: store.ToolSourceSpec, Method: "GET"}
	if !mcpexport.NeedsExportApproval(store.Tool{RequireApproval: true}, mcpexport.ApprovalOpts{WriteHeuristicElicit: true}) {
		t.Fatal("RequireApproval should need approval")
	}
	if mcpexport.NeedsExportApproval(base, mcpexport.ApprovalOpts{WriteHeuristicElicit: false}) {
		t.Fatal("GET with heuristic off should not need approval")
	}
	if !mcpexport.NeedsExportApproval(store.Tool{Method: "POST"}, mcpexport.ApprovalOpts{WriteHeuristicElicit: true}) {
		t.Fatal("POST should need approval")
	}
	mcpWrite := store.Tool{Source: store.ToolSourceMCP, Name: "db_insert_row", Method: "GET"}
	if !mcpexport.NeedsExportApproval(mcpWrite, mcpexport.ApprovalOpts{WriteHeuristicElicit: true}) {
		t.Fatal("MCP write signal should need approval")
	}
	destructive := true
	if !mcpexport.NeedsExportApproval(store.Tool{Method: "GET", DestructiveHint: &destructive}, mcpexport.ApprovalOpts{WriteHeuristicElicit: false}) {
		t.Fatal("DestructiveHint should need approval even when write heuristic is off")
	}
}

func TestExportApprovalNoElicitationCapability(t *testing.T) {
	st, reg, ids, plaintext, invokeCount := setupApprovalExportFixture(t)
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	session, err := connectExportHTTP(ctx, srv.URL, plaintext, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := mcpbridge.CallTool(ctx, session, "approved_get", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result, got %+v", result)
	}
	text := toolResultText(t, result)
	if !strings.Contains(text, "does not support") {
		t.Fatalf("expected unsupported elicitation message, got %q", text)
	}
	if invokeCount.Load() != 0 {
		t.Fatalf("invoke count=%d want 0", invokeCount.Load())
	}
}

func TestExportApprovalAccept(t *testing.T) {
	st, reg, ids, plaintext, invokeCount := setupApprovalExportFixture(t)
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	session, err := connectExportHTTP(ctx, srv.URL, plaintext, &mcp.ClientOptions{
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept"}, nil
		},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := mcpbridge.CallTool(ctx, session, "approved_get", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %+v", result)
	}
	if invokeCount.Load() != 1 {
		t.Fatalf("invoke count=%d want 1", invokeCount.Load())
	}
}

func TestExportApprovalDecline(t *testing.T) {
	st, reg, ids, plaintext, invokeCount := setupApprovalExportFixture(t)
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	session, err := connectExportHTTP(ctx, srv.URL, plaintext, &mcp.ClientOptions{
		ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := mcpbridge.CallTool(ctx, session, "approved_get", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError || !strings.Contains(toolResultText(t, result), "declined") {
		t.Fatalf("expected declined error, got %+v", result)
	}
	if invokeCount.Load() != 0 {
		t.Fatalf("invoke count=%d want 0", invokeCount.Load())
	}
}
