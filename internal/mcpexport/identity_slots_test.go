package mcpexport

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestResolveExportIdentityPrefersConnectorSlot(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_def", Name: "Default"})
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_mall", Name: "Mall"})
	key := store.MCPExportKey{
		IdentityID: "mei_def",
		IdentitySlots: []store.MCPExportIdentitySlot{
			{IdentityID: "mei_def"},
			{ConnectorID: "mall", IdentityID: "mei_mall"},
		},
	}
	got, err := resolveExportIdentity(st, key, "mall", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "mei_mall" || got.Name != "Mall" {
		t.Fatalf("got=%+v", got)
	}
	fallback, err := resolveExportIdentity(st, key, "other", "")
	if err != nil {
		t.Fatal(err)
	}
	if fallback.ID != "mei_def" {
		t.Fatalf("fallback=%+v", fallback)
	}
}
