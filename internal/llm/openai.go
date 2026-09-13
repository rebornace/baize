package llm

import (
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
	if strings.TrimSpace(o.APIKey) == "" {
		return Message{}, fmt.Errorf("openai_compatible: api_key is required")
	}
	if o.BaseURL == "" {
		return Message{}, fmt.Errorf("openai_compatible: base_url is required")
	}
	if o.Model == "" {
		return Message{}, fmt.Errorf("openai_compatible: model is required")
	}

	client := o.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	reqBody := openAIRequest{
		Model:    o.Model,
		Messages: toOpenAIMessages(messages),
	}
	if len(tools) > 0 {
		reqBody.Tools = toOpenAITools(tools)
	}
	// DeepSeek V4 defaults thinking on; disable to avoid billing reasoning tokens.
	if o.DisableThinking {
		reqBody.Thinking = &openAIThinking{Type: "disabled"}
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

	resp, err := client.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("openai_compatible: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, fmt.Errorf("openai_compatible: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Message{}, fmt.Errorf("openai_compatible: status %d: %s", resp.StatusCode, truncateBytes(body, 512))
	}

	var parsed openAIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Message{}, fmt.Errorf("openai_compatible: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Message{}, fmt.Errorf("openai_compatible: empty choices")
	}

	return fromOpenAIMessage(parsed.Choices[0].Message)
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Tools    []openAITool    `json:"tools,omitempty"`
	Thinking *openAIThinking `json:"thinking,omitempty"`
}

type openAIThinking struct {
	Type string `json:"type"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    any              `json:"content,omitempty"` // string or []openAIContentPart
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
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
}

func toOpenAIMessages(messages []Message) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages))
	for _, m := range messages {
		om := openAIMessage{
			Role:       string(m.Role),
			ToolCallID: m.ToolCallID,
		}
		if len(m.Parts) > 0 {
			om.Content = toOpenAIContentParts(m.Parts)
		} else if m.Content != "" {
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

func fromOpenAIMessage(m openAIMessage) (Message, error) {
	msg := Message{
		Role:       Role(m.Role),
		Content:    contentString(m.Content),
		ToolCallID: m.ToolCallID,
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
