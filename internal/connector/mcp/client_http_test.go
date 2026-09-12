package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/store"
)

type httpEchoArgs struct {
	Message string `json:"message"`
}

type httpEchoOutput struct {
	Message string `json:"message"`
}

func TestHTTPDiscoverAndCallTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "http-mock", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "echo",
		Description: "echo",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args httpEchoArgs) (*mcp.CallToolResult, httpEchoOutput, error) {
		return nil, httpEchoOutput{Message: args.Message}, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	ctx := context.Background()
	session, err := mcpbridge.ConnectHTTP(ctx, httpServer.URL, nil)
	if err != nil {
		t.Fatalf("ConnectHTTP: %v", err)
	}
	defer session.Close()

	tools, err := mcpbridge.DiscoverTools(ctx, session, "http-test")
	if err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools=%+v", tools)
	}
	if tools[0].Source != store.ToolSourceMCP {
		t.Fatalf("source=%q", tools[0].Source)
	}

	result, err := mcpbridge.CallTool(ctx, session, "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %+v", result)
	}
}
