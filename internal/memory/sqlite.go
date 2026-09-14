package memory

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const sqliteMemorySchema = `
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
CREATE VIRTUAL TABLE IF NOT EXISTS account_memory_fts USING fts5(
  text, content='account_memory', content_rowid='rowid'
);
CREATE TRIGGER IF NOT EXISTS account_memory_ai AFTER INSERT ON account_memory BEGIN
  INSERT INTO account_memory_fts(rowid, text) VALUES (new.rowid, new.text);
END;
CREATE TRIGGER IF NOT EXISTS account_memory_ad AFTER DELETE ON account_memory BEGIN
  INSERT INTO account_memory_fts(account_memory_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
END;
CREATE TRIGGER IF NOT EXISTS account_memory_au AFTER UPDATE ON account_memory BEGIN
  INSERT INTO account_memory_fts(account_memory_fts, rowid, text) VALUES ('delete', old.rowid, old.text);
  INSERT INTO account_memory_fts(rowid, text) VALUES (new.rowid, new.text);
END;
`

// SQLiteStore persists account memory in SQLite with FTS5 search.
type SQLiteStore struct {
	db *sql.DB
}

var _ Store = (*SQLiteStore)(nil)

// OpenSQLite creates a SQLiteStore on db and ensures schema exists.
// db should use MaxOpenConns(1) when shared with other writers on the same file.
func OpenSQLite(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db required")
	}
	if _, err := db.Exec(sqliteMemorySchema); err != nil {
		return nil, fmt.Errorf("migrate account_memory schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func formatMemoryTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseMemoryTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339, s)
}

func scanEntry(row interface {
	Scan(dest ...any) error
}) (Entry, error) {
	var (
		e       Entry
		created string
		updated string
	)
	if err := row.Scan(&e.ID, &e.OwnerID, &e.Key, &e.Text, &e.Source, &created, &updated); err != nil {
		return Entry{}, err
	}
	var err error
	e.CreatedAt, err = parseMemoryTime(created)
	if err != nil {
		return Entry{}, err
	}
	e.UpdatedAt, err = parseMemoryTime(updated)
	if err != nil {
		return Entry{}, err
	}
	return e, nil
}

func (s *SQLiteStore) Upsert(e Entry) (Entry, error) {
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
			`SELECT id FROM account_memory WHERE owner_id = ? AND key = ?`,
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
			`SELECT owner_id FROM account_memory WHERE id = ?`,
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
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
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

func (s *SQLiteStore) updateEntryTx(tx *sql.Tx, id string, e Entry, now time.Time) (Entry, error) {
	var prev Entry
	var created string
	err := tx.QueryRow(
		`SELECT id, owner_id, key, text, source, created_at, updated_at FROM account_memory WHERE id = ?`,
		id,
	).Scan(&prev.ID, &prev.OwnerID, &prev.Key, &prev.Text, &prev.Source, &created, &prev.UpdatedAt)
	if err != nil {
		return Entry{}, err
	}
	prev.CreatedAt, err = parseMemoryTime(created)
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
		`UPDATE account_memory SET key = ?, text = ?, source = ?, updated_at = ? WHERE id = ?`,
		prev.Key, prev.Text, prev.Source, formatMemoryTime(prev.UpdatedAt), id,
	); err != nil {
		return Entry{}, err
	}
	return prev, nil
}

func (s *SQLiteStore) Get(id string) (Entry, error) {
	row := s.db.QueryRow(
		`SELECT id, owner_id, key, text, source, created_at, updated_at FROM account_memory WHERE id = ?`,
		id,
	)
	e, err := scanEntry(row)
	if err == sql.ErrNoRows {
		return Entry{}, fmt.Errorf("memory entry not found")
	}
	return e, err
}

func (s *SQLiteStore) Delete(ownerID, id string) error {
	e, err := s.Get(id)
	if err != nil {
		return err
	}
	if e.OwnerID != ownerID {
		return fmt.Errorf("owner mismatch")
	}
	res, err := s.db.Exec(`DELETE FROM account_memory WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("memory entry not found")
	}
	return nil
}

func (s *SQLiteStore) List(ownerID string, limit, offset int) ([]Entry, error) {
	rows, err := s.db.Query(
		`SELECT id, owner_id, key, text, source, created_at, updated_at
		 FROM account_memory WHERE owner_id = ?
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

func (s *SQLiteStore) Search(ownerID, query string, topK int) ([]Entry, error) {
	if topK <= 0 {
		topK = DefaultTopK
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}

	candidates, err := s.searchFTS(ownerID, q)
	if err != nil || len(candidates) == 0 {
		like, likeErr := s.searchLIKE(ownerID, q)
		if likeErr != nil {
			if err != nil {
				return nil, err
			}
			return nil, likeErr
		}
		candidates = like
	}
	return rankSearchResults(candidates, q, topK), nil
}

func ftsMatchQuery(term string) string {
	term = strings.ReplaceAll(term, `"`, `""`)
	return `"` + term + `"`
}

func (s *SQLiteStore) searchFTS(ownerID, query string) ([]Entry, error) {
	rows, err := s.db.Query(
		`SELECT m.id, m.owner_id, m.key, m.text, m.source, m.created_at, m.updated_at
		 FROM account_memory m
		 INNER JOIN account_memory_fts fts ON fts.rowid = m.rowid
		 WHERE m.owner_id = ? AND fts MATCH ?`,
		ownerID, ftsMatchQuery(query),
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

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func (s *SQLiteStore) searchLIKE(ownerID, query string) ([]Entry, error) {
	pattern := "%" + escapeLike(query) + "%"
	rows, err := s.db.Query(
		`SELECT id, owner_id, key, text, source, created_at, updated_at
		 FROM account_memory
		 WHERE owner_id = ? AND text LIKE ? ESCAPE '\'`,
		ownerID, pattern,
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

func rankSearchResults(candidates []Entry, query string, topK int) []Entry {
	type scored struct {
		e     Entry
		score int
	}
	var hits []scored
	for _, e := range candidates {
		if sc := Rank(query, e.Text); sc > 0 {
			hits = append(hits, scored{e: e, score: sc})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		if hits[i].e.UpdatedAt.Equal(hits[j].e.UpdatedAt) {
			return hits[i].e.ID < hits[j].e.ID
		}
		return hits[i].e.UpdatedAt.After(hits[j].e.UpdatedAt)
	})
	if len(hits) > topK {
		hits = hits[:topK]
	}
	out := make([]Entry, len(hits))
	for i, h := range hits {
		out[i] = h.e
	}
	return out
}

func (s *SQLiteStore) Forget(ownerID, key, text string) (int, error) {
	if key != "" {
		res, err := s.db.Exec(
			`DELETE FROM account_memory WHERE owner_id = ? AND key = ?`,
			ownerID, key,
		)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}
	res, err := s.db.Exec(
		`DELETE FROM account_memory WHERE owner_id = ? AND text = ?`,
		ownerID, text,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
