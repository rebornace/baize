package memory

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const postgresMemorySchema = `
CREATE TABLE IF NOT EXISTS account_memory (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  key TEXT NOT NULL DEFAULT '',
  text TEXT NOT NULL,
  source TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS account_memory_owner_key
  ON account_memory(owner_id, key) WHERE key != '';
`

// PostgresStore persists account memory in PostgreSQL with ILIKE search.
type PostgresStore struct {
	db *sql.DB
}

var _ Store = (*PostgresStore)(nil)

// OpenPostgres creates a PostgresStore on db and ensures schema exists.
func OpenPostgres(db *sql.DB) (*PostgresStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db required")
	}
	if _, err := db.Exec(postgresMemorySchema); err != nil {
		return nil, fmt.Errorf("migrate account_memory schema: %w", err)
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Upsert(e Entry) (Entry, error) {
	if e.OwnerID == "" {
		return Entry{}, fmt.Errorf("owner_id required")
	}
	e.Text = truncateText(e.Text)
	now := time.Now().UTC()

	tx, err := s.db.Begin()
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if e.Key != "" {
		var existingID string
		err := tx.QueryRow(
			`SELECT id FROM account_memory WHERE owner_id = $1 AND key = $2`,
			e.OwnerID, e.Key,
		).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return Entry{}, err
		}
		if existingID != "" {
			out, err := s.updateEntryTx(tx, existingID, e, now)
			if err != nil {
				return Entry{}, err
			}
			if err := tx.Commit(); err != nil {
				return Entry{}, err
			}
			return out, nil
		}
	}

	if e.ID != "" {
		var ownerID string
		err := tx.QueryRow(
			`SELECT owner_id FROM account_memory WHERE id = $1`,
			e.ID,
		).Scan(&ownerID)
		if err == nil {
			if ownerID != e.OwnerID {
				return Entry{}, fmt.Errorf("owner mismatch")
			}
			out, err := s.updateEntryTx(tx, e.ID, e, now)
			if err != nil {
				return Entry{}, err
			}
			if err := tx.Commit(); err != nil {
				return Entry{}, err
			}
			return out, nil
		}
		if err != sql.ErrNoRows {
			return Entry{}, err
		}
	}

	out := e
	out.ID = "mem_" + uuid.NewString()
	if out.Source == "" {
		out.Source = SourceExplicit
	}
	out.CreatedAt = now
	out.UpdatedAt = now
	if _, err := tx.Exec(
		`INSERT INTO account_memory (id, owner_id, key, text, source, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		out.ID, out.OwnerID, out.Key, out.Text, out.Source,
		formatMemoryTime(out.CreatedAt), formatMemoryTime(out.UpdatedAt),
	); err != nil {
		return Entry{}, err
	}
	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return out, nil
}

func (s *PostgresStore) updateEntryTx(tx *sql.Tx, id string, e Entry, now time.Time) (Entry, error) {
	var prev Entry
	var created, updated string
	err := tx.QueryRow(
		`SELECT id, owner_id, key, text, source, created_at, updated_at FROM account_memory WHERE id = $1`,
		id,
	).Scan(&prev.ID, &prev.OwnerID, &prev.Key, &prev.Text, &prev.Source, &created, &updated)
	if err != nil {
		return Entry{}, err
	}
	prev.CreatedAt, err = parseMemoryTime(created)
	if err != nil {
		return Entry{}, err
	}
	prev.UpdatedAt, err = parseMemoryTime(updated)
	if err != nil {
		return Entry{}, err
	}
	prev.Text = e.Text
	prev.Key = e.Key
	if e.Source != "" {
		prev.Source = e.Source
	}
	prev.UpdatedAt = now
	if _, err := tx.Exec(
		`UPDATE account_memory SET key = $1, text = $2, source = $3, updated_at = $4 WHERE id = $5`,
		prev.Key, prev.Text, prev.Source, formatMemoryTime(prev.UpdatedAt), id,
	); err != nil {
		return Entry{}, err
	}
	return prev, nil
}

func (s *PostgresStore) Get(id string) (Entry, error) {
	row := s.db.QueryRow(
		`SELECT id, owner_id, key, text, source, created_at, updated_at FROM account_memory WHERE id = $1`,
		id,
	)
	e, err := scanEntry(row)
	if err == sql.ErrNoRows {
		return Entry{}, fmt.Errorf("memory entry not found")
	}
	return e, err
}

func (s *PostgresStore) Delete(ownerID, id string) error {
	e, err := s.Get(id)
	if err != nil {
		return err
	}
	if e.OwnerID != ownerID {
		return fmt.Errorf("owner mismatch")
	}
	res, err := s.db.Exec(`DELETE FROM account_memory WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("memory entry not found")
	}
	return nil
}

func (s *PostgresStore) List(ownerID string, limit, offset int) ([]Entry, error) {
	rows, err := s.db.Query(
		`SELECT id, owner_id, key, text, source, created_at, updated_at
		 FROM account_memory WHERE owner_id = $1
		 ORDER BY updated_at DESC, id ASC`,
		ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		all = append(all, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if offset > len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func (s *PostgresStore) Search(ownerID, query string, topK int) ([]Entry, error) {
	if topK <= 0 {
		topK = DefaultTopK
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	candidates, err := s.searchILIKE(ownerID, q, topK)
	if err != nil {
		return nil, err
	}
	return rankSearchResults(candidates, q, topK), nil
}

func (s *PostgresStore) searchILIKE(ownerID, query string, topK int) ([]Entry, error) {
	rows, err := s.db.Query(
		`SELECT id, owner_id, key, text, source, created_at, updated_at
		 FROM account_memory
		 WHERE owner_id = $1 AND text ILIKE '%' || $2 || '%' ESCAPE '\'
		 ORDER BY updated_at DESC, id ASC
		 LIMIT $3`,
		ownerID, escapeLike(query), topK,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PostgresStore) Forget(ownerID, key, text string) (int, error) {
	if key != "" {
		res, err := s.db.Exec(
			`DELETE FROM account_memory WHERE owner_id = $1 AND key = $2`,
			ownerID, key,
		)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	res, err := s.db.Exec(
		`DELETE FROM account_memory WHERE owner_id = $1 AND text = $2`,
		ownerID, text,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
