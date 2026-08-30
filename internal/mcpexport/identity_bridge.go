package mcpexport

import (
	"context"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

const ExportConvPrefix = "mcp-export-id:"

// ConversationIDForIdentity maps an export identity id to its isolated conversation key.
func ConversationIDForIdentity(identityID string) string {
	return ExportConvPrefix + identityID
}

func exportToIdentity(export store.MCPExportIdentity) identity.Identity {
	return identity.Identity{
		ID:                export.ID,
		Label:             export.Name,
		Scheme:            export.Scheme,
		CredentialHeaders: export.Headers,
		Source:            identity.SourceManual,
		Subject:           export.ID,
	}
}

// EnsureExportIdentityInStore upserts export into identity.Store under ConversationIDForIdentity.
func EnsureExportIdentityInStore(ids identity.Store, export store.MCPExportIdentity) error {
	_, err := ids.Upsert(ConversationIDForIdentity(export.ID), exportToIdentity(export))
	return err
}

// InvokeContext prepares identity store state and returns a ctx for require_login tool calls.
func InvokeContext(parent context.Context, ids identity.Store, export store.MCPExportIdentity) (context.Context, error) {
	if err := EnsureExportIdentityInStore(ids, export); err != nil {
		return parent, err
	}
	ctx := identity.WithConversationID(parent, ConversationIDForIdentity(export.ID))
	ctx = identity.WithForceIdentityID(ctx, export.ID)
	return ctx, nil
}
