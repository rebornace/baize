package httpplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client // nil → 30s timeout client
}

func NewClient(baseURL string) *Client {
	return &Client{BaseURL: baseURL}
}

func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+"/healthz", nil)
	if err != nil {
		return ErrInvalidPlugin
	}
	c.setProtocolHeader(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return ErrInvalidPlugin
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ErrInvalidPlugin
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return ErrInvalidPlugin
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Status != "ok" {
		return ErrInvalidPlugin
	}
	return nil
}

func (c *Client) ListTools(ctx context.Context) ([]ToolDesc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL()+"/v0/tools", nil)
	if err != nil {
		return nil, ErrInvalidPlugin
	}
	c.setProtocolHeader(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, ErrInvalidPlugin
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrInvalidPlugin
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrInvalidPlugin
	}

	var body struct {
		Tools []ToolDesc `json:"tools"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, ErrInvalidPlugin
	}
	if len(body.Tools) == 0 {
		return nil, ErrInvalidPlugin
	}
	for _, tool := range body.Tools {
		if strings.TrimSpace(tool.Name) == "" {
			return nil, ErrInvalidPlugin
		}
	}
	return body.Tools, nil
}

func (c *Client) Invoke(ctx context.Context, name string, args map[string]any, meta InvokeMeta) (InvokeResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	ctxMap := map[string]any{
		"run_id":   meta.RunID,
		"agent_id": meta.AgentID,
	}
	if strings.TrimSpace(meta.CallbackEventURL) != "" {
		ctxMap["callback_urls"] = map[string]any{
			"event": meta.CallbackEventURL,
		}
	}
	payload := map[string]any{
		"arguments": args,
		"context":   ctxMap,
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("marshal invoke body: %w", err)
	}

	url := fmt.Sprintf("%s/v0/tools/%s/invoke", c.baseURL(), name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawPayload))
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
	c.setProtocolHeader(req)
	if meta.RunID != "" {
		req.Header.Set(HeaderRunID, meta.RunID)
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

func (c *Client) baseURL() string {
	return strings.TrimRight(c.BaseURL, "/")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) setProtocolHeader(req *http.Request) {
	req.Header.Set(HeaderProtocol, ProtocolV0)
}
