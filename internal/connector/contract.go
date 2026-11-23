package connector

import (
	"encoding/json"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// ContractHash is a thin alias for store.ContractHash (call-site convenience).
func ContractHash(t store.Tool) string { return store.ContractHash(t) }

// Empty reports whether the catalog diff has no added/removed/changed tools.
func (d CatalogDiff) Empty() bool {
	return len(d.Added) == 0 && len(d.Removed) == 0 && len(d.Changed) == 0
}

func recordCatalogRevision(st store.Store, connectorID string, revision int64, diff CatalogDiff, merged []store.Tool) {
	if st == nil || revision <= 0 {
		return
	}
	byName := map[string]store.Tool{}
	for _, t := range merged {
		byName[t.Name] = t
	}
	hashes := map[string]string{}
	for _, name := range diff.Added {
		if t, ok := byName[name]; ok {
			hashes[name] = store.ContractHash(t)
		}
	}
	for _, ch := range diff.Changed {
		if t, ok := byName[ch.Name]; ok {
			hashes[ch.Name] = store.ContractHash(t)
		}
	}
	raw, err := json.Marshal(diff)
	if err != nil {
		return
	}
	_ = st.PutCatalogRevision(store.ConnectorCatalogRevision{
		ConnectorID: connectorID,
		Revision:    revision,
		At:          time.Now().UTC(),
		DiffJSON:    raw,
		ToolHashes:  hashes,
	})
}
