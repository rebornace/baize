package tool_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

func TestRegistryListAndConnectorReplace(t *testing.T) {
	r := tool.NewRegistry()
	nop := func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	}
	r.RegisterMeta(tool.Meta{
		Spec: llm.ToolSpec{Name: "create_ticket", Description: "c"},
		ConnectorID: "ticket", OperationID: "create_ticket",
		Method: "POST", Path: "/tickets",
	}, nop, true)

	infos := r.List()
	if len(infos) != 1 || infos[0].Method != "POST" || infos[0].ConnectorID != "ticket" {
		t.Fatalf("list=%+v", infos)
	}

	// 另一 connector 同名 → 冲突
	if !r.WouldConflict("other", []string{"create_ticket"}) {
		t.Fatal("expected conflict")
	}

	r.UnregisterConnector("ticket")
	if len(r.List()) != 0 {
		t.Fatal("expected empty after unregister connector")
	}
}

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
