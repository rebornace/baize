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
	"time"
)

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

// defaultHTTPClient bounds OpenAPI tool calls when Invoker.HTTPClient is nil.
// Engine also applies a per-invoke context timeout; this is a backstop.
var defaultHTTPClient = &http.Client{Timeout: 60 * time.Second}

// InvokeResult is the normalized result of an HTTP tool call.
type InvokeResult struct {
	Content map[string]any
	IsError bool
}

// Invoker executes loaded ToolRoutes against a base URL.
type Invoker struct {
	BaseURL    string
	Tools      []ToolRoute
	Headers    map[string]string
	HTTPClient *http.Client
}

// Invoke looks up toolName and performs the corresponding HTTP request.
// Non-2xx responses return InvokeResult{IsError: true} without error.
func (inv *Invoker) Invoke(ctx context.Context, toolName string, args map[string]any) (InvokeResult, error) {
	return inv.invoke(ctx, toolName, args, nil)
}

// InvokeWithHeaders is like Invoke but merges overlay headers over inv.Headers
// for this call only (same key overlays; inv.Headers is not mutated).
func (inv *Invoker) InvokeWithHeaders(ctx context.Context, toolName string, args map[string]any, overlay map[string]string) (InvokeResult, error) {
	return inv.invoke(ctx, toolName, args, overlay)
}

func (inv *Invoker) invoke(ctx context.Context, toolName string, args map[string]any, overlay map[string]string) (InvokeResult, error) {
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
		payload, err := marshalRequestBody(route.BodyKind, bodyArgs)
		if err != nil {
			return InvokeResult{}, fmt.Errorf("marshal args: %w", err)
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, route.Method, url, body)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range mergeHeaders(inv.Headers, overlay) {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	client := inv.HTTPClient
	if client == nil {
		client = defaultHTTPClient
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
	// Some gateways return HTTP 200 with a business auth failure body.
	if looksLikeAuthFailure(content) {
		return InvokeResult{Content: content, IsError: true}, nil
	}
	return InvokeResult{Content: content, IsError: false}, nil
}

func looksLikeAuthFailure(content map[string]any) bool {
	if content == nil {
		return false
	}
	switch v := content["code"].(type) {
	case float64:
		if int(v) == 401 || int(v) == 403 {
			return true
		}
	case int:
		if v == 401 || v == 403 {
			return true
		}
	case string:
		if v == "401" || v == "403" || strings.EqualFold(v, "unauthorized") || strings.EqualFold(v, "forbidden") {
			return true
		}
	}
	if msg, ok := content["message"].(string); ok {
		m := strings.ToLower(strings.TrimSpace(msg))
		if m == "unauthorized" || m == "forbidden" || strings.Contains(m, "unauthorized") {
			return true
		}
	}
	return false
}

func mergeHeaders(base, overlay map[string]string) map[string]string {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
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

func marshalRequestBody(bodyKind string, args map[string]any) ([]byte, error) {
	switch bodyKind {
	case "array":
		items, ok := args[bodyArrayKey]
		if !ok {
			return nil, fmt.Errorf("missing body array field %q", bodyArrayKey)
		}
		return json.Marshal(items)
	case "value":
		v, ok := args["value"]
		if !ok {
			return nil, fmt.Errorf("missing body field %q", "value")
		}
		return json.Marshal(v)
	default:
		return json.Marshal(args)
	}
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
