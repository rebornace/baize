package openapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
	"github.com/rebornace/baize/internal/connector/openapi"
)

func TestLoadToolsFromSpec(t *testing.T) {
	tools, err := openapi.LoadTools("../../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
	}
	if !names["create_ticket"] || !names["list_tickets"] {
		t.Fatalf("tools=%v", names)
	}
	var create openapi.ToolRoute
	for _, tl := range tools {
		if tl.Name == "create_ticket" {
			create = tl
		}
	}
	if create.Method != http.MethodPost || create.Path != "/tickets" {
		t.Fatalf("route=%+v", create)
	}
	if create.InputSchema == nil {
		t.Fatal("create_ticket InputSchema is nil")
	}
	props, _ := create.InputSchema["properties"].(map[string]any)
	if props == nil {
		t.Fatalf("properties missing: %+v", create.InputSchema)
	}
	title, _ := props["title"].(map[string]any)
	if title == nil || title["type"] != "string" {
		t.Fatalf("title schema=%v", title)
	}
	if _, ok := props["priority"]; !ok {
		t.Fatalf("priority missing: %+v", props)
	}
}

func TestInvokerCreateTicket(t *testing.T) {
	srv := httptest.NewServer(mockticket.NewHandler())
	t.Cleanup(srv.Close)

	tools, err := openapi.LoadTools("../../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	inv := &openapi.Invoker{BaseURL: srv.URL, Tools: tools}
	res, err := inv.Invoke(context.Background(), "create_ticket", map[string]any{
		"title":    "VPN 挂了",
		"priority": "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if res.Content["id"] == nil || res.Content["title"] != "VPN 挂了" {
		t.Fatalf("content=%v", res.Content)
	}
}

func TestInvokerNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	inv := &openapi.Invoker{
		BaseURL: srv.URL,
		Tools: []openapi.ToolRoute{{
			Name:   "create_ticket",
			Method: http.MethodPost,
			Path:   "/tickets",
		}},
	}
	res, err := inv.Invoke(context.Background(), "create_ticket", map[string]any{"title": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError, got %+v", res)
	}
}
