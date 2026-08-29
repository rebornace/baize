package conversation

import (
	"database/sql"
	"fmt"

	"github.com/rebornace/baize/internal/dbutil"
	"github.com/rebornace/baize/internal/store"
)

func openSQLStore(db *sql.DB, dialect store.SQLDialect) (*SQLiteStore, error) {
	if db == nil {
		return nil, sql.ErrConnDone
	}
	if dialect == store.DialectSQLite || dialect == "" {
		if _, err := db.Exec(sqliteMessagesSchema); err != nil {
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

// OpenSQL opens a message store on db for the given SQL dialect.
func OpenSQL(db *sql.DB, dialect store.SQLDialect) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	s, err := openSQLStore(db, dialect)
	if err != nil {
		return nil, fmt.Errorf("migrate messages schema: %w", err)
	}
	return s, nil
}
