// Package mobileagent is the MOBILE-RT M0 thin bind surface: trimmed on-device
// orchestration (memory session + one chat turn) that talks to an
// OpenAI-compatible LLM over HTTPS. It does not import cmd/baize, channels,
// MCP, Postgres, or the full run Engine.
//
// Intended gomobile target:
//
//	gomobile bind -target=android -o mobileagent.aar ./internal/mobileagent
//
// The client is intentionally self-contained (no internal/llm) so the mobile
// dependency graph stays free of store/pgx/OTel.
package mobileagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Agent holds in-process conversation state and LLM config.
// Safe for concurrent Configure / ChatOnce from a single session UI.
type Agent struct {
	mu      sync.Mutex
	baseURL string
	apiKey  string
	model   string
	msgs    []chatMessage
	client  *http.Client
}

// New returns an unconfigured Agent. Call Configure before ChatOnce.
func New() *Agent {
	return &Agent{
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// defaultSession is the package-level agent for bind-friendly free functions.
var defaultSession = New()

// Configure sets the OpenAI-compatible API base, key, and model, and clears
// prior in-memory messages. apiBase should be the root (e.g. https://api.openai.com/v1).
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
	return nil
}

// Configure configures the default session (gomobile-friendly).
func Configure(apiBase, apiKey, model string) error {
	return defaultSession.Configure(apiBase, apiKey, model)
}

// Reset clears in-memory messages without changing API config.
func (a *Agent) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.msgs = nil
}

// Reset clears the default session history.
func Reset() { defaultSession.Reset() }

// ChatOnce appends a user turn, calls the LLM with no tools, appends the
// assistant reply to memory, and returns the assistant text.
// Does not use baize serve; the device (or this process) owns the loop.
func (a *Agent) ChatOnce(userText string) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return "", fmt.Errorf("mobileagent: empty user text")
	}

	a.mu.Lock()
	base := a.baseURL
	key := a.apiKey
	model := a.model
	client := a.client
	history := append([]chatMessage(nil), a.msgs...)
	a.mu.Unlock()

	if base == "" || model == "" {
		return "", fmt.Errorf("mobileagent: call Configure first")
	}

	history = append(history, chatMessage{Role: "user", Content: userText})
	assistant, err := postChat(context.Background(), client, base, key, model, history)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	a.msgs = append(a.msgs, chatMessage{Role: "user", Content: userText})
	a.msgs = append(a.msgs, chatMessage{Role: "assistant", Content: assistant})
	a.mu.Unlock()
	return assistant, nil
}

// ChatOnce runs on the default session.
func ChatOnce(userText string) (string, error) {
	return defaultSession.ChatOnce(userText)
}

// MessageCount returns how many messages are in memory (user+assistant).
func (a *Agent) MessageCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.msgs)
}

// MessageCount for the default session.
func MessageCount() int { return defaultSession.MessageCount() }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func postChat(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []chatMessage) (string, error) {
	body, err := json.Marshal(chatRequest{Model: model, Messages: messages})
	if err != nil {
		return "", err
	}
	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("mobileagent: llm request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("mobileagent: decode response (HTTP %d): %w", resp.StatusCode, err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("mobileagent: llm error: %s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("mobileagent: llm HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("mobileagent: empty choices")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("mobileagent: empty assistant content")
	}
	return text, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
