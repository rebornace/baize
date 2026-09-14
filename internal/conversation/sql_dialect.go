package conversation

import (
	"database/sql"
	"fmt"
	"strings"

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
		if err := migrateMessagesThinkingColumns(db, store.DialectSQLite); err != nil {
			return nil, err
		}
	} else if dialect == store.DialectPostgres {
		// messages table is created by store.OpenPostgres; ensure meta for shared DB.
		const pgMeta = `
CREATE TABLE IF NOT EXISTS conversation_meta (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  source TEXT NOT NULL,
  title TEXT,
  channel_peer TEXT,
  updated_at TEXT NOT NULL
);`
		if _, err := db.Exec(pgMeta); err != nil {
			return nil, err
		}
		const pgSummaries = `
CREATE TABLE IF NOT EXISTS conversation_summaries (
  conversation_id TEXT PRIMARY KEY,
  summary TEXT NOT NULL,
  covers_through_message_id TEXT NOT NULL,
  covers_through_order INTEGER NOT NULL,
  updated_at TEXT NOT NULL
);`
		if _, err := db.Exec(pgSummaries); err != nil {
			return nil, err
		}
		if err := migrateMessagesThinkingColumns(db, store.DialectPostgres); err != nil {
			return nil, err
		}
	}
	return &SQLiteStore{db: db, dialect: dialect}, nil
}

// migrateMessagesThinkingColumns adds thinking / thinking_redacted for DBs created
// before those columns existed. SQLite lacks IF NOT EXISTS on ADD COLUMN.
func migrateMessagesThinkingColumns(db *sql.DB, dialect store.SQLDialect) error {
	if dialect == store.DialectPostgres {
		if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN IF NOT EXISTS thinking TEXT`); err != nil {
			return fmt.Errorf("migrate messages thinking: %w", err)
		}
		if _, err := db.Exec(`ALTER TABLE messages ADD COLUMN IF NOT EXISTS thinking_redacted BOOLEAN NOT NULL DEFAULT false`); err != nil {
			return fmt.Errorf("migrate messages thinking_redacted: %w", err)
		}
		return nil
	}
	for _, q := range []string{
		`ALTER TABLE messages ADD COLUMN thinking TEXT`,
		`ALTER TABLE messages ADD COLUMN thinking_redacted INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.Exec(q); err != nil && !isDuplicateColumnErr(err) {
			return err
		}
	}
	return nil
}

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
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
