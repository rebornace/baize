package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/memory"
	"github.com/rebornace/baize/internal/tool"
)

func ctxWithConv(conv string) context.Context {
	return identity.WithConversationID(context.Background(), conv)
}

func setupTools(t *testing.T) (*tool.Registry, memory.Store, *conversation.MemoryStore) {
	t.Helper()
	mem := memory.NewMemoryStore()
	meta := conversation.NewMemoryStore()
	if err := meta.EnsureMeta(conversation.Meta{
		ID: "c1", OwnerID: "alice", Source: "ui", UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	for _, tm := range memory.Tools(mem, meta) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	return reg, mem, meta
}

func TestRememberInvokerListsEntry(t *testing.T) {
	reg, mem, _ := setupTools(t)
	ctx := ctxWithConv("c1")

	c, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
		"text": "喜欢绿茶",
		"key":  "drink",
	})
	if err != nil || isErr {
		t.Fatalf("remember err=%v isErr=%v c=%v", err, isErr, c)
	}
	if c["ok"] != true || c["id"] == nil || c["key"] != "drink" {
		t.Fatalf("unexpected result: %v", c)
	}

	got, err := mem.List("alice", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "喜欢绿茶" || got[0].Key != "drink" || got[0].Source != memory.SourceExplicit {
		t.Fatalf("List=%+v", got)
	}
}

func TestRememberRequiresConversation(t *testing.T) {
	reg, _, _ := setupTools(t)
	c, isErr, err := reg.Invoke(context.Background(), memory.RememberName, map[string]any{
		"text": "x",
	})
	if err != nil || !isErr {
		t.Fatalf("want isError without conversation, got isErr=%v err=%v c=%v", isErr, err, c)
	}
}

func TestForgetByKey(t *testing.T) {
	reg, mem, _ := setupTools(t)
	ctx := ctxWithConv("c1")

	_, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
		"text": "红茶",
		"key":  "drink",
	})
	if err != nil || isErr {
		t.Fatalf("remember: err=%v isErr=%v", err, isErr)
	}

	c, isErr, err := reg.Invoke(ctx, memory.ForgetName, map[string]any{"key": "drink"})
	if err != nil || isErr {
		t.Fatalf("forget err=%v isErr=%v c=%v", err, isErr, c)
	}
	if c["forgotten"] != 1 {
		t.Fatalf("forgotten=%v want 1", c["forgotten"])
	}
	got, _ := mem.List("alice", 10, 0)
	if len(got) != 0 {
		t.Fatalf("want empty after forget, got %+v", got)
	}
}

func TestForgetByText(t *testing.T) {
	reg, mem, _ := setupTools(t)
	ctx := ctxWithConv("c1")

	_, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{"text": "喜欢绿茶"})
	if err != nil || isErr {
		t.Fatal(err, isErr)
	}
	c, isErr, err := reg.Invoke(ctx, memory.ForgetName, map[string]any{"text": "喜欢绿茶"})
	if err != nil || isErr {
		t.Fatalf("forget err=%v isErr=%v c=%v", err, isErr, c)
	}
	if c["forgotten"] != 1 {
		t.Fatalf("forgotten=%v", c["forgotten"])
	}
	got, _ := mem.List("alice", 10, 0)
	if len(got) != 0 {
		t.Fatalf("want empty, got %+v", got)
	}
}

func TestRememberKeyOverwrites(t *testing.T) {
	reg, mem, _ := setupTools(t)
	ctx := ctxWithConv("c1")

	first, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
		"text": "绿茶", "key": "pref",
	})
	if err != nil || isErr {
		t.Fatal(err, isErr)
	}
	second, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
		"text": "红茶", "key": "pref",
	})
	if err != nil || isErr {
		t.Fatal(err, isErr)
	}
	if first["id"] != second["id"] {
		t.Fatalf("want same id on key overwrite: %v vs %v", first["id"], second["id"])
	}
	got, _ := mem.List("alice", 10, 0)
	if len(got) != 1 || got[0].Text != "红茶" {
		t.Fatalf("List=%+v", got)
	}
}
