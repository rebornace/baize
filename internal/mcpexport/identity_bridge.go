package mcpexport

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

const ExportConvPrefix = "mcp-export-id:"

// ConversationIDForIdentity maps an export identity id to its isolated conversation key.
func ConversationIDForIdentity(identityID string) string {
	return ExportConvPrefix + identityID
}

func exportToIdentity(export store.MCPExportIdentity) identity.Identity {
	scheme := strings.ToLower(strings.TrimSpace(export.Scheme))
	if scheme == "" {
		scheme = "bearer"
	}
	return identity.Identity{
		ID:                export.ID,
		Label:             export.Name,
		Scheme:            scheme,
		CredentialHeaders: export.Headers,
		Source:            identity.SourceManual,
		Subject:           export.ID,
	}
}

// EnsureExportIdentityInStore upserts export into identity.Store under ConversationIDForIdentity
// and returns the store-assigned identity id.
func EnsureExportIdentityInStore(ids identity.Store, export store.MCPExportIdentity) (string, error) {
	return ids.Upsert(ConversationIDForIdentity(export.ID), exportToIdentity(export))
}

// InvokeContext prepares identity store state and returns a ctx for require_login tool calls.
func InvokeContext(parent context.Context, ids identity.Store, export store.MCPExportIdentity) (context.Context, error) {
	id, err := EnsureExportIdentityInStore(ids, export)
	if err != nil {
		return parent, err
	}
	ctx := identity.WithConversationID(parent, ConversationIDForIdentity(export.ID))
	ctx = identity.WithForceIdentityID(ctx, id)
	return ctx, nil
}
