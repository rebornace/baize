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

	export := store.MCPExportIdentity{
		ID: "idt_export_1", Name: "Ops", Scheme: "Bearer",
		Headers: map[string]string{"Authorization": "Bearer test-token"},
	}
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
		AllowedTools: []string{"get_status"},
	}
	if err := st.InsertMCPExportKey(key); err != nil {
		t.Fatalf("InsertMCPExportKey: %v", err)
	}
	return st, reg, ids, plaintext, key.ID
}

func TestExportWorkflowPromptRegistered(t *testing.T) {
	st, reg, ids, plaintext, _ := setupExportFixture(t)
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + plaintext,
	}, nil)
	if err != nil {
		t.Fatalf("ConnectHTTP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	listed, err := session.ListPrompts(ctx, &mcp.ListPromptsParams{})
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	found := false
	for _, p := range listed.Prompts {
		if p.Name == "baize_export_workflow" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("prompts=%v want baize_export_workflow", listed.Prompts)
	}

	got, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "baize_export_workflow"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if len(got.Messages) == 0 {
		t.Fatal("expected prompt messages")
	}
	if tc, ok := got.Messages[0].Content.(*mcp.TextContent); ok {
		if tc.Text != mcpexport.ExportWorkflowSkillMarkdown() {
			t.Fatal("prompt body should match ExportWorkflowSkillMarkdown")
		}
		if !strings.Contains(strings.ToLower(tc.Text), "decline") {
			t.Fatalf("prompt body missing decline theme: %q", tc.Text)
		}
	} else {
		t.Fatalf("expected text content, got %T", got.Messages[0].Content)
	}
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
	}, nil)
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

	if _, err := mcpbridge.ConnectHTTP(ctx, srv.URL, nil, nil); err == nil {
		t.Fatal("expected connect without key to fail")
	}
	if _, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer not-a-real-key",
	}, nil); err == nil {
		t.Fatal("expected connect with bad key to fail")
	}

	if err := st.RevokeMCPExportKey(keyID); err != nil {
		t.Fatalf("RevokeMCPExportKey: %v", err)
	}
	if _, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + plaintext,
	}, nil); err == nil {
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

func TestHTTPHandlerEmptyAllowlistExportsNothing(t *testing.T) {
	st, reg, ids, plaintext, _ := setupExportFixture(t)
	keys, err := st.ListMCPExportKeys("")
	if err != nil || len(keys) != 1 {
		t.Fatalf("keys=%+v err=%v", keys, err)
	}
	k := keys[0]
	k.AllowedTools = nil
	if err := st.UpdateMCPExportKey(k); err != nil {
		t.Fatalf("UpdateMCPExportKey: %v", err)
	}
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + plaintext,
	}, nil)
	if err != nil {
		t.Fatalf("ConnectHTTP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	listed, err := mcpbridge.ListTools(ctx, session)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(listed.Tools) != 0 {
		t.Fatalf("expected no tools, got %#v", listed.Tools)
	}
}

func TestHTTPHandlerPerKeyAllowlistsDiffer(t *testing.T) {
	st := store.NewMemory()
	ids := identity.NewMemoryStore()
	reg := tool.NewRegistry()
	st.UpsertConnector(store.Connector{ID: "c1", Type: "openapi"})
	for _, name := range []string{"get_status", "get_health"} {
		st.UpsertTool(store.Tool{
			ConnectorID: "c1", Name: name, Source: store.ToolSourceSpec,
			Enabled: true, Method: "GET", Export: "default",
		})
		reg.Register(name, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			return map[string]any{"tool": name}, false, nil
		})
	}
	export := store.MCPExportIdentity{ID: "idt_1", Name: "Ops"}
	_ = st.UpsertMCPExportIdentity(export)

	mkKey := func(id string, allowed []string) (plaintext string) {
		t.Helper()
		pt, hash, prefix, err := mcpexport.GenerateKey()
		if err != nil {
			t.Fatal(err)
		}
		if err := st.InsertMCPExportKey(store.MCPExportKey{
			ID: id, Name: id, IdentityID: export.ID, KeyHash: hash, Prefix: prefix,
			AllowedTools: allowed,
		}); err != nil {
			t.Fatal(err)
		}
		return pt
	}
	keyA := mkKey("ka", []string{"get_status"})
	keyB := mkKey("kb", []string{"get_health"})

	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()

	listFor := func(token string) map[string]bool {
		t.Helper()
		session, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
			"Authorization": "Bearer " + token,
		}, nil)
		if err != nil {
			t.Fatalf("ConnectHTTP: %v", err)
		}
		defer session.Close()
		listed, err := mcpbridge.ListTools(ctx, session)
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}
		names := map[string]bool{}
		for _, tl := range listed.Tools {
			names[tl.Name] = true
		}
		return names
	}
	nA := listFor(keyA)
	if !nA["get_status"] || nA["get_health"] {
		t.Fatalf("key A list=%v", nA)
	}
	nB := listFor(keyB)
	if nB["get_status"] || !nB["get_health"] {
		t.Fatalf("key B list=%v", nB)
	}
}

