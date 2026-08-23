package executecallback

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/connector/httpplugin"
)

// Client POSTs tool invocations to an enterprise execution callback URL (§4.3).
type Client struct {
	URL        string
	HTTPClient *http.Client // nil → 30s timeout
}

func NewClient(url string) *Client {
	return &Client{URL: url}
}

type InvokeMeta struct {
	RunID          string
	AgentID        string
	IdempotencyKey string
	Headers        map[string]string
}

type InvokeResult struct {
	Content map[string]any
	IsError bool
}

func (c *Client) Invoke(ctx context.Context, tool string, args map[string]any, meta InvokeMeta) (InvokeResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	payload := map[string]any{
		"tool":             tool,
		"arguments":        args,
		"run_id":           meta.RunID,
		"agent_id":         meta.AgentID,
		"idempotency_key":  meta.IdempotencyKey,
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("marshal invoke body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(c.URL), bytes.NewReader(rawPayload))
	if err != nil {
		return InvokeResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range meta.Headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
	req.Header.Set(httpplugin.HeaderProtocol, httpplugin.ProtocolV0)
	if meta.RunID != "" {
		req.Header.Set(httpplugin.HeaderRunID, meta.RunID)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out, _ := parseInvokeResponse(raw)
		out.IsError = true
		return out, nil
	}

	return parseInvokeResponse(raw)
}

func parseInvokeResponse(raw []byte) (InvokeResult, error) {
	var resp struct {
		Content map[string]any `json:"content"`
		IsError bool           `json:"is_error"`
		Error   *struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return InvokeResult{Content: decodeBody(raw), IsError: true}, nil
	}

	content := resp.Content
	if content == nil {
		content = map[string]any{}
	}
	if resp.Error != nil && resp.Error.Code != "" {
		content["message"] = resp.Error.Message
	}
	if resp.IsError || (resp.Error != nil && resp.Error.Code != "") {
		return InvokeResult{Content: content, IsError: true}, nil
	}
	return InvokeResult{Content: content, IsError: false}, nil
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
	return map[string]any{"raw": string(raw)}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}
