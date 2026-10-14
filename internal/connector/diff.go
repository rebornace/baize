package connector

import (
	"bytes"
	"encoding/json"
	"sort"

	"github.com/rebornace/baize/internal/store"
)

// CatalogDiff summarizes tool-catalog changes after a connector Apply/merge.
type CatalogDiff struct {
	Added   []string         `json:"added"`
	Removed []string         `json:"removed"`
	Changed []CatalogChange  `json:"changed"`
}

// CatalogChange describes how one retained tool differed from its prior row.
type CatalogChange struct {
	Name                string `json:"name"`
	MethodPathChanged   bool   `json:"method_path_changed,omitempty"`
	OperationIDChanged  bool   `json:"operation_id_changed,omitempty"`
	SchemaChanged       bool   `json:"schema_changed,omitempty"`
}

// DiffCatalog compares the previous connector catalog with the merged result.
// Extra rows are never listed as removed (they are always preserved by merge).
func DiffCatalog(existing, merged []store.Tool) CatalogDiff {
	prev := map[string]store.Tool{}
	for _, t := range existing {
		if t.Source == store.ToolSourceExtra {
			continue
		}
		prev[t.Name] = t
	}
	next := map[string]store.Tool{}
	for _, t := range merged {
		if t.Source == store.ToolSourceExtra {
			continue
		}
		next[t.Name] = t
	}

	var d CatalogDiff
	for name := range next {
		if _, ok := prev[name]; !ok {
			d.Added = append(d.Added, name)
		}
	}
	for name := range prev {
		if _, ok := next[name]; !ok {
			d.Removed = append(d.Removed, name)
		}
	}
	for name, n := range next {
		p, ok := prev[name]
		if !ok {
			continue
		}
		ch := CatalogChange{Name: name}
		if p.Method != n.Method || p.Path != n.Path {
			ch.MethodPathChanged = true
		}
		if p.OperationID != n.OperationID {
			ch.OperationIDChanged = true
		}
		if !canonicalJSONEqual(p.InputSchema, n.InputSchema) {
			ch.SchemaChanged = true
		}
		if ch.MethodPathChanged || ch.OperationIDChanged || ch.SchemaChanged {
			d.Changed = append(d.Changed, ch)
		}
	}
	sort.Strings(d.Added)
	sort.Strings(d.Removed)
	sort.Slice(d.Changed, func(i, j int) bool { return d.Changed[i].Name < d.Changed[j].Name })
	if d.Added == nil {
		d.Added = []string{}
	}
	if d.Removed == nil {
		d.Removed = []string{}
	}
	if d.Changed == nil {
		d.Changed = []CatalogChange{}
	}
	return d
}

func canonicalJSONEqual(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	var aa, bbv any
	if err := json.Unmarshal(ab, &aa); err != nil {
		return bytes.Equal(ab, bb)
	}
	if err := json.Unmarshal(bb, &bbv); err != nil {
		return bytes.Equal(ab, bb)
	}
	cab, _ := json.Marshal(aa)
	cbb, _ := json.Marshal(bbv)
	return bytes.Equal(cab, cbb)
}
