package bootstrap

import (
	"encoding/json"
	"strings"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

type connectorGroupsPersist struct {
	Groups []config.ConnectorGroup `json:"groups"`
}

// loadConnectorGroupsOverlay returns KV groups when present, else yamlBaseline.
func loadConnectorGroupsOverlay(st store.Store, yamlBaseline []config.ConnectorGroup) ([]config.ConnectorGroup, string, error) {
	if st == nil {
		return yamlBaseline, "yaml", nil
	}
	raw, ok, err := st.GetSetting(store.SettingKeyConnectorGroups)
	if err != nil {
		return yamlBaseline, "yaml", err
	}
	if !ok || len(raw) == 0 || string(raw) == "null" || string(raw) == "[]" {
		return yamlBaseline, "yaml", nil
	}
	var body connectorGroupsPersist
	if err := json.Unmarshal(raw, &body); err != nil {
		var arr []config.ConnectorGroup
		if err2 := json.Unmarshal(raw, &arr); err2 != nil {
			return yamlBaseline, "yaml", err
		}
		return normalizeGroups(arr), "kv", nil
	}
	return normalizeGroups(body.Groups), "kv", nil
}

func normalizeGroups(in []config.ConnectorGroup) []config.ConnectorGroup {
	seenID := map[string]bool{}
	var out []config.ConnectorGroup
	for _, g := range in {
		id := strings.TrimSpace(g.ID)
		if id == "" || seenID[id] {
			continue
		}
		seenID[id] = true
		seen := map[string]bool{}
		var conns []string
		for _, c := range g.Connectors {
			c = strings.TrimSpace(c)
			if c == "" || seen[c] {
				continue
			}
			seen[c] = true
			conns = append(conns, c)
		}
		if len(conns) == 0 {
			continue
		}
		out = append(out, config.ConnectorGroup{ID: id, Connectors: conns})
	}
	return out
}
