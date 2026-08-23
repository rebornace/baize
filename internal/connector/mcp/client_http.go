package mcp

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConnectHTTP opens a Streamable HTTP MCP session.
func ConnectHTTP(ctx context.Context, endpoint string, headers map[string]string) (*mcp.ClientSession, error) {
	client := mcp.NewClient(baizeClientInfo, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Transport: headerRoundTripper{headers: headers},
		},
		DisableStandaloneSSE: true,
	}
	return client.Connect(ctx, transport, nil)
}

type headerRoundTripper struct {
	next    http.RoundTripper
	headers map[string]string
}

func (h headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}
	next := h.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}
