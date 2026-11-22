// Package mobileagent is the MOBILE-RT thin on-device orchestration surface:
// memory session, OpenAI-compatible chat, optional tool loop (get_time /
// http_get / OpenAPI connectors, read-only by default with opt-in writes) and
// foreground HITL for every network tool. It does not import cmd/baize,
// channels, MCP, Postgres, or the full run Engine.
//
// Bind via the public package github.com/rebornace/baize/mobileagent (gomobile
// cannot bind internal/).
package mobileagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	maxToolRounds      = 8
	httpGetMaxBytes    = 256 << 10
	httpGetTimeout     = 15 * time.Second
	httpGetMaxRedirect = 3
)

// Agent holds in-process conversation state and LLM config.
type Agent struct {
	mu       sync.Mutex
	baseURL  string
	apiKey   string
	model    string
	msgs     []chatMessage
	client   *http.Client // LLM
	pending  *pendingCall
	readOnly *readonlyAPI
	// stream is the active asynchronous turn started by StartStream.
	stream *streamState
	// allowPrivateNetworks permits http_get / FetchRemoteConfig / spec_url to
	// reach loopback, private and link-local addresses. Off by default.
	allowPrivateNetworks bool
	// httpClients caches the guarded clients per policy. [0] = public only,
	// [1] = allow private. Guarded clients are immutable after creation.
	httpClients [2]*http.Client
	// fetchHits counts successful http_get network attempts (tests).
	fetchHits int
	// remoteHits counts successful readonly-connector GET attempts (tests).
	remoteHits int
}

type pendingCall struct {
	CallID   string
	Name     string
	ArgsJSON string
	Remote   *remoteTool
}

// New returns an unconfigured Agent.
func New() *Agent {
	return &Agent{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func limitRedirects(req *http.Request, via []*http.Request) error {
	if len(via) >= httpGetMaxRedirect {
		return fmt.Errorf("stopped after %d redirects", httpGetMaxRedirect)
	}
	return nil
}

var defaultSession = New()

// Configure sets API base/key/model and clears messages + pending HITL.
func (a *Agent) Configure(apiBase, apiKey, model string) error {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if apiBase == "" {
		return fmt.Errorf("mobileagent: apiBase is required")
	}
	if model == "" {
		return fmt.Errorf("mobileagent: model is required")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.baseURL = apiBase
	a.apiKey = apiKey
	a.model = model
	a.msgs = nil
	a.pending = nil
	a.fetchHits = 0
	return nil
}

// Configure configures the default session.
func Configure(apiBase, apiKey, model string) error {
	return defaultSession.Configure(apiBase, apiKey, model)
}

// SetAllowPrivateNetworks controls whether http_get, FetchRemoteConfig,
// spec_url and connector calls may reach loopback, private, link-local and
// reserved addresses. Off by default.
func (a *Agent) SetAllowPrivateNetworks(allow bool) {
	a.mu.Lock()
	a.allowPrivateNetworks = allow
	a.mu.Unlock()
}

// SetAllowPrivateNetworks on the default session.
func SetAllowPrivateNetworks(allow bool) { defaultSession.SetAllowPrivateNetworks(allow) }

// AllowPrivateNetworks reports the session network policy.
func (a *Agent) AllowPrivateNetworks() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.allowPrivateNetworks
}

// AllowPrivateNetworks on the default session.
func AllowPrivateNetworks() bool { return defaultSession.AllowPrivateNetworks() }

// httpClientFor returns a cached guarded client. The client is immutable, so
// sharing it across calls is safe.
func (a *Agent) httpClientFor(allowPrivate bool) *http.Client {
	idx := 0
	if allowPrivate {
		idx = 1
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.httpClients[idx] == nil {
		a.httpClients[idx] = guardedHTTPClient(allowPrivate)
	}
	return a.httpClients[idx]
}

func (a *Agent) httpGetClient() *http.Client {
	a.mu.Lock()
	allow := a.allowPrivateNetworks
	a.mu.Unlock()
	return a.httpClientFor(allow)
}

// Reset clears messages and pending HITL without changing API config.
func (a *Agent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.msgs = nil
	a.pending = nil
}

// Reset clears the default session history.
func Reset() { defaultSession.Reset() }

// ChatOnce is the M0 no-tools shortcut.
func (a *Agent) ChatOnce(userText string) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return "", fmt.Errorf("mobileagent: empty user text")
	}
	a.mu.Lock()
	if a.pending != nil {
		a.mu.Unlock()
		return "", fmt.Errorf("mobileagent: pending approval; Approve or Deny first")
	}
	base, key, model, client := a.baseURL, a.apiKey, a.model, a.client
	history := append([]chatMessage(nil), a.msgs...)
	a.mu.Unlock()
	if base == "" || model == "" {
		return "", fmt.Errorf("mobileagent: call Configure first")
	}
	history = append(history, chatMessage{Role: "user", Content: userText})
	asst, err := postChat(context.Background(), client, base, key, model, history, nil)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(asst.Content)
	if text == "" {
		return "", fmt.Errorf("mobileagent: empty assistant content")
	}
	a.mu.Lock()
	a.msgs = append(a.msgs, chatMessage{Role: "user", Content: userText})
	a.msgs = append(a.msgs, chatMessage{Role: "assistant", Content: text})
	a.mu.Unlock()
	return text, nil
}

// ChatOnce on the default session.
func ChatOnce(userText string) (string, error) { return defaultSession.ChatOnce(userText) }

// Send runs a user turn with tools until done or needs_approval. Returns JSON.
func (a *Agent) Send(userText string) string {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return errJSON("empty user text")
	}
	a.mu.Lock()
	if a.pending != nil {
		a.mu.Unlock()
		return errJSON("pending approval; Approve or Deny first")
	}
	if a.stream != nil && a.stream.active() {
		a.mu.Unlock()
		return errJSON("stream in progress; PollStream or CancelStream first")
	}
	if a.baseURL == "" || a.model == "" {
		a.mu.Unlock()
		return errJSON("call Configure first")
	}
	a.msgs = append(a.msgs, chatMessage{Role: "user", Content: userText})
	a.mu.Unlock()
	return a.continueLoop(context.Background(), nil)
}

