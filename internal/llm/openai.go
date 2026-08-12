package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAI is an openai_compatible Provider (chat/completions + tool_calls).
type OpenAI struct {
	BaseURL         string
	APIKey          string
	Model           string
	DisableThinking bool
	HTTPClient      *http.Client
}

func NewOpenAI(baseURL, apiKey, model string) *OpenAI {
	return &OpenAI{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: http.DefaultClient,
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
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
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
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
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
		Content:    m.Content,
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
