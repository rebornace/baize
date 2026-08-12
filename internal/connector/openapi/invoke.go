package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// InvokeResult is the normalized result of an HTTP tool call.
type InvokeResult struct {
	Content map[string]any
	IsError bool
}

// Invoker executes loaded ToolRoutes against a base URL.
type Invoker struct {
	BaseURL    string
	Tools      []ToolRoute
	HTTPClient *http.Client
}

// Invoke looks up toolName and performs the corresponding HTTP request.
// Non-2xx responses return InvokeResult{IsError: true} without error.
func (inv *Invoker) Invoke(ctx context.Context, toolName string, args map[string]any) (InvokeResult, error) {
	route, ok := inv.find(toolName)
	if !ok {
		return InvokeResult{}, fmt.Errorf("unknown tool: %s", toolName)
	}

	url := strings.TrimRight(inv.BaseURL, "/") + route.Path
	var body io.Reader
	if route.Method == http.MethodPost || route.Method == http.MethodPut || route.Method == http.MethodPatch {
		raw, err := json.Marshal(args)
		if err != nil {
			return InvokeResult{}, fmt.Errorf("marshal args: %w", err)
		}
		body = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, route.Method, url, body)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := inv.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("read body: %w", err)
	}

	content := decodeBody(raw)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return InvokeResult{Content: content, IsError: true}, nil
	}
	return InvokeResult{Content: content, IsError: false}, nil
}

func (inv *Invoker) find(name string) (ToolRoute, bool) {
	for _, t := range inv.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return ToolRoute{}, false
}

func decodeBody(raw []byte) map[string]any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return map[string]any{}
	}
	var asMap map[string]any
	if err := json.Unmarshal(trimmed, &asMap); err == nil {
		return asMap
	}
	var asAny any
	if err := json.Unmarshal(trimmed, &asAny); err == nil {
		return map[string]any{"data": asAny}
	}
	return map[string]any{"raw": string(raw)}
}
