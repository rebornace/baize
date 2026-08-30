package mcpexport_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
)

func TestConversationIDForIdentity(t *testing.T) {
	got := mcpexport.ConversationIDForIdentity("idt_export_1")
	want := mcpexport.ExportConvPrefix + "idt_export_1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestInvokeContextEnsuresIdentity(t *testing.T) {
	ids := identity.NewMemoryStore()
	export := store.MCPExportIdentity{
		ID:      "idt_export_1",
		Name:    "Ops Bot",
		Scheme:  "Bearer",
		Headers: map[string]string{"Authorization": "Bearer secret-token"},
	}
	ctx, err := mcpexport.InvokeContext(context.Background(), ids, export)
	if err != nil {
		t.Fatalf("InvokeContext: %v", err)
	}
	conv := mcpexport.ConversationIDForIdentity(export.ID)
	if got := identity.ConversationIDFrom(ctx); got != conv {
		t.Fatalf("ConversationIDFrom=%q want %q", got, conv)
	}
	list := ids.List(conv)
	if len(list) != 1 {
		t.Fatalf("List=%+v", list)
	}
	if got := identity.ForceIdentityIDFrom(ctx); got != list[0].ID {
		t.Fatalf("ForceIdentityIDFrom=%q store ID=%q", got, list[0].ID)
	}
	if list[0].Label != export.Name || list[0].Scheme != "bearer" {
		t.Fatalf("identity=%+v", list[0])
	}
	if list[0].CredentialHeaders["Authorization"] != "Bearer secret-token" {
		t.Fatalf("headers=%v", list[0].CredentialHeaders)
	}
	if list[0].Source != identity.SourceManual {
		t.Fatalf("source=%q", list[0].Source)
	}
}
