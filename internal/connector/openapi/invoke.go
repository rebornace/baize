package openapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

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

	path, pathKeys, err := expandPath(route.Path, args)
	if err != nil {
		return InvokeResult{}, err
	}
	url := strings.TrimRight(inv.BaseURL, "/") + path

	var body io.Reader
	if route.Method == http.MethodPost || route.Method == http.MethodPut || route.Method == http.MethodPatch {
		bodyArgs := omitKeys(args, pathKeys)
		raw, err := json.Marshal(bodyArgs)
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

func expandPath(tmpl string, args map[string]any) (string, map[string]bool, error) {
	keys := map[string]bool{}
	var missing []string
	path := pathParamPattern.ReplaceAllStringFunc(tmpl, func(m string) string {
		name := m[1 : len(m)-1]
		keys[name] = true
		v, ok := args[name]
		if !ok || v == nil {
			missing = append(missing, name)
			return m
		}
		return fmt.Sprint(v)
	})
	if len(missing) > 0 {
		return "", nil, fmt.Errorf("missing path param: %s", strings.Join(missing, ", "))
	}
	return path, keys, nil
}

func omitKeys(args map[string]any, keys map[string]bool) map[string]any {
	if len(keys) == 0 {
		return args
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		if keys[k] {
			continue
		}
		out[k] = v
	}
	return out
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
