package conversation

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/dbutil"
	"github.com/rebornace/baize/internal/store"
)

func openSQLStore(db *sql.DB, dialect store.SQLDialect) (*SQLiteStore, error) {
	if db == nil {
		return nil, sql.ErrConnDone
	}
	switch dialect {
	case store.DialectSQLite, "":
		if _, err := db.Exec(sqliteMessagesSchema); err != nil {
			return nil, err
		}
		if err := migrateMessagesThinkingColumns(db, store.DialectSQLite); err != nil {
			return nil, err
		}
		if err := migrateConversationProjections(db); err != nil {
			return nil, err
		}
		if err := migrateChatWorkspaces(db, store.DialectSQLite); err != nil {
			return nil, err
		}
	case store.DialectPostgres:
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
		if err := migrateConversationProjections(db); err != nil {
			return nil, err
		}
		if err := migrateChatWorkspaces(db, store.DialectPostgres); err != nil {
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

func migrateConversationProjections(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS conversation_projections (
  conversation_id TEXT PRIMARY KEY,
  pins TEXT NOT NULL DEFAULT '[]',
  summary TEXT NOT NULL DEFAULT '',
  dropped TEXT NOT NULL DEFAULT '[]',
  revision INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
);`)
	return err
}

func migrateChatWorkspaces(db *sql.DB, dialect store.SQLDialect) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if dialect == store.DialectPostgres {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chat_workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
)`); err != nil {
			return fmt.Errorf("chat_workspaces: %w", err)
		}
		if _, err := db.Exec(`INSERT INTO chat_workspaces (id, name, created_at) VALUES ($1, $2, $3)
			ON CONFLICT (id) DO NOTHING`, DefaultWorkspaceID, DefaultWorkspaceName, now); err != nil {
			return fmt.Errorf("default workspace: %w", err)
		}
		if _, err := db.Exec(`ALTER TABLE conversation_meta ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("meta workspace_id: %w", err)
		}
		if _, err := db.Exec(`UPDATE conversation_meta SET workspace_id = $1 WHERE (source = 'ui' OR source = '') AND (workspace_id = '' OR workspace_id IS NULL)`, DefaultWorkspaceID); err != nil {
			return fmt.Errorf("backfill ui workspace_id: %w", err)
		}
		return nil
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS chat_workspaces (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL
)`); err != nil {
		return fmt.Errorf("chat_workspaces: %w", err)
	}
	if _, err := db.Exec(`INSERT OR IGNORE INTO chat_workspaces (id, name, created_at) VALUES (?, ?, ?)`,
		DefaultWorkspaceID, DefaultWorkspaceName, now); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE conversation_meta ADD COLUMN workspace_id TEXT NOT NULL DEFAULT ''`); err != nil && !isDuplicateColumnErr(err) {
		return err
	}
	if _, err := db.Exec(`UPDATE conversation_meta SET workspace_id = ? WHERE (source = 'ui' OR source = '') AND workspace_id = ''`, DefaultWorkspaceID); err != nil {
		return err
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