// Send on the default session.
func Send(userText string) string { return defaultSession.Send(userText) }

// Approve executes the pending network tool (http_get or a connector call,
// including writes) and continues.
func (a *Agent) Approve(callID string) string {
	callID = strings.TrimSpace(callID)
	a.mu.Lock()
	p := a.pending
	if p == nil {
		a.mu.Unlock()
		return errJSON("no pending approval")
	}
	if callID != "" && callID != p.CallID {
		a.mu.Unlock()
		return errJSON("call_id mismatch")
	}
	_, argsJSON, id, remote := p.Name, p.ArgsJSON, p.CallID, p.Remote
	a.pending = nil
	a.mu.Unlock()

	var content string
	var err error
	if remote != nil {
		content, err = a.execRemoteTool(remote, argsJSON)
	} else {
		content, err = a.execHTTPGet(a.httpGetClient(), argsJSON)
	}
	if err != nil {
		content = fmt.Sprintf(`{"error":%q}`, err.Error())
	} else {
		a.mu.Lock()
		if remote != nil {
			a.remoteHits++
		} else {
			a.fetchHits++
		}
		a.mu.Unlock()
	}
	a.mu.Lock()
	a.msgs = append(a.msgs, chatMessage{Role: "tool", ToolCallID: id, Content: content})
	a.mu.Unlock()
	return a.continueLoop(context.Background(), nil)
}

// Approve on the default session.
func Approve(callID string) string { return defaultSession.Approve(callID) }

// Deny rejects a pending network tool without network I/O and continues.
func (a *Agent) Deny(callID string) string {
	callID = strings.TrimSpace(callID)
	a.mu.Lock()
	p := a.pending
	if p == nil {
		a.mu.Unlock()
		return errJSON("no pending approval")
	}
	if callID != "" && callID != p.CallID {
		a.mu.Unlock()
		return errJSON("call_id mismatch")
	}
	id := p.CallID
	a.pending = nil
	a.msgs = append(a.msgs, chatMessage{
		Role:       "tool",
		ToolCallID: id,
		Content:    `{"denied_by_user":true}`,
	})
	a.mu.Unlock()
	return a.continueLoop(context.Background(), nil)
}

