package identity

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const sqliteIdentitiesSchema = `
CREATE TABLE IF NOT EXISTS identities (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  label TEXT,
  scheme TEXT,
  subject TEXT,
  source TEXT,
  is_default INTEGER NOT NULL DEFAULT 0,
  headers_json TEXT,
  claims_json TEXT,
  last_used_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_identities_conv ON identities(conversation_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_identities_conv_scheme_subject
  ON identities(conversation_id, scheme, subject);
`

// SQLiteStore persists per-conversation identities in SQLite.
//
// Callers must configure the shared *sql.DB for serialized access before
// OpenSQLite: SetMaxOpenConns(1), SetMaxIdleConns(1), and PRAGMA busy_timeout
// (see store.OpenSQLite). Without these settings, concurrent writers may hit
// "database is locked".
type SQLiteStore struct {
	db *sql.DB
}

var _ Store = (*SQLiteStore)(nil)

// OpenSQLite creates a SQLiteStore on db, ensuring the identities schema
// exists. db must already use MaxOpenConns(1) and busy_timeout, or be the same
// *sql.DB opened by store.OpenSQLite. OpenSQLite does not apply connection-pool
// or pragma settings.
func OpenSQLite(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if _, err := db.Exec(sqliteIdentitiesSchema); err != nil {
		return nil, fmt.Errorf("migrate identities schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Upsert inserts or updates an identity. Same conversation + scheme + subject
// reuses the existing id, matching MemoryStore semantics.
func (s *SQLiteStore) Upsert(conversationID string, id Identity) (string, error) {
	now := time.Now().UTC()

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		existingID     string
		existingClaims sql.NullString
	)
	err = tx.QueryRow(
		`SELECT id, claims_json FROM identities WHERE conversation_id = ? AND scheme = ? AND subject = ?`,
		conversationID, id.Scheme, id.Subject,
	).Scan(&existingID, &existingClaims)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	if existingID != "" {
		// Reuse existing id; update mutable fields.
		updatedAt := id.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}

		// ClaimsSummary is only replaced when the caller supplies a non-nil map,
		// matching MemoryStore which keeps prior claims when id.ClaimsSummary is nil.
		var claimsJSON any
		if id.ClaimsSummary != nil {
			b, err := json.Marshal(id.ClaimsSummary)
			if err != nil {
				return "", err
			}
			claimsJSON = string(b)
		} else if existingClaims.Valid && existingClaims.String != "" && existingClaims.String != "null" {
			claimsJSON = existingClaims.String
		}

		headersJSON, err := marshalHeaders(id.CredentialHeaders)
		if err != nil {
			return "", err
		}

		if _, err := tx.Exec(
			`UPDATE identities SET label = ?, headers_json = ?, claims_json = ?, updated_at = ? WHERE id = ?`,
			id.Label, headersJSON, claimsJSON, updatedAt.Format(time.RFC3339Nano), existingID,
		); err != nil {
			return "", err
		}

		if id.IsDefault {
			if _, err := tx.Exec(
				`UPDATE identities SET is_default = 0 WHERE conversation_id = ? AND scheme = ? AND id <> ?`,
				conversationID, id.Scheme, existingID,
			); err != nil {
				return "", err
			}
			if _, err := tx.Exec(
				`UPDATE identities SET is_default = 1 WHERE id = ?`,
				existingID,
			); err != nil {
				return "", err
			}
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return existingID, nil
	}

	// Insert new identity.
	newID := "idt_" + uuid.NewString()
	createdAt := id.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := id.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}

	headersJSON, err := marshalHeaders(id.CredentialHeaders)
	if err != nil {
		return "", err
	}
	var claimsJSON any
	if id.ClaimsSummary != nil {
		b, err := json.Marshal(id.ClaimsSummary)
		if err != nil {
			return "", err
		}
		claimsJSON = string(b)
	}

	if _, err := tx.Exec(
		`INSERT INTO identities (id, conversation_id, label, scheme, subject, source, is_default, headers_json, claims_json, last_used_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		newID, conversationID, id.Label, id.Scheme, id.Subject, id.Source, boolToInt(id.IsDefault),
		headersJSON, claimsJSON, createdAt.Format(time.RFC3339Nano), updatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return "", err
	}

	if id.IsDefault {
		if _, err := tx.Exec(
			`UPDATE identities SET is_default = 0 WHERE conversation_id = ? AND scheme = ? AND id <> ?`,
			conversationID, id.Scheme, newID,
		); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return newID, nil
}

// List returns all identities for a conversation.
func (s *SQLiteStore) List(conversationID string) []Identity {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, label, scheme, subject, source, is_default, headers_json, claims_json, last_used_at, created_at, updated_at
		 FROM identities WHERE conversation_id = ? ORDER BY created_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []Identity
	for rows.Next() {
		item, err := scanIdentity(rows)
		if err != nil {
			return nil
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return out
}

// Get returns a single identity by id within a conversation.
func (s *SQLiteStore) Get(conversationID, id string) (Identity, error) {
	row := s.db.QueryRow(
		`SELECT id, conversation_id, label, scheme, subject, source, is_default, headers_json, claims_json, last_used_at, created_at, updated_at
		 FROM identities WHERE conversation_id = ? AND id = ?`,
		conversationID, id,
	)
	item, err := scanIdentity(row)
	if err == sql.ErrNoRows {
		return Identity{}, fmt.Errorf("identity not found")
	}
	if err != nil {
		return Identity{}, err
	}
	return item, nil
}

// ListPublic returns sanitized public views without credential headers.
func (s *SQLiteStore) ListPublic(conversationID string) []PublicView {
	rows, err := s.db.Query(
		`SELECT id, label, scheme, source, is_default, claims_json, last_used_at
		 FROM identities WHERE conversation_id = ? ORDER BY created_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []PublicView
	for rows.Next() {
		var (
			v          PublicView
			isDefault  int
			claimsJSON sql.NullString
			lastUsedAt sql.NullString
		)
		if err := rows.Scan(&v.ID, &v.Label, &v.Scheme, &v.Source, &isDefault, &claimsJSON, &lastUsedAt); err != nil {
			return nil
		}
		v.IsDefault = isDefault != 0
		if claimsJSON.Valid && claimsJSON.String != "" && claimsJSON.String != "null" {
			if err := json.Unmarshal([]byte(claimsJSON.String), &v.ClaimsSummary); err != nil {
				return nil
			}
		}
		if lastUsedAt.Valid && lastUsedAt.String != "" {
			ts, err := time.Parse(time.RFC3339Nano, lastUsedAt.String)
			if err != nil {
				ts, err = time.Parse(time.RFC3339, lastUsedAt.String)
				if err != nil {
					return nil
				}
			}
			v.LastUsedAt = &ts
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return out
}

// Delete removes an identity from a conversation.
func (s *SQLiteStore) Delete(conversationID, id string) error {
	res, err := s.db.Exec(
		`DELETE FROM identities WHERE conversation_id = ? AND id = ?`,
		conversationID, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("identity not found")
	}
	return nil
}

// SetDefault marks an identity as the default for its scheme within a
// conversation, clearing IsDefault on other identities of the same scheme.
func (s *SQLiteStore) SetDefault(conversationID, id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var scheme string
	err = tx.QueryRow(
		`SELECT scheme FROM identities WHERE conversation_id = ? AND id = ?`,
		conversationID, id,
	).Scan(&scheme)
	if err == sql.ErrNoRows {
		return fmt.Errorf("identity not found")
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		`UPDATE identities SET is_default = 0 WHERE conversation_id = ? AND scheme = ?`,
		conversationID, scheme,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE identities SET is_default = 1 WHERE conversation_id = ? AND id = ?`,
		conversationID, id,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearCaptured removes login_capture and manual identities for a conversation.
func (s *SQLiteStore) ClearCaptured(conversationID string) {
	_, _ = s.db.Exec(
		`DELETE FROM identities WHERE conversation_id = ? AND source IN (?, ?)`,
		conversationID, SourceLoginCapture, SourceManual,
	)
}

// Touch updates LastUsedAt for an identity.
func (s *SQLiteStore) Touch(conversationID, id string) error {
	res, err := s.db.Exec(
		`UPDATE identities SET last_used_at = ? WHERE conversation_id = ? AND id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), conversationID, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("identity not found")
	}
	return nil
}

func marshalHeaders(h map[string]string) (any, error) {
	if h == nil {
		return nil, nil
	}
	b, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type scanner interface {
	Scan(dest ...any) error
}

func scanIdentity(sc scanner) (Identity, error) {
	var (
		id           Identity
		convID       string
		isDefault    int
		headersJSON  sql.NullString
		claimsJSON   sql.NullString
		lastUsedAt   sql.NullString
		createdAt    string
		updatedAt    string
	)
	if err := sc.Scan(
		&id.ID, &convID, &id.Label, &id.Scheme, &id.Subject, &id.Source,
		&isDefault, &headersJSON, &claimsJSON, &lastUsedAt, &createdAt, &updatedAt,
	); err != nil {
		return Identity{}, err
	}
	id.IsDefault = isDefault != 0
	if headersJSON.Valid && headersJSON.String != "" && headersJSON.String != "null" {
		if err := json.Unmarshal([]byte(headersJSON.String), &id.CredentialHeaders); err != nil {
			return Identity{}, fmt.Errorf("parse headers_json: %w", err)
		}
	}
	if claimsJSON.Valid && claimsJSON.String != "" && claimsJSON.String != "null" {
		if err := json.Unmarshal([]byte(claimsJSON.String), &id.ClaimsSummary); err != nil {
			return Identity{}, fmt.Errorf("parse claims_json: %w", err)
		}
	}
	if lastUsedAt.Valid && lastUsedAt.String != "" {
		ts, err := time.Parse(time.RFC3339Nano, lastUsedAt.String)
		if err != nil {
			ts, err = time.Parse(time.RFC3339, lastUsedAt.String)
			if err != nil {
				return Identity{}, fmt.Errorf("parse last_used_at: %w", err)
			}
		}
		id.LastUsedAt = ts
	}
	created, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		created, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return Identity{}, fmt.Errorf("parse created_at: %w", err)
		}
	}
	id.CreatedAt = created
	updated, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		updated, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return Identity{}, fmt.Errorf("parse updated_at: %w", err)
		}
	}
	id.UpdatedAt = updated
	return id, nil
}
