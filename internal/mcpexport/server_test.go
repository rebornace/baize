package mcpexport_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func setupExportFixture(t *testing.T) (st store.Store, reg *tool.Registry, ids identity.Store, plaintext, keyID string) {
	t.Helper()
	st = store.NewMemory()
	ids = identity.NewMemoryStore()
	reg = tool.NewRegistry()

	st.UpsertConnector(store.Connector{ID: "c1", Type: "openapi"})
	st.UpsertTool(store.Tool{
		ConnectorID: "c1",
		Name:        "get_status",
		Source:      store.ToolSourceSpec,
		Enabled:     true,
		Method:      "GET",
		Description: "status",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
		},
		Export: "default",
	})
	st.UpsertTool(store.Tool{
		ConnectorID: "c1",
		Name:        "create_item",
		Source:      store.ToolSourceSpec,
		Enabled:     true,
		Method:      "POST",
		Description: "create",
		Export:      "default",
	})

	reg.Register("get_status", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true, "id": args["id"]}, false, nil
	})
	reg.Register("create_item", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"created": true}, false, nil
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
		ID:         "key_1",
		Name:       "test",
		IdentityID: export.ID,
		KeyHash:    hash,
		Prefix:     prefix,
	}
	if err := st.InsertMCPExportKey(key); err != nil {
		t.Fatalf("InsertMCPExportKey: %v", err)
	}
	return st, reg, ids, plaintext, key.ID
}

func TestHTTPHandlerListAndCall(t *testing.T) {
	st, reg, ids, plaintext, _ := setupExportFixture(t)
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store:      st,
		Registry:   reg,
		Identities: ids,
		Enabled:    true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + plaintext,
	})
	if err != nil {
		t.Fatalf("ConnectHTTP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	listed, err := mcpbridge.ListTools(ctx, session)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tl := range listed.Tools {
		names[tl.Name] = true
	}
	if !names["get_status"] {
		t.Fatalf("expected get_status in list, got %#v", names)
	}
	if names["create_item"] {
		t.Fatalf("POST default tool must not be listed: %#v", names)
	}

	result, err := mcpbridge.CallTool(ctx, session, "get_status", map[string]any{"id": "42"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %+v", result)
	}
	got := toolResultText(t, result)
	if !strings.Contains(got, `"ok":true`) || !strings.Contains(got, `"id":"42"`) {
		t.Fatalf("result text=%s structured=%v", got, result.StructuredContent)
	}
}

func toolResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result.StructuredContent != nil {
		b, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatalf("marshal structured: %v", err)
		}
		return string(b)
	}
	var parts []string
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, "")
}

func TestHTTPHandlerUnauthorized(t *testing.T) {
	st, reg, ids, plaintext, keyID := setupExportFixture(t)
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	if _, err := mcpbridge.ConnectHTTP(ctx, srv.URL, nil); err == nil {
		t.Fatal("expected connect without key to fail")
	}
	if _, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer not-a-real-key",
	}); err == nil {
		t.Fatal("expected connect with bad key to fail")
	}

	if err := st.RevokeMCPExportKey(keyID); err != nil {
		t.Fatalf("RevokeMCPExportKey: %v", err)
	}
	if _, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + plaintext,
	}); err == nil {
		t.Fatal("expected connect with revoked key to fail")
	}

	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", resp.StatusCode)
	}
}

func TestHTTPHandlerDisabled(t *testing.T) {
	st, reg, ids, plaintext, _ := setupExportFixture(t)
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: false,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, _ := http.NewRequest(http.MethodPost, srv.URL, nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want 503", resp.StatusCode)
	}
}