// Deny on the default session.
func Deny(callID string) string { return defaultSession.Deny(callID) }

// PendingApproval returns the current HITL pause, or {"status":"none"}.
// Useful when switching between sessions: each Agent keeps its own pending
// call, and the UI must restore the approval card for the selected one.
func (a *Agent) PendingApproval() string {
	a.mu.Lock()
	p := a.pending
	a.mu.Unlock()
	if p == nil {
		return `{"status":"none"}`
	}
	if p.Remote != nil {
		return needsApprovalJSON(p.CallID, p.Name, p.ArgsJSON, p.Remote.summary(p.ArgsJSON), p.Remote.method())
	}
	return needsApprovalJSON(p.CallID, p.Name, p.ArgsJSON, summarizeHTTPGetArgs(p.ArgsJSON), http.MethodGet)
}

// PendingApproval on the default session.
func PendingApproval() string { return defaultSession.PendingApproval() }

// NewSession returns a new Agent with the same LLM config, network policy and
// connector tools, but no messages, stream or pending approval.
func (a *Agent) NewSession() *Agent {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := New()
	b.baseURL = a.baseURL
	b.apiKey = a.apiKey
	b.model = a.model
	b.allowPrivateNetworks = a.allowPrivateNetworks
	b.readOnly = a.readOnly
	return b
}

// NewSession on the default session.
func NewSession() *Agent { return defaultSession.NewSession() }

// MessageCount returns stored message count.
func (a *Agent) MessageCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.msgs)
}

// MessageCount for the default session.
func MessageCount() int { return defaultSession.MessageCount() }

// FetchHits returns how many http_get network calls succeeded (tests).
func (a *Agent) FetchHits() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.fetchHits
}

// RemoteHits returns successful readonly-connector requests (tests).
func (a *Agent) RemoteHits() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.remoteHits
}

func (a *Agent) continueLoop(ctx context.Context, emit func(string)) string {
	for round := 0; round < maxToolRounds; round++ {
		a.mu.Lock()
		base, key, model, client := a.baseURL, a.apiKey, a.model, a.client
		history := append([]chatMessage(nil), a.msgs...)
		tools := a.toolSpecsLocked()
		a.mu.Unlock()

		var asst chatMessage
		var err error
		if emit != nil {
			asst, err = postChatStream(ctx, client, base, key, model, history, tools, emit)
		} else {
			asst, err = postChat(ctx, client, base, key, model, history, tools)
		}
		if err != nil {
			return errJSON(err.Error())
		}

		if len(asst.ToolCalls) == 0 {
			text := strings.TrimSpace(asst.Content)
			if text == "" {
				return errJSON("empty assistant content")
			}
			a.mu.Lock()
			a.msgs = append(a.msgs, chatMessage{Role: "assistant", Content: text})
			a.mu.Unlock()
			return doneJSON(text)
		}

		// Persist assistant tool_calls message once.
		a.mu.Lock()
		a.msgs = append(a.msgs, asst)
		a.mu.Unlock()

		for _, tc := range asst.ToolCalls {
			name := tc.Function.Name
			args := tc.Function.Arguments
			id := tc.ID
			if id == "" {
				id = fmt.Sprintf("call_%d", time.Now().UnixNano())
			}
			switch name {
			case "get_time":
				result := time.Now().Format(time.RFC3339)
				a.mu.Lock()
				a.msgs = append(a.msgs, chatMessage{Role: "tool", ToolCallID: id, Content: result})
				a.mu.Unlock()
			case "http_get":
				summary := summarizeHTTPGetArgs(args)
				a.mu.Lock()
				a.pending = &pendingCall{CallID: id, Name: name, ArgsJSON: args}
				a.mu.Unlock()
				return needsApprovalJSON(id, name, args, summary, http.MethodGet)
			default:
				a.mu.Lock()
				remote := a.remoteToolLocked(name)
				if remote != nil {
					a.pending = &pendingCall{CallID: id, Name: name, ArgsJSON: args, Remote: remote}
					a.mu.Unlock()
					return needsApprovalJSON(id, name, args, remote.summary(args), remote.method())
				}
				a.msgs = append(a.msgs, chatMessage{
					Role:       "tool",
					ToolCallID: id,
					Content:    fmt.Sprintf(`{"error":"unknown tool %q"}`, name),
				})
				a.mu.Unlock()
			}
		}
	}
	return errJSON("tool round limit exceeded")
}

