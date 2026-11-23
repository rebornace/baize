package mcp

import (
	"context"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ConnectHTTP opens a Streamable HTTP MCP session.
// hc is optional: nil uses a client with only header injection (prior behavior).
// When non-nil, header injection wraps hc.Transport (or http.DefaultTransport)
// and preserves hc.Timeout.
func ConnectHTTP(ctx context.Context, endpoint string, headers map[string]string, hc *http.Client) (*mcp.ClientSession, error) {
	client := mcp.NewClient(baizeClientInfo, nil)
	var next http.RoundTripper
	var timeout time.Duration
	if hc != nil {
		next = hc.Transport
		timeout = hc.Timeout
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint: endpoint,
		HTTPClient: &http.Client{
			Timeout:   timeout,
			Transport: headerRoundTripper{next: next, headers: headers},
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
