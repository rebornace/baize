package store

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
)

func (s *Memory) PutCatalogRevision(r ConnectorCatalogRevision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalogRevisions == nil {
		s.catalogRevisions = map[string][]ConnectorCatalogRevision{}
	}
	r = cloneCatalogRevision(r)
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	s.catalogRevisions[r.ConnectorID] = append(s.catalogRevisions[r.ConnectorID], r)
	return nil
}

func (s *Memory) ListCatalogRevisions(connectorID string, limit int) ([]ConnectorCatalogRevision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	list := s.catalogRevisions[connectorID]
	if len(list) == 0 {
		return []ConnectorCatalogRevision{}, nil
	}
	// Newest first.
	out := make([]ConnectorCatalogRevision, len(list))
	for i := range list {
		out[len(list)-1-i] = cloneCatalogRevision(list[i])
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Memory) PutMCPExportInvocation(inv MCPExportInvocation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv = cloneMCPExportInvocation(inv)
	if inv.ID == "" {
		inv.ID = "mei_inv_" + uuid.NewString()
	}
	if inv.At.IsZero() {
		inv.At = time.Now().UTC()
	}
	s.mcpExportInvocations = append(s.mcpExportInvocations, inv)
	return nil
}

func (s *Memory) ListMCPExportInvocations(limit int) ([]MCPExportInvocation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := len(s.mcpExportInvocations)
	if n == 0 {
		return []MCPExportInvocation{}, nil
	}
	out := make([]MCPExportInvocation, 0, n)
	for i := n - 1; i >= 0; i-- {
		out = append(out, cloneMCPExportInvocation(s.mcpExportInvocations[i]))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func cloneCatalogRevision(r ConnectorCatalogRevision) ConnectorCatalogRevision {
	out := r
	if r.DiffJSON != nil {
		out.DiffJSON = append(json.RawMessage(nil), r.DiffJSON...)
	}
	if r.ToolHashes != nil {
		out.ToolHashes = make(map[string]string, len(r.ToolHashes))
		for k, v := range r.ToolHashes {
			out.ToolHashes[k] = v
		}
	}
	return out
}

func cloneMCPExportInvocation(inv MCPExportInvocation) MCPExportInvocation {
	return inv
}

// SortedToolHashNames returns tool hash keys sorted (for stable tests/APIs).
func SortedToolHashNames(m map[string]string) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
