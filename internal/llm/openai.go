package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI is an openai_compatible Provider (chat/completions + tool_calls).
type OpenAI struct {
	BaseURL         string
	APIKey          string
	Model           string
	DisableThinking bool
	ThinkingLevel   string
	ThinkingDialect string
	// VisionSupported reports whether the backing model accepts image parts.
	// When false, callers should fall back to text-only. Defaults to false.
	// Exposed as SupportsVision() through the Provider interface.
	VisionSupported bool
	HTTPClient      *http.Client
}

func NewOpenAI(baseURL, apiKey, model string) *OpenAI {
	return &OpenAI{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTPClient: &http.Client{
			// Avoid runs stuck in "running" forever when the provider hangs.
			Timeout: 120 * time.Second,
		},
	}
}

func (o *OpenAI) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	client, reqBody, err := o.prepareChat(ctx, messages, tools, false)
	if err != nil {
		return Message{}, err
	}

	body, status, err := o.postChat(ctx, client, reqBody)
	if err != nil {
		return Message{}, err
	}
	if status == http.StatusBadRequest && shouldRetryWithoutOff(body) {
		clearOffThinkingFields(&reqBody)
		body, status, err = o.postChat(ctx, client, reqBody)
		if err != nil {
			return Message{}, err
		}
	}
	if status < 200 || status >= 300 {
		return Message{}, fmt.Errorf("openai_compatible: status %d: %s", status, truncateBytes(body, 512))
	}

	var parsed openAIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Message{}, fmt.Errorf("openai_compatible: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Message{}, fmt.Errorf("openai_compatible: empty choices")
	}

	return fromOpenAIMessage(parsed.Choices[0].Message, parsed.Usage)
}

// ChatStream posts stream:true and parses SSE deltas. Callbacks receive cumulative
// thinking/content. Errors (4xx, non-SSE, timeout) are returned; callers may fall
// back to Chat — this method does not silently downgrade.
func (o *OpenAI) ChatStream(ctx context.Context, messages []Message, tools []ToolSpec, onThink, onContent func(cumulative string)) (Message, error) {
	client, reqBody, err := o.prepareChat(ctx, messages, tools, true)
	if err != nil {
		return Message{}, err
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return Message{}, fmt.Errorf("openai_compatible: marshal request: %w", err)
	}

	url := o.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return Message{}, fmt.Errorf("openai_compatible: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := client.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("openai_compatible: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return Message{}, fmt.Errorf("openai_compatible: status %d: %s", resp.StatusCode, truncateBytes(body, 512))
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		body, _ := io.ReadAll(resp.Body)
		return Message{}, fmt.Errorf("openai_compatible: non-SSE response (%s): %s", ct, truncateBytes(body, 512))
	}

	return o.readChatSSE(resp.Body, onThink, onContent)
}

