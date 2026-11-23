package hostedlogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// LoginMethodSuggestion is one AI-refined label/default hint for a rule candidate.
type LoginMethodSuggestion struct {
	ID               string `json:"id"`
	Label            string `json:"label,omitempty"`
	RecommendDefault bool   `json:"recommend_default,omitempty"`
}

const suggestUserPromptPrefix = `你是托管登录配置助手。根据下列规则推断出的登录方式候选，为每项给出简短中文展示名（label），并可选标记一项 recommend_default。
只输出 JSON，格式：{"suggestions":[{"id":"<候选id>","label":"展示名","recommend_default":true|false}]}
suggestions 的 id 必须来自候选列表；不要输出解释、markdown 代码块或其它字段；不要编造候选之外的 id。

候选：
`

// BuildSuggestUserMessage summarizes candidates for the model (no secrets).
func BuildSuggestUserMessage(candidates []store.HostedLoginMethod) string {
	var b strings.Builder
	b.WriteString(suggestUserPromptPrefix)
	for _, c := range candidates {
		fmt.Fprintf(&b, "- id=%q mode=%q login_tool=%q label=%q\n", c.ID, c.Mode, c.LoginTool, c.Label)
	}
	return b.String()
}

// ParseSuggestResponse decodes the model JSON (optional markdown fences).
func ParseSuggestResponse(raw []byte) ([]LoginMethodSuggestion, error) {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return nil, errors.New("empty suggest response")
	}
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSpace(s)
		if strings.HasPrefix(strings.ToLower(s), "json") {
			s = strings.TrimSpace(s[4:])
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
	}
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}
	var payload struct {
		Suggestions []LoginMethodSuggestion `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(s), &payload); err != nil {
		return nil, err
	}
	return payload.Suggestions, nil
}

// SuggestLoginLabels calls the LLM and returns parsed suggestions aligned to candidates.
func SuggestLoginLabels(ctx context.Context, provider llm.Provider, candidates []store.HostedLoginMethod) ([]LoginMethodSuggestion, error) {
	if provider == nil {
		return nil, errors.New("llm provider is nil")
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	msg, err := provider.Chat(ctx, []llm.Message{
		{Role: llm.RoleUser, Content: BuildSuggestUserMessage(candidates)},
	}, nil)
	if err != nil {
		return nil, err
	}
	parsed, err := ParseSuggestResponse([]byte(msg.Content))
	if err != nil {
		return nil, fmt.Errorf("parse suggest response: %w", err)
	}
	allowed := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		allowed[c.ID] = true
	}
	out := make([]LoginMethodSuggestion, 0, len(parsed))
	for _, s := range parsed {
		id := strings.TrimSpace(s.ID)
		if id == "" || !allowed[id] {
			continue
		}
		s.ID = id
		s.Label = strings.TrimSpace(s.Label)
		out = append(out, s)
	}
	return out, nil
}
