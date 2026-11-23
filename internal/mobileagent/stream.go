package mobileagent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

type streamState struct {
	mu     sync.Mutex
	deltas []string
	text   strings.Builder
	status string // streaming, cancelling, done, needs_approval, error, cancelled
	result string
	cancel context.CancelFunc
}

func (s *streamState) active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status == "streaming" || s.status == "cancelling"
}

func (s *streamState) append(delta string) {
	if delta == "" {
		return
	}
	s.mu.Lock()
	s.deltas = append(s.deltas, delta)
	s.text.WriteString(delta)
	s.mu.Unlock()
}

func (s *streamState) finish(status, result string) {
	s.mu.Lock()
	s.status = status
	s.result = result
	s.mu.Unlock()
}

func (s *streamState) pollJSON() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	delta := strings.Join(s.deltas, "")
	s.deltas = nil
	out := map[string]any{
		"status": s.status,
		"delta":  delta,
		"text":   s.text.String(),
	}
	if s.result != "" {
		out["result"] = s.result
	}
	if s.status != "streaming" && s.status != "cancelling" {
		out["final"] = true
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// StartStream begins a tool-capable turn in the background. The returned JSON
// is {"status":"streaming"}; consume deltas with PollStream. The final text is
// identical to Send for the same model response.
func (a *Agent) StartStream(userText string) string {
	return a.startStream(userText, nil)
}

// StartStreamWithAttachments starts a streamed turn with display-only
// attachment metadata. File bytes stay in the shell.
func (a *Agent) StartStreamWithAttachments(userText, attachmentsJSON string) string {
	atts, err := parseAttachments(attachmentsJSON)
	if err != nil {
		return errJSON(err.Error())
	}
	return a.startStream(userText, atts)
}

func (a *Agent) startStream(userText string, atts []Attachment) string {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return errJSON("empty user text")
	}
	a.mu.Lock()
	if a.stream != nil && a.stream.active() {
		a.mu.Unlock()
		return errJSON("stream already in progress")
	}
	if a.pending != nil {
		a.mu.Unlock()
		return errJSON("pending approval; Approve or Deny first")
	}
	if a.baseURL == "" || a.model == "" {
		a.mu.Unlock()
		return errJSON("call Configure first")
	}
	a.msgs = append(a.msgs, chatMessage{Role: "user", Content: userText, Attachments: atts})
	return a.startStreamLocked()
}

// StartStreamWithAttachments on the default session.
func StartStreamWithAttachments(userText, attachmentsJSON string) string {
	return defaultSession.StartStreamWithAttachments(userText, attachmentsJSON)
}

// RetryStream restarts the last user turn after a stream error or cancellation.
// It reuses the existing user message, so a retry does not duplicate the turn
// in history. It is rejected once an assistant reply exists or while a stream
// is active.
func (a *Agent) RetryStream() string {
	a.mu.Lock()
	if a.stream != nil && a.stream.active() {
		a.mu.Unlock()
		return errJSON("stream already in progress")
	}
	if a.pending != nil {
		a.mu.Unlock()
		return errJSON("pending approval; Approve or Deny first")
	}
	if a.baseURL == "" || a.model == "" {
		a.mu.Unlock()
		return errJSON("call Configure first")
	}
	if len(a.msgs) == 0 || a.msgs[len(a.msgs)-1].Role != "user" {
		a.mu.Unlock()
		return errJSON("nothing to retry")
	}
	return a.startStreamLocked()
}

// startStreamLocked starts the goroutine for the current a.msgs. The caller
// must hold a.mu; this method releases it.
func (a *Agent) startStreamLocked() string {
	ctx, cancel := context.WithCancel(context.Background())
	st := &streamState{status: "streaming", cancel: cancel}
	a.stream = st
	a.mu.Unlock()

	go func() {
		raw := a.continueLoop(ctx, st.append)
		if ctx.Err() != nil {
			cancel()
			st.finish("cancelled", `{"status":"cancelled"}`)
			return
		}
		cancel()
		st.finish(streamStatusOf(raw), raw)
	}()

	b, _ := json.Marshal(map[string]any{"status": "streaming"})
	return string(b)
}

