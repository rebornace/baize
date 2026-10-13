package mcpexport_test

import (
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
)

func TestMigrateAllowlistsBackfillsSQLiteKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "allowlist-migrate.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	})

	s.UpsertConnector(store.Connector{ID: "c1", Type: "openapi"})
	s.UpsertTool(store.Tool{
		ConnectorID: "c1", Name: "get_status", Source: store.ToolSourceSpec,
		Enabled: true, Method: "GET", Export: "default",
	})
	s.UpsertTool(store.Tool{
		ConnectorID: "c1", Name: "create_item", Source: store.ToolSourceSpec,
		Enabled: true, Method: "POST", Export: "default",
	})
	id := store.MCPExportIdentity{ID: "idt_1", Name: "Ops"}
	if err := s.UpsertMCPExportIdentity(id); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertMCPExportKey(store.MCPExportKey{
		ID: "k1", Name: "k1", IdentityID: id.ID,
		KeyHash: "abc123" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
		Prefix:  "bze_test",
	}); err != nil {
		t.Fatal(err)
	}

	if err := mcpexport.MigrateAllowlists(s); err != nil {
		t.Fatalf("MigrateAllowlists: %v", err)
	}
	got, err := s.GetMCPExportKey("k1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.AllowedTools) != 1 || got.AllowedTools[0] != "get_status" {
		t.Fatalf("backfill allowlist=%v", got.AllowedTools)
	}
	if err := mcpexport.MigrateAllowlists(s); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	got2, _ := s.GetMCPExportKey("k1")
	if len(got2.AllowedTools) != 1 {
		t.Fatalf("migrate should be idempotent, got %v", got2.AllowedTools)
	}
}

func TestMigrateAllowlistsSkipsMemory(t *testing.T) {
	st := store.NewMemory()
	if err := mcpexport.MigrateAllowlists(st); err != nil {
		t.Fatal(err)
	}
}