func (o *OpenAI) prepareChat(ctx context.Context, messages []Message, tools []ToolSpec, stream bool) (*http.Client, openAIRequest, error) {
	if strings.TrimSpace(o.APIKey) == "" {
		return nil, openAIRequest{}, fmt.Errorf("openai_compatible: api_key is required")
	}
	if o.BaseURL == "" {
		return nil, openAIRequest{}, fmt.Errorf("openai_compatible: base_url is required")
	}
	if o.Model == "" {
		return nil, openAIRequest{}, fmt.Errorf("openai_compatible: model is required")
	}

	client := o.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	level := ThinkingLevelFromContext(ctx)
	if level == "" {
		level = o.ThinkingLevel
	}
	if level == "" && o.DisableThinking {
		level = ThinkingOff
	}
	d := InferDialect(o.ThinkingDialect, o.Model, o.BaseURL)
	f := ApplyThinking(d, level)

	reqBody := openAIRequest{
		Model:    o.Model,
		Messages: toOpenAIMessages(messages),
		Stream:   stream,
	}
	if stream {
		// Ask the gateway to send a final usage chunk (OpenAI-compatible).
		reqBody.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	if len(tools) > 0 {
		reqBody.Tools = toOpenAITools(tools)
	}
	applyThinkingToRequest(&reqBody, f)
	return client, reqBody, nil
}

func (o *OpenAI) readChatSSE(r io.Reader, onThink, onContent func(string)) (Message, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var thinkBuf, contentBuf strings.Builder
	toolAcc := map[int]*openAIToolCall{}
	maxToolIdx := -1
	var usage Usage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return Message{}, fmt.Errorf("openai_compatible: decode SSE: %w", err)
		}
		// With stream_options.include_usage the final chunk carries usage and
		// empty choices. Capture it and skip the delta handling.
		if chunk.Usage != nil {
			usage = *chunk.Usage
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta

		if piece := streamThinkingDelta(delta); piece != "" {
			thinkBuf.WriteString(piece)
			if onThink != nil {
				onThink(thinkBuf.String())
			}
		}
		if piece := contentString(delta.Content); piece != "" {
			contentBuf.WriteString(piece)
			if onContent != nil {
				onContent(contentBuf.String())
			}
		}
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			acc, ok := toolAcc[idx]
			if !ok {
				acc = &openAIToolCall{Type: "function"}
				toolAcc[idx] = acc
			}
			if idx > maxToolIdx {
				maxToolIdx = idx
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Type != "" {
				acc.Type = tc.Type
			}
			if tc.Function.Name != "" {
				acc.Function.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.Function.Arguments += tc.Function.Arguments
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Message{}, fmt.Errorf("openai_compatible: read SSE: %w", err)
	}

	om := openAIMessage{
		Role:             string(RoleAssistant),
		Content:          contentBuf.String(),
		ReasoningContent: thinkBuf.String(),
	}
	for i := 0; i <= maxToolIdx; i++ {
		if acc, ok := toolAcc[i]; ok {
			om.ToolCalls = append(om.ToolCalls, *acc)
		}
	}
	return fromOpenAIMessage(om, usage)
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta openAIStreamDelta `json:"delta"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

type openAIStreamDelta struct {
	Role             string                 `json:"role"`
	Content          any                    `json:"content"`
	ReasoningContent string                 `json:"reasoning_content"`
	Reasoning        json.RawMessage        `json:"reasoning"`
	ToolCalls        []openAIStreamToolCall `json:"tool_calls"`
}

type openAIStreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

func streamThinkingDelta(d openAIStreamDelta) string {
	if strings.TrimSpace(d.ReasoningContent) != "" {
		return d.ReasoningContent
	}
	if len(d.Reasoning) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(d.Reasoning, &s); err == nil {
		return s
	}
	return ""
}

func (o *OpenAI) postChat(ctx context.Context, client *http.Client, reqBody openAIRequest) ([]byte, int, error) {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("openai_compatible: marshal request: %w", err)
	}

	url := o.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("openai_compatible: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("openai_compatible: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("openai_compatible: read body: %w", err)
	}
	return body, resp.StatusCode, nil
}

func applyThinkingToRequest(req *openAIRequest, f ThinkingFields) {
	req.ReasoningEffort = f.ReasoningEffort
	req.Thinking = f.Thinking
	req.EnableThinking = f.EnableThinking
	req.ThinkingBudget = f.ThinkingBudget
	req.Reasoning = f.Reasoning
}

func clearOffThinkingFields(req *openAIRequest) {
	if req.ReasoningEffort == "none" {
		req.ReasoningEffort = ""
	}
	if req.Thinking != nil && req.Thinking.Type == "disabled" {
		req.Thinking = nil
	}
	if req.EnableThinking != nil && !*req.EnableThinking {
		req.EnableThinking = nil
	}
	if req.Reasoning != nil && req.Reasoning.Effort == "none" {
		req.Reasoning = nil
	}
}

func shouldRetryWithoutOff(body []byte) bool {
	s := string(body)
	return strings.Contains(s, "none") && strings.Contains(s, "reasoning_effort")
}

type openAIRequest struct {
	Model           string               `json:"model"`
	Messages        []openAIMessage      `json:"messages"`
	Tools           []openAITool         `json:"tools,omitempty"`
	Stream          bool                 `json:"stream,omitempty"`
	StreamOptions   *streamOptions       `json:"stream_options,omitempty"`
	Thinking        *ThinkingToggle      `json:"thinking,omitempty"`
	ReasoningEffort string               `json:"reasoning_effort,omitempty"`
	EnableThinking  *bool                `json:"enable_thinking,omitempty"`
	ThinkingBudget  *int                 `json:"thinking_budget,omitempty"`
	Reasoning       *OpenRouterReasoning `json:"reasoning,omitempty"`
}

type openAIMessage struct {
	Role             string           `json:"role"`
	Content          any              `json:"content"` // string or []openAIContentPart; always set (strict gateways require the field)
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	ToolCalls        []openAIToolCall `json:"tool_calls,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	Reasoning        json.RawMessage  `json:"reasoning,omitempty"`
}

// openAIContentPart is one element of the OpenAI multimodal content array.
type openAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *openAIImageURL `json:"image_url,omitempty"`
}

type openAIImageURL struct {
	URL string `json:"url"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

// streamOptions is the OpenAI-compatible stream options object.
type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

func toOpenAIMessages(messages []Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages))
	for _, m := range messages {
		om := openAIMessage{
			Role:       string(m.Role),
			ToolCallID: m.ToolCallID,
			Content:    "",
		}
		if len(m.Parts) > 0 {
			if parts := toOpenAIContentParts(m.Parts); len(parts) > 0 {
				om.Content = parts
			}
		} else {
			om.Content = m.Content
		}
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Arguments)
			if args == nil {
				args = []byte("{}")
			}
			om.ToolCalls = append(om.ToolCalls, openAIToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openAIFunctionCall{
					Name:      tc.Name,
					Arguments: string(args),
				},
			})
		}
		out = append(out, om)
	}
	return out
}

// toOpenAIContentParts encodes Message.Parts as the OpenAI multimodal content
// array. Text parts are forwarded as {"type":"text","text":...}; image parts
// become {"type":"image_url","image_url":{"url":"data:<mime>;base64,<...>"}}.
// A ContentPart with DataURL set forwards that URI verbatim. Empty/invalid
// image parts (no bytes and no DataURL) are dropped to avoid sending a
// malformed image_url entry.
func toOpenAIContentParts(parts []ContentPart) []openAIContentPart {
	out := make([]openAIContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			if p.Text == "" {
				continue
			}
			out = append(out, openAIContentPart{Type: "text", Text: p.Text})
		case "image":
			url := p.DataURL
			if url == "" {
				if len(p.ImageBytes) == 0 {
					continue
				}
				mime := p.ImageMIME
				if mime == "" {
					mime = "image/png"
				}
				url = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(p.ImageBytes)
			}
			out = append(out, openAIContentPart{
				Type:     "image_url",
				ImageURL: &openAIImageURL{URL: url},
			})
		}
	}
	return out
}

// SupportsVision implements Provider.
func (o *OpenAI) SupportsVision() bool { return o.VisionSupported }

func toOpenAITools(tools []ToolSpec) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, t := range tools {
		params := t.InputSchema
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

func fromOpenAIMessage(m openAIMessage, usage Usage) (Message, error) {
	msg := Message{
		Role:       Role(m.Role),
		Content:    contentString(m.Content),
		ToolCallID: m.ToolCallID,
		Thinking:   extractThinking(m),
		Usage:      usage,
	}
	for _, tc := range m.ToolCalls {
		args := map[string]any{}
		if strings.TrimSpace(tc.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				return Message{}, fmt.Errorf("openai_compatible: decode tool arguments: %w", err)
			}
		}
		msg.ToolCalls = append(msg.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}
	if msg.Role == "" {
		msg.Role = RoleAssistant
	}
	return msg, nil
}

func extractThinking(m openAIMessage) string {
	if strings.TrimSpace(m.ReasoningContent) != "" {
		return m.ReasoningContent
	}
	if len(m.Reasoning) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Reasoning, &s); err == nil {
		return s
	}
	return ""
}

func truncateBytes(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

// contentString extracts the plain-text content from an openAIMessage.Content
// field. The OpenAI chat/completions response always returns content as a
// string, but we defensively handle the array form (concatenating text parts)
// in case a proxy returns the multimodal shape.
func contentString(c any) string {
	switch v := c.(type) {
	case string:
		return v
	case []any:
		var sb strings.Builder
		for _, p := range v {
			mp, ok := p.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := mp["type"].(string); t == "text" {
				if txt, _ := mp["text"].(string); txt != "" {
					sb.WriteString(txt)
				}
			}
		}
		return sb.String()
	}
	return ""
}
