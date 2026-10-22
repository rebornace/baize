package procfs

import (
	"context"
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/skill"
)

const polishSystem = `You rewrite a Baize SKILL.md draft for clarity.
Return ONLY a complete SKILL.md document with YAML frontmatter (name, description, optional tools) and markdown body.
Do not invent tool names that are not already listed in the input frontmatter tools.
Keep the human-review warning meaning. No code fences around the whole document.`

// MergeToolsPreserveRule keeps polished tools that appear in ruleTools (order of polished first),
// then appends any missing rule tools. Never adds names outside ruleTools.
func MergeToolsPreserveRule(ruleTools, polishedTools []string) []string {
	allowed := make(map[string]bool, len(ruleTools))
	for _, t := range ruleTools {
		if t != "" {
			allowed[t] = true
		}
	}
	seen := make(map[string]bool)
	var out []string
	for _, t := range polishedTools {
		if !allowed[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	for _, t := range ruleTools {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// ApplyPolishResult parses LLM markdown and merges tools from the rule draft.
func ApplyPolishResult(ruleMarkdown, llmMarkdown string) (string, error) {
	rulePkg, err := skill.ParseSKILLMD([]byte(ruleMarkdown))
	if err != nil {
		return "", fmt.Errorf("rule draft: %w", err)
	}
	polished, err := skill.ParseSKILLMD([]byte(stripCodeFence(llmMarkdown)))
	if err != nil {
		return "", fmt.Errorf("polished draft: %w", err)
	}
	tools := MergeToolsPreserveRule(rulePkg.Tools, polished.Tools)
	name := polished.Name
	if name == "" {
		name = rulePkg.Name
	}
	desc := polished.Description
	if strings.TrimSpace(desc) == "" {
		desc = rulePkg.Description
	}
	body := polished.Body
	if strings.TrimSpace(body) == "" {
		body = rulePkg.Body
	}
	raw, err := skill.BuildSKILLMD(skill.ProposeDraft{
		Name:        name,
		Description: desc,
		Tools:       tools,
		Body:        body,
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	// drop first fence line
	lines = lines[1:]
	if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
		lines = lines[:n-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// FormatTimelineForPolish builds a short text summary for the polish prompt.
func FormatTimelineForPolish(tl Timeline) string {
	var b strings.Builder
	for _, e := range tl.Entries {
		line := e.Summary
		if line == "" {
			line = e.Type
		}
		fmt.Fprintf(&b, "- %s | %s | %s\n", e.RunID, e.Type, line)
	}
	return b.String()
}

// PolishMarkdown calls the LLM then ApplyPolishResult against ruleMarkdown.
func PolishMarkdown(ctx context.Context, provider llm.Provider, tl Timeline, ruleMarkdown string) (string, error) {
	if provider == nil {
		return "", fmt.Errorf("llm provider is nil")
	}
	user := "Timeline:\n" + FormatTimelineForPolish(tl) + "\n\nCurrent SKILL.md:\n" + ruleMarkdown
	msg, err := provider.Chat(ctx, []llm.Message{
		{Role: llm.RoleSystem, Content: polishSystem},
		{Role: llm.RoleUser, Content: user},
	}, nil)
	if err != nil {
		return "", err
	}
	return ApplyPolishResult(ruleMarkdown, msg.Content)
}
