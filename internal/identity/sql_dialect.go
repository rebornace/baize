package identity

import (
	"database/sql"
	"fmt"

	"github.com/rebornace/baize/internal/dbutil"
	"github.com/rebornace/baize/internal/store"
)

func openSQLStore(db *sql.DB, dialect store.SQLDialect) (*SQLiteStore, error) {
	if dialect == store.DialectSQLite || dialect == "" {
		if _, err := db.Exec(sqliteIdentitiesSchema); err != nil {
			return nil, err
		}
	}
	return &SQLiteStore{db: db, dialect: dialect}, nil
}

func (s *SQLiteStore) q(query string) string {
	if s.dialect == store.DialectPostgres {
		return dbutil.RebindPostgres(query)
	}
	return query
}

func (s *SQLiteStore) exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(s.q(query), args...)
}

func (s *SQLiteStore) query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(s.q(query), args...)
}

func (s *SQLiteStore) queryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(s.q(query), args...)
}

func (s *SQLiteStore) txExec(tx *sql.Tx, query string, args ...any) (sql.Result, error) {
	return tx.Exec(s.q(query), args...)
}

func (s *SQLiteStore) txQueryRow(tx *sql.Tx, query string, args ...any) *sql.Row {
	return tx.QueryRow(s.q(query), args...)
}

// OpenSQL opens an identity store on db for the given SQL dialect.
func OpenSQL(db *sql.DB, dialect store.SQLDialect) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	s, err := openSQLStore(db, dialect)
	if err != nil {
		return nil, fmt.Errorf("migrate identities schema: %w", err)
	}
	if err := s.migrateWebIdentitiesToWorkspace(); err != nil {
		return nil, fmt.Errorf("migrate web identities to workspace: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrateWebIdentitiesToWorkspace() error {
	rows, err := s.query(
		`SELECT id, conversation_id, scheme, subject FROM identities ORDER BY updated_at DESC, id DESC`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rec struct {
		id, conv, scheme, subject string
	}
	var list []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.conv, &r.scheme, &r.subject); err != nil {
			return err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range list {
		if r.conv == WorkspaceScope || isolatedIdentityConversation(r.conv) {
			continue
		}
		var existingID string
		err := s.queryRow(
			`SELECT id FROM identities WHERE conversation_id = ? AND scheme = ? AND subject = ?`,
			WorkspaceScope, r.scheme, r.subject,
		).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if existingID != "" && existingID != r.id {
			if _, err := s.exec(`DELETE FROM identities WHERE id = ?`, r.id); err != nil {
				return err
			}
			continue
		}
		if _, err := s.exec(`UPDATE identities SET conversation_id = ? WHERE id = ?`, WorkspaceScope, r.id); err != nil {
			return err
		}
	}
	return nil
}
