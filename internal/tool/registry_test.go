package tool_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/tool"
)

func TestRegistryInvoke(t *testing.T) {
	r := tool.NewRegistry()
	r.Register("echo", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true, "args": args}, false, nil
	})
	out, isErr, err := r.Invoke(context.Background(), "echo", map[string]any{"a": 1})
	if err != nil || isErr || out["ok"] != true {
		t.Fatalf("%v %v %v", out, isErr, err)
	}
}
