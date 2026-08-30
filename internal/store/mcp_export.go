package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

func cloneMCPExportIdentity(id MCPExportIdentity) MCPExportIdentity {
	if id.Headers != nil {
		id.Headers = cloneStringMap(id.Headers)
	}
	return id
}

func cloneMCPExportKey(k MCPExportKey) MCPExportKey {
	if k.RevokedAt != nil {
		t := *k.RevokedAt
		k.RevokedAt = &t
	}
	return k
}

func cloneStringMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func encodeHeadersJSON(headers map[string]string) ([]byte, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	return json.Marshal(headers)
}

func decodeHeadersJSON(raw []byte) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func formatOptionalTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.UTC().Format(time.RFC3339Nano), Valid: true}
}

func parseOptionalTime(raw sql.NullString) (*time.Time, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw.String)
	if err != nil {
		return nil, fmt.Errorf("parse time: %w", err)
	}
	utc := t.UTC()
	return &utc, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "constraint failed")
}

// migrateMCPExportKeys adds a UNIQUE constraint on key_hash for DBs created before
// the column-level UNIQUE landed in the CREATE TABLE DDL.
func migrateMCPExportKeys(db *sql.DB) error {
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uq_mcp_export_keys_hash ON mcp_export_keys(key_hash)`); err != nil {
		if isNoSuchTableErr(err) {
			return nil
		}
		return err
	}
	_, _ = db.Exec(`DROP INDEX IF EXISTS idx_mcp_export_keys_hash`)
	return nil
}

func isNoSuchTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "does not exist")
}

func (s *Memory) UpsertMCPExportIdentity(id MCPExportIdentity) error {
	if id.ID == "" {
		return fmt.Errorf("mcp export identity id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mcpExportIDs == nil {
		s.mcpExportIDs = map[string]MCPExportIdentity{}
	}
	now := time.Now().UTC()
	existing, exists := s.mcpExportIDs[id.ID]
	if exists {
		id.CreatedAt = existing.CreatedAt
	} else if id.CreatedAt.IsZero() {
		id.CreatedAt = now
	}
	id.UpdatedAt = now
	s.mcpExportIDs[id.ID] = cloneMCPExportIdentity(id)
	return nil
}

func (s *Memory) GetMCPExportIdentity(id string) (MCPExportIdentity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	got, ok := s.mcpExportIDs[id]
	if !ok {
		return MCPExportIdentity{}, ErrMCPExportIdentityNotFound
	}
	return cloneMCPExportIdentity(got), nil
}

func (s *Memory) ListMCPExportIdentities() ([]MCPExportIdentity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.mcpExportIDs))
	for id := range s.mcpExportIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]MCPExportIdentity, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneMCPExportIdentity(s.mcpExportIDs[id]))
	}
	return out, nil
}

func (s *Memory) DeleteMCPExportIdentity(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.mcpExportIDs[id]; !ok {
		return ErrMCPExportIdentityNotFound
	}
	delete(s.mcpExportIDs, id)
	for keyID, k := range s.mcpExportKeys {
		if k.IdentityID == id {
			delete(s.mcpExportKeys, keyID)
		}
	}
	return nil
}

func (s *Memory) InsertMCPExportKey(k MCPExportKey) error {
	if k.ID == "" {
		k.ID = uuid.NewString()
	}
	if k.IdentityID == "" {
		return fmt.Errorf("mcp export key identity_id is required")
	}
	if k.KeyHash == "" {
		return fmt.Errorf("mcp export key hash is required")
	}
	if k.Prefix == "" {
		return fmt.Errorf("mcp export key prefix is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.mcpExportIDs[k.IdentityID]; !ok {
		return ErrMCPExportIdentityNotFound
	}
	if s.mcpExportKeys == nil {
		s.mcpExportKeys = map[string]MCPExportKey{}
	}
	for _, existing := range s.mcpExportKeys {
		if existing.KeyHash == k.KeyHash {
			return ErrMCPExportKeyHashExists
		}
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now().UTC()
	}
	s.mcpExportKeys[k.ID] = cloneMCPExportKey(k)
	return nil
}

func (s *Memory) GetMCPExportKey(id string) (MCPExportKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.mcpExportKeys[id]
	if !ok {
		return MCPExportKey{}, ErrMCPExportKeyNotFound
	}
	return cloneMCPExportKey(k), nil
}

func (s *Memory) ListMCPExportKeys(identityID string) ([]MCPExportKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.mcpExportKeys))
	for id, k := range s.mcpExportKeys {
		if identityID != "" && k.IdentityID != identityID {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]MCPExportKey, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneMCPExportKey(s.mcpExportKeys[id]))
	}
	return out, nil
}

func (s *Memory) RevokeMCPExportKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.mcpExportKeys[id]
	if !ok {
		return ErrMCPExportKeyNotFound
	}
	if k.RevokedAt != nil {
		return nil
	}
	now := time.Now().UTC()
	k.RevokedAt = &now
	s.mcpExportKeys[id] = k
	return nil
}

func (s *Memory) LookupMCPExportKeyByHash(hash string) (*MCPExportKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, k := range s.mcpExportKeys {
		if k.KeyHash != hash {
			continue
		}
		if k.RevokedAt != nil {
			return nil, ErrMCPExportKeyNotFound
		}
		cp := cloneMCPExportKey(k)
		return &cp, nil
	}
	return nil, ErrMCPExportKeyNotFound
}

func scanMCPExportIdentity(sc interface {
	Scan(dest ...any) error
}) (MCPExportIdentity, error) {
	var id MCPExportIdentity
	var headersRaw sql.NullString
	var created, updated string
	if err := sc.Scan(&id.ID, &id.Name, &id.Scheme, &headersRaw, &created, &updated); err != nil {
		return MCPExportIdentity{}, err
	}
	if headersRaw.Valid && headersRaw.String != "" {
		headers, err := decodeHeadersJSON([]byte(headersRaw.String))
		if err != nil {
			return MCPExportIdentity{}, fmt.Errorf("parse headers_json: %w", err)
		}
		id.Headers = headers
	}
	var err error
	id.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return MCPExportIdentity{}, fmt.Errorf("parse created_at: %w", err)
	}
	id.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return MCPExportIdentity{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return id, nil
}

func scanMCPExportKey(sc interface {
	Scan(dest ...any) error
}) (MCPExportKey, error) {
	var k MCPExportKey
	var revoked sql.NullString
	var created string
	if err := sc.Scan(&k.ID, &k.Name, &k.IdentityID, &k.KeyHash, &k.Prefix, &revoked, &created); err != nil {
		return MCPExportKey{}, err
	}
	var err error
	k.RevokedAt, err = parseOptionalTime(revoked)
	if err != nil {
		return MCPExportKey{}, err
	}
	k.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return MCPExportKey{}, fmt.Errorf("parse created_at: %w", err)
	}
	return k, nil
}

func (s *SQLStore) UpsertMCPExportIdentity(id MCPExportIdentity) error {
	if id.ID == "" {
		return fmt.Errorf("mcp export identity id is required")
	}
	now := time.Now().UTC()
	var existingCreated sql.NullString
	err := s.queryRow(`SELECT created_at FROM mcp_export_identities WHERE id = ?`, id.ID).Scan(&existingCreated)
	var createdAt time.Time
	switch {
	case err == sql.ErrNoRows:
		if id.CreatedAt.IsZero() {
			createdAt = now
		} else {
			createdAt = id.CreatedAt.UTC()
		}
	case err != nil:
		return err
	default:
		createdAt, err = time.Parse(time.RFC3339Nano, existingCreated.String)
		if err != nil {
			return fmt.Errorf("parse existing created_at: %w", err)
		}
	}
	headersJSON, err := encodeHeadersJSON(id.Headers)
	if err != nil {
		return err
	}
	_, err = s.exec(`INSERT INTO mcp_export_identities (id, name, scheme, headers_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			scheme = excluded.scheme,
			headers_json = excluded.headers_json,
			updated_at = excluded.updated_at`,
		id.ID, id.Name, id.Scheme, string(headersJSON),
		createdAt.UTC().Format(time.RFC3339Nano),
		now.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLStore) GetMCPExportIdentity(id string) (MCPExportIdentity, error) {
	row := s.queryRow(`SELECT id, name, scheme, headers_json, created_at, updated_at
		FROM mcp_export_identities WHERE id = ?`, id)
	got, err := scanMCPExportIdentity(row)
	if err == sql.ErrNoRows {
		return MCPExportIdentity{}, ErrMCPExportIdentityNotFound
	}
	return got, err
}

func (s *SQLStore) ListMCPExportIdentities() ([]MCPExportIdentity, error) {
	rows, err := s.query(`SELECT id, name, scheme, headers_json, created_at, updated_at
		FROM mcp_export_identities ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MCPExportIdentity
	for rows.Next() {
		id, err := scanMCPExportIdentity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *SQLStore) DeleteMCPExportIdentity(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(s.q(`DELETE FROM mcp_export_keys WHERE identity_id = ?`), id); err != nil {
		return err
	}
	res, err := tx.Exec(s.q(`DELETE FROM mcp_export_identities WHERE id = ?`), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMCPExportIdentityNotFound
	}
	return tx.Commit()
}

func (s *SQLStore) InsertMCPExportKey(k MCPExportKey) error {
	if k.ID == "" {
		k.ID = uuid.NewString()
	}
	if k.IdentityID == "" {
		return fmt.Errorf("mcp export key identity_id is required")
	}
	if k.KeyHash == "" {
		return fmt.Errorf("mcp export key hash is required")
	}
	if k.Prefix == "" {
		return fmt.Errorf("mcp export key prefix is required")
	}
	if _, err := s.GetMCPExportIdentity(k.IdentityID); err != nil {
		return err
	}
	if k.CreatedAt.IsZero() {
		k.CreatedAt = time.Now().UTC()
	}
	_, err := s.exec(`INSERT INTO mcp_export_keys (id, name, identity_id, key_hash, prefix, revoked_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.Name, k.IdentityID, k.KeyHash, k.Prefix,
		formatOptionalTime(k.RevokedAt), k.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil && isUniqueViolation(err) {
		return ErrMCPExportKeyHashExists
	}
	return err
}

func (s *SQLStore) GetMCPExportKey(id string) (MCPExportKey, error) {
	row := s.queryRow(`SELECT id, name, identity_id, key_hash, prefix, revoked_at, created_at
		FROM mcp_export_keys WHERE id = ?`, id)
	k, err := scanMCPExportKey(row)
	if err == sql.ErrNoRows {
		return MCPExportKey{}, ErrMCPExportKeyNotFound
	}
	return k, err
}

func (s *SQLStore) ListMCPExportKeys(identityID string) ([]MCPExportKey, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if identityID == "" {
		rows, err = s.query(`SELECT id, name, identity_id, key_hash, prefix, revoked_at, created_at
			FROM mcp_export_keys ORDER BY created_at ASC`)
	} else {
		rows, err = s.query(`SELECT id, name, identity_id, key_hash, prefix, revoked_at, created_at
			FROM mcp_export_keys WHERE identity_id = ? ORDER BY created_at ASC`, identityID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MCPExportKey
	for rows.Next() {
		k, err := scanMCPExportKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *SQLStore) RevokeMCPExportKey(id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.exec(`UPDATE mcp_export_keys SET revoked_at = ?
		WHERE id = ? AND revoked_at IS NULL`, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var revoked sql.NullString
		err := s.queryRow(`SELECT revoked_at FROM mcp_export_keys WHERE id = ?`, id).Scan(&revoked)
		if err == sql.ErrNoRows {
			return ErrMCPExportKeyNotFound
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLStore) LookupMCPExportKeyByHash(hash string) (*MCPExportKey, error) {
	row := s.queryRow(`SELECT id, name, identity_id, key_hash, prefix, revoked_at, created_at
		FROM mcp_export_keys WHERE key_hash = ? AND revoked_at IS NULL`, hash)
	k, err := scanMCPExportKey(row)
	if err == sql.ErrNoRows {
		return nil, ErrMCPExportKeyNotFound
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}