func (a *Agent) execHTTPGet(client *http.Client, argsJSON string) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid args: %w", err)
	}
	rawURL := strings.TrimSpace(args.URL)
	if rawURL == "" {
		return "", fmt.Errorf("url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("only http/https allowed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpGetTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", redactNetworkError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, httpGetMaxBytes+1))
	if err != nil {
		return "", err
	}
	truncated := false
	if len(body) > httpGetMaxBytes {
		body = body[:httpGetMaxBytes]
		truncated = true
	}
	out, _ := json.Marshal(map[string]any{
		"status":    resp.StatusCode,
		"truncated": truncated,
		"body":      string(body),
		"final_url": resp.Request.URL.String(),
	})
	return string(out), nil
}

func summarizeHTTPGetArgs(argsJSON string) string {
	var args struct {
		URL string `json:"url"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &args)
	u := strings.TrimSpace(args.URL)
	if u == "" {
		return "http_get"
	}
	return "GET " + truncate(u, 120)
}

// --- wire types ---

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Tools    []toolSpec    `json:"tools,omitempty"`
	Stream   bool          `json:"stream,omitempty"`
}

type toolSpec struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func builtinToolSpecs() []toolSpec {
	var timeTool, httpTool toolSpec
	timeTool.Type = "function"
	timeTool.Function.Name = "get_time"
	timeTool.Function.Description = "Return the device local time as RFC3339."
	timeTool.Function.Parameters = map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	httpTool.Type = "function"
	httpTool.Function.Name = "http_get"
	httpTool.Function.Description = "HTTP GET a URL (read-only). Requires user approval before the request is sent."
	httpTool.Function.Parameters = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{"type": "string", "description": "http or https URL"},
		},
		"required": []string{"url"},
	}
	return []toolSpec{timeTool, httpTool}
}

func (a *Agent) toolSpecsLocked() []toolSpec {
	specs := builtinToolSpecs()
	if a.readOnly == nil {
		return specs
	}
	for _, tool := range a.readOnly.tools {
		specs = append(specs, tool.spec())
	}
	return specs
}

func postChat(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []chatMessage, tools []toolSpec) (chatMessage, error) {
	reqBody := chatRequest{Model: model, Messages: messages}
	reqBody.Tools = tools
	body, err := json.Marshal(reqBody)
	if err != nil {
		return chatMessage{}, err
	}
	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return chatMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return chatMessage{}, fmt.Errorf("mobileagent: llm request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return chatMessage{}, err
	}
	return decodeChatResponse(raw, resp.StatusCode)
}

func decodeChatResponse(raw []byte, status int) (chatMessage, error) {
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return chatMessage{}, fmt.Errorf("mobileagent: decode response (HTTP %d): %w", status, err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return chatMessage{}, fmt.Errorf("mobileagent: llm error: %s", parsed.Error.Message)
	}
	if status < 200 || status >= 300 {
		return chatMessage{}, fmt.Errorf("mobileagent: llm HTTP %d: %s", status, truncate(string(raw), 200))
	}
	if len(parsed.Choices) == 0 {
		return chatMessage{}, fmt.Errorf("mobileagent: empty choices")
	}
	msg := parsed.Choices[0].Message
	for i := range msg.ToolCalls {
		if msg.ToolCalls[i].Type == "" {
			msg.ToolCalls[i].Type = "function"
		}
	}
	return msg, nil
}

func doneJSON(text string) string {
	b, _ := json.Marshal(map[string]any{"status": "done", "text": text})
	return string(b)
}

func needsApprovalJSON(callID, name, argsJSON, summary, method string) string {
	payload := map[string]any{
		"status":    "needs_approval",
		"call_id":   callID,
		"name":      name,
		"args_json": argsJSON,
		"summary":   summary,
	}
	if method != "" {
		payload["method"] = method
		if method != http.MethodGet {
			payload["write"] = true
		}
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func errJSON(msg string) string {
	b, _ := json.Marshal(map[string]any{"status": "error", "message": msg})
	return string(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
