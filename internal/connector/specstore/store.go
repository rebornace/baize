package specstore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rebornace/baize/internal/blob"
)

// IsBlobSpecKey reports whether spec is a connector blob object key
// (connectors/<id>/...) rather than a filesystem path.
func IsBlobSpecKey(spec string) bool {
	return strings.HasPrefix(spec, "connectors/") && !filepath.IsAbs(spec)
}

// IsLegacyConnectorFSPath reports whether spec is an absolute path to the
// old on-disk connectors/<id>/openapi.normalized.json layout.
func IsLegacyConnectorFSPath(spec string) bool {
	if !filepath.IsAbs(spec) {
		return false
	}
	slash := filepath.ToSlash(spec)
	return strings.Contains(slash, "/connectors/") && strings.HasSuffix(slash, "/openapi.normalized.json")
}

// Write persists imported and normalized spec bytes in store and returns the
// normalized object key for connector.Spec.
func Write(ctx context.Context, store blob.Store, connectorID string, original, normalized []byte) (specKey string, err error) {
	if store == nil {
		return "", fmt.Errorf("blob store is required")
	}
	importedKey := blob.ConnectorImportedKey(connectorID)
	if err := store.Put(ctx, importedKey, original, "application/octet-stream"); err != nil {
		return "", fmt.Errorf("write imported spec: %w", err)
	}
	specKey = blob.ConnectorNormalizedKey(connectorID)
	if err := store.Put(ctx, specKey, normalized, "application/json"); err != nil {
		return "", fmt.Errorf("write normalized spec: %w", err)
	}
	return specKey, nil
}
