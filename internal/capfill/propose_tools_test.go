package capfill_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/capfill"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/store"
)

func proposePluginValidArgs() map[string]any {
	return map[string]any{
		"connector_id": "orders-ro",
		"goal":         "按月列订单",
		"gap":          "现有 list_orders 无 user/month 过滤",
		"read_only":    true,
		"risk":         "订单含 PII",
		"tools": []any{
			map[string]any{"name": "list-orders-by-month", "description": "只读按月"},
		},
	}
}

func TestProposePluginInvokerInstalls(t *testing.T) {
	st := store.NewMemory()
	tm := capfill.ProposePluginToolMeta(st)
	out, isErr, err := tm.Invoker(context.Background(), proposePluginValidArgs())
	if err != nil || isErr {
		t.Fatalf("%v %v %v", out, isErr, err)
	}
	if out["ok"] != true || out["connector_id"] != "orders-ro" {
		t.Fatalf("%v", out)
	}
	tools, ok := out["tools"].([]string)
	if !ok || len(tools) != 1 || tools[0] != "list-orders-by-month" {
		t.Fatalf("tools=%v", out["tools"])
	}
	if _, err := st.GetConnector("orders-ro"); err != nil {
		t.Fatal(err)
	}
}

func TestProposePluginInvokerOperatorDenied(t *testing.T) {
	st := store.NewMemory()
	tm := capfill.ProposePluginToolMeta(st)
	ctx := controlplane.WithRole(context.Background(), controlplane.RoleOperator)
	out, isErr, err := tm.Invoker(ctx, proposePluginValidArgs())
	if err != nil || !isErr {
		t.Fatalf("out=%v isErr=%v err=%v", out, isErr, err)
	}
	if out["error"] != "propose_plugin requires admin role" {
		t.Fatalf("out=%v", out)
	}
	if _, err := st.GetConnector("orders-ro"); err == nil {
		t.Fatal("must not write")
	}
}

func TestProposePluginInvokerNilStore(t *testing.T) {
	tm := capfill.ProposePluginToolMeta(nil)
	out, isErr, err := tm.Invoker(context.Background(), proposePluginValidArgs())
	if err != nil || !isErr {
		t.Fatalf("out=%v isErr=%v err=%v", out, isErr, err)
	}
	if out["error"] != "store unavailable" {
		t.Fatalf("out=%v", out)
	}
}

func TestProposePluginSpec(t *testing.T) {
	spec := capfill.ProposePluginSpec()
	if spec.Name != capfill.ProposeName {
		t.Fatalf("name=%q", spec.Name)
	}
	required, ok := spec.InputSchema["required"].([]string)
	if !ok {
		t.Fatal("missing required")
	}
	want := map[string]bool{
		"connector_id": true,
		"goal":         true,
		"gap":          true,
		"read_only":    true,
		"risk":         true,
		"tools":        true,
	}
	for _, k := range required {
		if !want[k] {
			t.Fatalf("unexpected required key %q", k)
		}
		delete(want, k)
	}
	if len(want) != 0 {
		t.Fatalf("missing required keys: %v", want)
	}
}
