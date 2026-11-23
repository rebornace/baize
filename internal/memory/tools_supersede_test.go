package memory_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/memory"
)

func TestRememberSupersedesKeys(t *testing.T) {
	reg, mem, _ := setupTools(t)
	ctx := ctxWithConv("c1")
	for _, key := range []string{"old1", "old2"} {
		if _, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
			"text": "old " + key, "key": key,
		}); err != nil || isErr {
			t.Fatalf("seed %s: err=%v isErr=%v", key, err, isErr)
		}
	}

	out, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
		"text":       "new preference",
		"key":        "new",
		"supersedes": []any{"old1", "old2", "missing"},
	})
	if err != nil || isErr {
		t.Fatalf("remember: err=%v isErr=%v out=%v", err, isErr, out)
	}
	if out["ok"] != true || out["superseded"] != 3 || out["forgotten"] != 2 {
		t.Fatalf("out=%v", out)
	}
	got, _ := mem.List("alice", 10, 0)
	if len(got) != 1 || got[0].Key != "new" || got[0].Text != "new preference" {
		t.Fatalf("list=%+v", got)
	}
}

func TestRememberSupersedesIsOwnerScoped(t *testing.T) {
	reg, mem, meta := setupTools(t)
	if err := meta.EnsureMeta(conversation.Meta{
		ID: "c2", OwnerID: "bob", Source: "ui", UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, isErr, err := reg.Invoke(ctxWithConv("c2"), memory.RememberName, map[string]any{
		"text": "bob's preference", "key": "shared",
	}); err != nil || isErr {
		t.Fatalf("bob seed: err=%v isErr=%v", err, isErr)
	}

	if _, isErr, err := reg.Invoke(ctxWithConv("c1"), memory.RememberName, map[string]any{
		"text": "alice preference", "key": "alice", "supersedes": []any{"shared"},
	}); err != nil || isErr {
		t.Fatalf("alice remember: err=%v isErr=%v", err, isErr)
	}
	bob, _ := mem.List("bob", 10, 0)
	if len(bob) != 1 || bob[0].Text != "bob's preference" {
		t.Fatalf("bob list=%+v", bob)
	}
}

func TestRememberSupersedesCapsAtTen(t *testing.T) {
	reg, mem, _ := setupTools(t)
	ctx := ctxWithConv("c1")
	keys := make([]any, 0, 11)
	for i := 0; i < 11; i++ {
		key := fmt.Sprintf("k%02d", i)
		keys = append(keys, key)
		if _, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
			"text": "old " + key, "key": key,
		}); err != nil || isErr {
			t.Fatalf("seed %s: err=%v isErr=%v", key, err, isErr)
		}
	}
	out, isErr, err := reg.Invoke(ctx, memory.RememberName, map[string]any{
		"text": "new", "key": "new", "supersedes": keys,
	})
	if err != nil || isErr {
		t.Fatalf("remember: err=%v isErr=%v out=%v", err, isErr, out)
	}
	if out["superseded"] != 10 || out["forgotten"] != 10 {
		t.Fatalf("out=%v", out)
	}
	got, _ := mem.List("alice", 20, 0)
	if len(got) != 2 { // k10 survives + new
		t.Fatalf("list=%+v", got)
	}
}
