package mcpexport

import (
	"strings"

	"github.com/rebornace/baize/internal/store"
)

// resolveExportIdentity loads the identity for a tool's connector (and optional role).
// Falls back to the key's default identity when no connector-specific slot exists.
func resolveExportIdentity(st store.Store, key store.MCPExportKey, connectorID, role string) (store.MCPExportIdentity, error) {
	id, ok := store.ResolveExportIdentityID(key, connectorID, role)
	if !ok || strings.TrimSpace(id) == "" {
		return store.MCPExportIdentity{}, store.ErrMCPExportIdentityNotFound
	}
	return st.GetMCPExportIdentity(id)
}
