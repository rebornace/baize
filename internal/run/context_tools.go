package run

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

type contextToolReg struct {
	Spec    llm.ToolSpec
	Invoker tool.Invoker
}

const (
	ContextPinName     = "context_pin"
	ContextDropName    = "context_drop_tool_results"
	ContextRewriteName = "context_rewrite_summary"
	ContextCompactName = "context_compact_now"

	maxContextPins     = 32
	maxContextPinRunes = 500
)

var contextToolNames = map[string]bool{
	ContextPinName:     true,
	ContextDropName:    true,
	ContextRewriteName: true,
	ContextCompactName: true,
}

// ContextTools registers the four whitelist projection editors. They only
// mutate conversation.ContextProjection / trigger Compactor; they never rewrite
// persisted messages.
func (e *Engine) ContextTools() []contextToolReg {
	return []contextToolReg{
		{Spec: contextPinSpec(), Invoker: e.contextPin},
		{Spec: contextDropSpec(), Invoker: e.contextDrop},
		{Spec: contextRewriteSpec(), Invoker: e.contextRewrite},
		{Spec: contextCompactSpec(), Invoker: e.contextCompactNow},
	}
}

func contextPinSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ContextPinName,
		Description: "把本轮必须记住的短事实钉进模型侧投影（例如订单号、审批结论、硬约束）。" +
			"不会改写已保存的对话历史。Pin a short fact into the model-facing overlay only.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{"type": "string", "description": "Fact to keep visible in later prompts"},
			},
			"required": []string{"text"},
		},
	}
}

func contextDropSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ContextDropName,
		Description: "把指定 tool_call_id 的工具结果从后续模型提示里换成短占位（原始结果仍在运行事件里）。" +
			"不会删除对话历史。Stub bulky in-run tool results in the live prompt only.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool_call_ids": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Tool call ids whose results can be stubbed in the live prompt",
				},
			},
			"required": []string{"tool_call_ids"},
		},
	}
}

func contextRewriteSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ContextRewriteName,
		Description: "重写模型侧投影笔记（不是落库的滚动摘要，也不是原始对话）。" +
			"Rewrite the overlay notes shown to the model on later turns.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{"type": "string", "description": "Replacement overlay notes"},
			},
			"required": []string{"summary"},
		},
	}
}

func contextCompactSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name: ContextCompactName,
		Description: "立即把较早对话折进滚动摘要（与自动压缩同一路径，仍不删除原文）。" +
			"Force rolling-summary compaction now.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func (e *Engine) contextPin(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	text, ok := strArg(args, "text")
	if !ok {
		return failContext("text is required")
	}
	if utf8.RuneCountInString(text) > maxContextPinRunes {
		return failContext("text exceeds %d runes", maxContextPinRunes)
	}
	p := e.loadProjection(conv)
	if len(p.Pins) >= maxContextPins {
		return failContext("at most %d pins", maxContextPins)
	}
	p.Pins = append(p.Pins, text)
	if err := e.saveProjection(p); err != nil {
		return failContext("%v", err)
	}
	return map[string]any{"ok": true, "pins": len(p.Pins), "revision": p.Revision}, false, nil
}

func (e *Engine) contextDrop(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	ids, ok := stringSliceArg(args, "tool_call_ids")
	if !ok || len(ids) == 0 {
		return failContext("tool_call_ids is required")
	}
	p := e.loadProjection(conv)
	have := make(map[string]struct{}, len(p.Dropped))
	for _, id := range p.Dropped {
		have[id] = struct{}{}
	}
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, exists := have[id]; exists {
			continue
		}
		p.Dropped = append(p.Dropped, id)
		have[id] = struct{}{}
	}
	if err := e.saveProjection(p); err != nil {
		return failContext("%v", err)
	}
	return map[string]any{"ok": true, "dropped": len(p.Dropped), "revision": p.Revision}, false, nil
}

func (e *Engine) contextRewrite(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	summary, ok := strArg(args, "summary")
	if !ok {
		return failContext("summary is required")
	}
	p := e.loadProjection(conv)
	p.Summary = summary
	if err := e.saveProjection(p); err != nil {
		return failContext("%v", err)
	}
	return map[string]any{"ok": true, "revision": p.Revision}, false, nil
}

func (e *Engine) contextCompactNow(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	conv, errPayload, ok := convFromToolCtx(ctx)
	if !ok {
		return errPayload, true, nil
	}
	if e.Compactor == nil {
		return failContext("compaction is not available")
	}
	profileID := ""
	if rec, err := e.Store.GetRun(identity.RunIDFrom(ctx)); err == nil && rec != nil {
		profileID = rec.ModelProfileID
	}
	changed, err := e.Compactor.ForceCompact(ctx, conv, profileID)
	if err != nil {
		return failContext("%v", err)
	}
	return map[string]any{"ok": true, "changed": changed}, false, nil
}

func convFromToolCtx(ctx context.Context) (string, map[string]any, bool) {
	conv := identity.ConversationIDFrom(ctx)
	if conv == "" {
		return "", map[string]any{"error": "conversation context required"}, false
	}
	return conv, nil, true
}

func strArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", false
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "<nil>" {
		return "", false
	}
	return s, true
}

func stringSliceArg(args map[string]any, key string) ([]string, bool) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, false
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, len(out) > 0
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" && s != "<nil>" {
				out = append(out, s)
			}
		}
		return out, len(out) > 0
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return nil, false
		}
		return []string{s}, true
	}
}

func failContext(format string, args ...any) (map[string]any, bool, error) {
	return map[string]any{"error": fmt.Sprintf(format, args...)}, true, nil
}
