package capfill_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/capfill"
	"github.com/rebornace/baize/internal/store"
)

func TestValidateProposePluginArgsRejects(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	if err := capfill.ValidateProposePluginArgs(map[string]any{
		"connector_id": "orders-ro",
		"goal":         "g",
		"gap":          "x",
		"risk":         "r",
		"read_only":    true,
		"tools":        []any{},
	}, now); err == nil {
		t.Fatal("empty tools")
	} else if !strings.Contains(err.Error(), "tools") {
		t.Fatalf("want tools in error, got: %v", err)
	}
	if err := capfill.ValidateProposePluginArgs(map[string]any{
		"connector_id": "", "goal": "g", "gap": "x",
		"tools": []any{map[string]any{"name": "list_orders"}},
	}, now); err == nil {
		t.Fatal("empty connector_id")
	}
	if err := capfill.ValidateProposePluginArgs(map[string]any{
		"connector_id": "orders-ro",
		"goal":         "g",
		"gap":          "x",
		"risk":         "r",
		"read_only":    true,
		"tools": []any{
			map[string]any{"name": "list_orders"},
			map[string]any{"name": "list_orders"},
		},
	}, now); err == nil {
		t.Fatal("duplicate tool names")
	}
}

func TestInstallPlaceholderConflict(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "orders-ro", Type: "http"})
	d := capfill.PluginDraft{
		ConnectorID: "orders-ro", Goal: "按月列订单", Gap: "无过滤",
		Tools:    []capfill.PluginToolDraft{{Name: "list_orders_by_month", Description: "只读"}},
		ReadOnly: true, Risk: "含 PII",
	}
	if err := capfill.InstallPluginPlaceholder(st, d); err == nil {
		t.Fatal("want connector conflict")
	}
}

func TestInstallPlaceholderToolNameConflict(t *testing.T) {
	st := store.NewMemory()
	st.UpsertTool(store.Tool{Name: "list_orders_by_month", ConnectorID: "other", Source: store.ToolSourceSpec})
	d := capfill.PluginDraft{
		ConnectorID: "orders-ro", Goal: "按月列订单", Gap: "无过滤",
		Tools:    []capfill.PluginToolDraft{{Name: "list_orders_by_month", Description: "只读"}},
		ReadOnly: true, Risk: "含 PII",
	}
	if err := capfill.InstallPluginPlaceholder(st, d); err == nil {
		t.Fatal("want tool name conflict")
	}
	if _, err := st.GetConnector("orders-ro"); err == nil {
		t.Fatal("must not write connector on tool conflict")
	}
}

func TestInstallPlaceholderWritesDisabledTools(t *testing.T) {
	st := store.NewMemory()
	d := capfill.PluginDraft{
		ConnectorID: "orders-ro", Goal: "按月列订单", Gap: "无过滤",
		Tools:    []capfill.PluginToolDraft{{Name: "list_orders_by_month", Description: "只读"}},
		ReadOnly: true, Risk: "含 PII", BaseURL: "http://127.0.0.1:9",
	}
	if err := capfill.InstallPluginPlaceholder(st, d); err != nil {
		t.Fatal(err)
	}
	c, err := st.GetConnector("orders-ro")
	if err != nil || c.Type != "http" || c.BaseURL != "http://127.0.0.1:9" {
		t.Fatalf("%+v %v", c, err)
	}
	tool, err := st.GetTool("list_orders_by_month")
	if err != nil || tool.Enabled || tool.Source != store.ToolSourcePlugin || tool.ConnectorID != "orders-ro" {
		t.Fatalf("%+v %v", tool, err)
	}
	if tool.Description != "只读" {
		t.Fatalf("description=%q", tool.Description)
	}
	if tool.ReadOnlyHint == nil || !*tool.ReadOnlyHint {
		t.Fatalf("read_only_hint=%v", tool.ReadOnlyHint)
	}
}