// StartStream on the default session.
func StartStream(userText string) string { return defaultSession.StartStream(userText) }

// RetryStream on the default session.
func RetryStream() string { return defaultSession.RetryStream() }

// PollStream returns {"status","delta","text"[,"result","final"]}. delta is
// only the text since the previous poll; text is the accumulated stream.
func (a *Agent) PollStream() string {
	a.mu.Lock()
	st := a.stream
	a.mu.Unlock()
	if st == nil {
		b, _ := json.Marshal(map[string]any{"status": "idle"})
		return string(b)
	}
	return st.pollJSON()
}

// PollStream on the default session.
func PollStream() string { return defaultSession.PollStream() }

// CancelStream asks the active stream to stop. Tool execution already in
// flight is not interrupted; the status becomes cancelled once the turn ends.
func (a *Agent) CancelStream() string {
	a.mu.Lock()
	st := a.stream
	a.mu.Unlock()
	if st == nil {
		b, _ := json.Marshal(map[string]any{"status": "idle"})
		return string(b)
	}
	st.mu.Lock()
	if st.status == "streaming" {
		st.status = "cancelling"
		if st.cancel != nil {
			st.cancel()
		}
	}
	status := st.status
	st.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"status": status})
	return string(b)
}

// CancelStream on the default session.
func CancelStream() string { return defaultSession.CancelStream() }

func streamStatusOf(raw string) string {
	var v struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil || v.Status == "" {
		return "error"
	}
	return v.Status
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

// postChatStream sends stream:true and forwards content deltas. Servers that
// answer with plain JSON (no text/event-stream) are decoded as a normal
// response, so a stub or a non-streaming gateway still works.
func postChatStream(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []chatMessage, tools []toolSpec, emit func(string)) (chatMessage, error) {
	reqBody := chatRequest{Model: model, Messages: messages, Tools: tools, Stream: true}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return chatMessage{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chatMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return chatMessage{}, fmt.Errorf("mobileagent: llm request: %w", err)
	}
	defer resp.Body.Close()

	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		if err != nil {
			return chatMessage{}, err
		}
		msg, err := decodeChatResponse(raw, resp.StatusCode)
		if err != nil {
			return chatMessage{}, err
		}
		if msg.Content != "" {
			emit(msg.Content)
		}
		return msg, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return chatMessage{}, fmt.Errorf("mobileagent: llm HTTP %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}

	var content strings.Builder
	type partialTool struct {
		id, typ, name string
		args          strings.Builder
	}
	parts := map[int]*partialTool{}
	var order []int
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return chatMessage{}, fmt.Errorf("mobileagent: decode stream: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				content.WriteString(choice.Delta.Content)
				emit(choice.Delta.Content)
			}
			for _, tc := range choice.Delta.ToolCalls {
				p := parts[tc.Index]
				if p == nil {
					p = &partialTool{}
					parts[tc.Index] = p
					order = append(order, tc.Index)
				}
				if tc.ID != "" {
					p.id = tc.ID
				}
				if tc.Type != "" {
					p.typ = tc.Type
				}
				if tc.Function.Name != "" {
					p.name = tc.Function.Name
				}
				p.args.WriteString(tc.Function.Arguments)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return chatMessage{}, fmt.Errorf("mobileagent: read stream: %w", err)
	}

	out := chatMessage{Role: "assistant", Content: content.String()}
	sort.Ints(order)
	for _, idx := range order {
		p := parts[idx]
		tc := toolCall{ID: p.id, Type: p.typ}
		if tc.Type == "" {
			tc.Type = "function"
		}
		tc.Function.Name = p.name
		tc.Function.Arguments = p.args.String()
		out.ToolCalls = append(out.ToolCalls, tc)
	}
	return out, nil
}