func TestHTTPHandlerCallDeniedWhenNotOnAllowlist(t *testing.T) {
	st, reg, ids, plaintext, _ := setupExportFixture(t)
	st.UpsertTool(store.Tool{
		ConnectorID: "c1", Name: "get_health", Source: store.ToolSourceSpec,
		Enabled: true, Method: "GET", Export: "default",
	})
	reg.Register("get_health", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"healthy": true}, false, nil
	})
	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + plaintext,
	}, nil)
	if err != nil {
		t.Fatalf("ConnectHTTP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := mcpbridge.CallTool(ctx, session, "get_health", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError || !strings.Contains(toolResultText(t, result), "tool not exported") {
		t.Fatalf("expected tool not exported, got %+v", result)
	}
}

func TestHTTPHandlerCallRechecksAllowlistAtInvoke(t *testing.T) {
	st, reg, ids, plaintext, keyID := setupExportFixture(t)
	st.UpsertTool(store.Tool{
		ConnectorID: "c1", Name: "extra_get", Source: store.ToolSourceSpec,
		Enabled: true, Method: "GET", Export: "default",
	})
	reg.Register("extra_get", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	k, err := st.GetMCPExportKey(keyID)
	if err != nil {
		t.Fatal(err)
	}
	k.AllowedTools = []string{"get_status", "extra_get"}
	if err := st.UpdateMCPExportKey(k); err != nil {
		t.Fatal(err)
	}

	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + plaintext,
	}, nil)
	if err != nil {
		t.Fatalf("ConnectHTTP: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	listed, err := mcpbridge.ListTools(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 2 {
		t.Fatalf("expected 2 tools listed, got %d", len(listed.Tools))
	}

	k.AllowedTools = []string{"get_status"}
	if err := st.UpdateMCPExportKey(k); err != nil {
		t.Fatal(err)
	}
	result, err := mcpbridge.CallTool(ctx, session, "extra_get", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !result.IsError || !strings.Contains(toolResultText(t, result), "tool not exported") {
		t.Fatalf("expected denied after allowlist shrink, got %+v", result)
	}
}

func TestHTTPHandlerForceDenyHiddenDespiteAllowlist(t *testing.T) {
	st := store.NewMemory()
	ids := identity.NewMemoryStore()
	reg := tool.NewRegistry()
	st.UpsertConnector(store.Connector{ID: "c1", Type: "openapi"})
	st.UpsertTool(store.Tool{
		ConnectorID: "c1", Name: "secret_get", Source: store.ToolSourceSpec,
		Enabled: true, Method: "GET", Export: "force_deny",
	})
	reg.Register("secret_get", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	})
	export := store.MCPExportIdentity{ID: "idt_1", Name: "Ops"}
	_ = st.UpsertMCPExportIdentity(export)
	pt, hash, prefix, _ := mcpexport.GenerateKey()
	_ = st.InsertMCPExportKey(store.MCPExportKey{
		ID: "k1", Name: "k1", IdentityID: export.ID, KeyHash: hash, Prefix: prefix,
		AllowedTools: []string{"secret_get"},
	})

	handler := mcpexport.NewHTTPHandler(mcpexport.ServerDeps{
		Store: st, Registry: reg, Identities: ids, Enabled: true,
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, srv.URL, map[string]string{
		"Authorization": "Bearer " + pt,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	listed, err := mcpbridge.ListTools(ctx, session)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 0 {
		t.Fatalf("force_deny must hide tool even when allowlisted: %#v", listed.Tools)
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
