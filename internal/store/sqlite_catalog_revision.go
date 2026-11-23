package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

func (s *SQLStore) PutCatalogRevision(r ConnectorCatalogRevision) error {
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	}
	diff := r.DiffJSON
	if len(diff) == 0 {
		diff = json.RawMessage(`{}`)
	}
	var hashes sql.NullString
	if len(r.ToolHashes) > 0 {
		b, err := json.Marshal(r.ToolHashes)
		if err != nil {
			return err
		}
		hashes = sql.NullString{String: string(b), Valid: true}
	}
	_, err := s.exec(
		`INSERT INTO connector_catalog_revisions (connector_id, revision, at, diff_json, tool_hashes_json)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(connector_id, revision) DO UPDATE SET
		   at=excluded.at, diff_json=excluded.diff_json, tool_hashes_json=excluded.tool_hashes_json`,
		r.ConnectorID, r.Revision, s.timeArg(r.At), string(diff), hashes,
	)
	return err
}

func (s *SQLStore) ListCatalogRevisions(connectorID string, limit int) ([]ConnectorCatalogRevision, error) {
	q := `SELECT connector_id, revision, at, diff_json, tool_hashes_json
	      FROM connector_catalog_revisions WHERE connector_id = ? ORDER BY revision DESC`
	args := []any{connectorID}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectorCatalogRevision
	for rows.Next() {
		var r ConnectorCatalogRevision
		var atStr string
		var diff string
		var hashes sql.NullString
		if err := rows.Scan(&r.ConnectorID, &r.Revision, &atStr, &diff, &hashes); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, atStr); err == nil {
			r.At = t
		} else if t, err := time.Parse(sqliteTimeLayout, atStr); err == nil {
			r.At = t
		}
		r.DiffJSON = json.RawMessage(diff)
		if hashes.Valid && hashes.String != "" && hashes.String != "null" {
			_ = json.Unmarshal([]byte(hashes.String), &r.ToolHashes)
		}
		out = append(out, r)
	}
	if out == nil {
		out = []ConnectorCatalogRevision{}
	}
	return out, rows.Err()
}

func (s *SQLStore) PutMCPExportInvocation(inv MCPExportInvocation) error {
	if inv.ID == "" {
		inv.ID = "mei_inv_" + uuid.NewString()
	}
	if inv.At.IsZero() {
		inv.At = time.Now().UTC()
	}
	isErr := 0
	if inv.IsError {
		isErr = 1
	}
	_, err := s.exec(
		`INSERT INTO mcp_export_invocations
		 (id, at, key_id, identity_id, tool_name, connector_id, contract_hash, catalog_revision, is_error)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, s.timeArg(inv.At), inv.KeyID, inv.IdentityID, inv.ToolName,
		inv.ConnectorID, inv.ContractHash, inv.CatalogRevision, isErr,
	)
	return err
}

func (s *SQLStore) ListMCPExportInvocations(limit int) ([]MCPExportInvocation, error) {
	q := `SELECT id, at, key_id, identity_id, tool_name, connector_id, contract_hash, catalog_revision, is_error
	      FROM mcp_export_invocations ORDER BY at DESC`
	args := []any{}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MCPExportInvocation
	for rows.Next() {
		var inv MCPExportInvocation
		var atStr string
		var connectorID, contractHash sql.NullString
		var isErr int
		if err := rows.Scan(&inv.ID, &atStr, &inv.KeyID, &inv.IdentityID, &inv.ToolName,
			&connectorID, &contractHash, &inv.CatalogRevision, &isErr); err != nil {
			return nil, err
		}
		if t, err := time.Parse(time.RFC3339, atStr); err == nil {
			inv.At = t
		} else if t, err := time.Parse(sqliteTimeLayout, atStr); err == nil {
			inv.At = t
		}
		inv.ConnectorID = connectorID.String
		inv.ContractHash = contractHash.String
		inv.IsError = isErr != 0
		out = append(out, inv)
	}
	if out == nil {
		out = []MCPExportInvocation{}
	}
	return out, rows.Err()
}
