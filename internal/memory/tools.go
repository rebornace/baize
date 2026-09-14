package memory

import (
	"context"
	"fmt"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

const (
	RememberName = "remember_fact"
	ForgetName   = "forget_fact"
)

// ToolMeta pairs a tool spec with its invoker for registration.
type ToolMeta struct {
	Spec    llm.ToolSpec
	Invoker tool.Invoker
}

// RememberSpec is the LLM tool schema for explicit remember.
func RememberSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        RememberName,
		Description: "Remember a durable fact about the current account for later turns and sessions.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Fact text to remember",
				},
				"key": map[string]any{
					"type":        "string",
					"description": "Optional stable key; same key overwrites the prior fact for this account",
				},
			},
			"required": []string{"text"},
		},
	}
}

// ForgetSpec is the LLM tool schema for explicit forget.
func ForgetSpec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        ForgetName,
		Description: "Forget a previously remembered fact by key or exact text match.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "Optional key of the fact to forget",
				},
				"text": map[string]any{
					"type":        "string",
					"description": "Optional exact text of the fact to forget",
				},
			},
		},
	}
}

// Tools returns remember_fact / forget_fact for registry registration.
func Tools(store Store, meta conversation.MetaStore) []ToolMeta {
	return []ToolMeta{
		{Spec: RememberSpec(), Invoker: RememberInvoker(store, meta)},
		{Spec: ForgetSpec(), Invoker: ForgetInvoker(store, meta)},
	}
}

// RememberInvoker upserts an explicit memory for the conversation owner.
func RememberInvoker(store Store, meta conversation.MetaStore) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		owner, errPayload, ok := ownerFromCtx(ctx, meta)
		if !ok {
			return errPayload, true, nil
		}
		text, ok := strArg(args, "text")
		if !ok {
			return fail("text is required")
		}
		key, _ := strArg(args, "key")
		e, err := store.Upsert(Entry{
			OwnerID: owner,
			Key:     key,
			Text:    text,
			Source:  SourceExplicit,
		})
		if err != nil {
			return fail("%v", err)
		}
		out := map[string]any{
			"ok":     true,
			"id":     e.ID,
			"source": e.Source,
		}
		if e.Key != "" {
			out["key"] = e.Key
		}
		return out, false, nil
	}
}

// ForgetInvoker removes memories by key (preferred) or exact text for the owner.
func ForgetInvoker(store Store, meta conversation.MetaStore) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		owner, errPayload, ok := ownerFromCtx(ctx, meta)
		if !ok {
			return errPayload, true, nil
		}
		key, hasKey := strArg(args, "key")
		text, hasText := strArg(args, "text")
		if !hasKey && !hasText {
			return fail("key or text is required")
		}
		n, err := store.Forget(owner, key, text)
		if err != nil {
			return fail("%v", err)
		}
		return map[string]any{"ok": true, "forgotten": n}, false, nil
	}
}

func ownerFromCtx(ctx context.Context, meta conversation.MetaStore) (string, map[string]any, bool) {
	conv := identity.ConversationIDFrom(ctx)
	if conv == "" {
		return "", map[string]any{"error": "memory requires a conversation context"}, false
	}
	if meta == nil {
		return "", map[string]any{"error": "memory meta store unavailable"}, false
	}
	m, err := meta.GetMeta(conv)
	if err != nil {
		return "", map[string]any{"error": fmt.Sprintf("conversation meta: %v", err)}, false
	}
	if m.OwnerID == "" {
		return "local-dev", nil, true
	}
	return m.OwnerID, nil, true
}

func strArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func fail(format string, a ...any) (map[string]any, bool, error) {
	return map[string]any{"error": fmt.Sprintf(format, a...)}, true, nil
}
