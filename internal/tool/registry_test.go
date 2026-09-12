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
		Spec:        llm.ToolSpec{Name: "create_ticket", Description: "c"},
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

func TestRequireLoginRoundTrip(t *testing.T) {
	reg := tool.NewRegistry()
	reg.RegisterMeta(tool.Meta{
		Spec:         llm.ToolSpec{Name: "create_ticket"},
		ConnectorID:  "ticket-api",
		RequireLogin: true,
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}, false)
	if !reg.RequiresLogin("create_ticket") {
		t.Fatal("expected require_login")
	}
	if reg.RequiresApproval("create_ticket") {
		t.Fatal("approval must stay independent")
	}
	info, ok := reg.Get("create_ticket")
	if !ok || !info.RequireLogin || info.RequireApproval {
		t.Fatalf("%+v ok=%v", info, ok)
	}
	if err := reg.SetRequireLogin("create_ticket", false); err != nil {
		t.Fatal(err)
	}
	if reg.RequiresLogin("create_ticket") {
		t.Fatal("toggle off")
	}
	if err := reg.SetRequireLogin("missing", true); err == nil {
		t.Fatal("expected unknown tool error")
	}
	got := tool.LoginRequiredContent()
	if got["code"] != "login_required" || got["message"] != "此工具需要先登录" {
		t.Fatalf("%+v", got)
	}
}

func TestRegistrySetDescription(t *testing.T) {
	r := tool.NewRegistry()
	nop := func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{}, false, nil
	}
	r.RegisterMeta(tool.Meta{
		Spec:        llm.ToolSpec{Name: "create_ticket", Description: "old"},
		ConnectorID: "ticket", Method: "POST", Path: "/tickets",
	}, nop, true)
	if err := r.SetDescription("create_ticket", "new"); err != nil {
		t.Fatal(err)
	}
	info, ok := r.Get("create_ticket")
	if !ok || info.Description != "new" || !info.RequireApproval {
		t.Fatalf("info=%+v ok=%v", info, ok)
	}
	if err := r.SetDescription("missing", "x"); err == nil {
		t.Fatal("expected unknown tool")
	}
}
