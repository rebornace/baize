package specstore

import (
	"fmt"
	"os"
	"path/filepath"
)

// Write persists imported and normalized spec files under dataRoot/connectors/{id}/.
// normalizedPath is relative to dataRoot for connector.Spec.
func Write(dataRoot, connectorID string, originalContent []byte, normalizedJSON []byte) (normalizedPath string, err error) {
	dir := filepath.Join(dataRoot, "connectors", connectorID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create connector dir: %w", err)
	}

	importedPath := filepath.Join(dir, "imported.bin")
	if err := os.WriteFile(importedPath, originalContent, 0o644); err != nil {
		return "", fmt.Errorf("write imported spec: %w", err)
	}

	normalizedPath = filepath.Join("connectors", connectorID, "openapi.normalized.json")
	fullNormalized := filepath.Join(dataRoot, normalizedPath)
	if err := os.WriteFile(fullNormalized, normalizedJSON, 0o644); err != nil {
		return "", fmt.Errorf("write normalized spec: %w", err)
	}
	return normalizedPath, nil
}
