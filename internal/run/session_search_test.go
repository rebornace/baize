package run

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

func TestSessionSearchTool(t *testing.T) {
	ms := conversation.NewMemoryStore()
	if _, err := ms.Append("conv1", conversation.Message{
		Role: conversation.RoleUser, Content: "合同号 HT-2026-9 已归档",
	}); err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	eng := &Engine{Store: st, Messages: ms}
	ctx := identity.WithConversationID(context.Background(), "conv1")
	out, isErr, err := eng.sessionSearch(ctx, map[string]any{"query": "HT-2026", "top_k": 5})
	if err != nil || isErr {
		t.Fatalf("sessionSearch: out=%v isErr=%v err=%v", out, isErr, err)
	}
	results, _ := out["results"].([]map[string]any)
	if len(results) != 1 {
		t.Fatalf("results=%+v", out)
	}
	if snip, _ := results[0]["snippet"].(string); snip == "" || results[0]["role"] != conversation.RoleUser {
		t.Fatalf("bad hit %+v", results[0])
	}
}

func TestSessionSearchRequiresConversation(t *testing.T) {
	eng := &Engine{Messages: conversation.NewMemoryStore()}
	out, isErr, err := eng.sessionSearch(context.Background(), map[string]any{"query": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !isErr {
		t.Fatalf("want error payload, got %+v", out)
	}
}
