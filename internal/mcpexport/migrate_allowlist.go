package mcpexport

import (
	"bytes"

	"github.com/rebornace/baize/internal/store"
)

const mcpExportAllowlistMigratedValue = "v1"

// MigrateAllowlists backfills empty MCP export key allowlists once per store
// using AllowExportToolGuards at migration time. Memory stores skip migration.
func MigrateAllowlists(st store.Store) error {
	if _, ok := st.(*store.Memory); ok {
		return nil
	}
	raw, ok, err := st.GetSetting(store.SettingKeyMCPExportAllowlistMigrated)
	if err != nil {
		return err
	}
	if ok && bytes.Equal(raw, []byte(mcpExportAllowlistMigratedValue)) {
		return nil
	}

	keys, err := st.ListMCPExportKeys("")
	if err != nil {
		return err
	}
	for _, key := range keys {
		if len(key.AllowedTools) > 0 {
			continue
		}
		var allowed []string
		for _, tool := range st.ListTools() {
			opts := policyOptsFor(st, tool.ConnectorID)
			if AllowExportToolGuards(tool, opts) {
				allowed = append(allowed, tool.Name)
			}
		}
		key.AllowedTools = allowed
		if err := st.UpdateMCPExportKey(key); err != nil {
			return err
		}
	}
	return st.UpsertSetting(store.SettingKeyMCPExportAllowlistMigrated, []byte(mcpExportAllowlistMigratedValue))
}
