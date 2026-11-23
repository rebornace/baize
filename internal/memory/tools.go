package memory

import (
	"context"
	"fmt"
	"strings"

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
		Description: "Remember a durable fact about the current account for later turns and sessions. When correcting an existing preference, reuse its key or list the old keys in supersedes so the new value replaces them.",
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
				"supersedes": map[string]any{
					"type":        "array",
					"maxItems":    10,
					"description": "Optional keys of older facts to forget before writing this one (at most 10)",
					"items":       map[string]any{"type": "string"},
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
		supersedes := stringListArg(args, "supersedes", 10)
		forgotten := 0
		supersededErr := ""
		for _, oldKey := range supersedes {
			n, ferr := store.Forget(owner, oldKey, "")
			if ferr != nil {
				if supersededErr == "" {
					supersededErr = shortErr(ferr.Error())
				}
				continue
			}
			forgotten += n
		}
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
		if len(supersedes) > 0 {
			out["superseded"] = len(supersedes)
			out["forgotten"] = forgotten
			if supersededErr != "" {
				out["superseded_error"] = supersededErr
			}
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

// stringListArg reads an optional string array, trimming, deduping and capping
// the entries.
func stringListArg(args map[string]any, name string, max int) []string {
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil
	}
	var vals []string
	switch v := raw.(type) {
	case []string:
		vals = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				vals = append(vals, s)
			}
		}
	default:
		return nil
	}
	out := make([]string, 0, len(vals))
	seen := map[string]bool{}
	for _, v := range vals {
		k := strings.TrimSpace(v)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
		if max > 0 && len(out) == max {
			break
		}
	}
	return out
}

func shortErr(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func fail(format string, a ...any) (map[string]any, bool, error) {
	return map[string]any{"error": fmt.Sprintf(format, a...)}, true, nil
}
