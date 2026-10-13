package store_test

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestNormalizeMCPExportKeyFromLegacyIdentityID(t *testing.T) {
	k := store.MCPExportKey{IdentityID: "mei_default"}
	store.NormalizeMCPExportKey(&k)
	if len(k.IdentitySlots) != 1 || k.IdentitySlots[0].IdentityID != "mei_default" {
		t.Fatalf("slots=%+v", k.IdentitySlots)
	}
	if k.IdentitySlots[0].ConnectorID != "" {
		t.Fatalf("default slot must have empty connector, got %q", k.IdentitySlots[0].ConnectorID)
	}
}

func TestResolveExportIdentityID(t *testing.T) {
	k := store.MCPExportKey{
		IdentityID: "mei_default",
		IdentitySlots: []store.MCPExportIdentitySlot{
			{IdentityID: "mei_default"},
			{ConnectorID: "mall", IdentityID: "mei_mall"},
			{ConnectorID: "mall", Role: "admin", IdentityID: "mei_mall_admin"},
			{ConnectorID: "crm", IdentityID: "mei_crm"},
		},
	}
	cases := []struct {
		connector, role, want string
	}{
		{"mall", "admin", "mei_mall_admin"},
		{"mall", "", "mei_mall"},
		{"mall", "user", "mei_mall"}, // role miss → connector default
		{"crm", "", "mei_crm"},
		{"unknown", "", "mei_default"},
		{"", "", "mei_default"},
	}
	for _, tc := range cases {
		got, ok := store.ResolveExportIdentityID(k, tc.connector, tc.role)
		if !ok || got != tc.want {
			t.Fatalf("connector=%q role=%q got=%q ok=%v want=%q", tc.connector, tc.role, got, ok, tc.want)
		}
	}
}

func TestResolveExportIdentityIDLegacyOnly(t *testing.T) {
	k := store.MCPExportKey{IdentityID: "mei_only"}
	got, ok := store.ResolveExportIdentityID(k, "mall", "")
	if !ok || got != "mei_only" {
		t.Fatalf("got=%q ok=%v", got, ok)
	}
}

func TestMemoryPersistsIdentitySlots(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_a", Name: "A"})
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_b", Name: "B"})
	key := store.MCPExportKey{
		ID: "mek_1", Name: "k", IdentityID: "mei_a", KeyHash: "h1", Prefix: "bz_",
		IdentitySlots: []store.MCPExportIdentitySlot{
			{IdentityID: "mei_a"},
			{ConnectorID: "mall", IdentityID: "mei_b"},
		},
		AllowedTools: []string{"t"},
	}
	if err := st.InsertMCPExportKey(key); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetMCPExportKey("mek_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.IdentitySlots) != 2 {
		t.Fatalf("slots=%+v", got.IdentitySlots)
	}
	id, ok := store.ResolveExportIdentityID(got, "mall", "")
	if !ok || id != "mei_b" {
		t.Fatalf("resolve=%q ok=%v", id, ok)
	}

	got.IdentitySlots = []store.MCPExportIdentitySlot{
		{IdentityID: "mei_a"},
		{ConnectorID: "crm", IdentityID: "mei_b"},
	}
	if err := st.UpdateMCPExportKey(got); err != nil {
		t.Fatal(err)
	}
	got2, _ := st.GetMCPExportKey("mek_1")
	id, ok = store.ResolveExportIdentityID(got2, "crm", "")
	if !ok || id != "mei_b" {
		t.Fatalf("after update resolve=%q ok=%v slots=%+v", id, ok, got2.IdentitySlots)
	}
}
