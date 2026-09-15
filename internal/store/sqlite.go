package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/dbutil"
	_ "modernc.org/sqlite"
)

// SQLDialect selects placeholder and DDL behavior for SQLStore.
type SQLDialect string

const (
	DialectSQLite   SQLDialect = "sqlite"
	DialectPostgres SQLDialect = "postgres"
)

// SQLite is a backward-compatible alias for SQLStore.
type SQLite = SQLStore

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY, agent_id TEXT, input TEXT, status TEXT,
  output TEXT, error TEXT, created_at TEXT, hitl_json TEXT,
  conversation_id TEXT, identity_id TEXT,
  lease_until TIMESTAMP
);
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT, type TEXT, timestamp TEXT, data_json TEXT
);
CREATE TABLE IF NOT EXISTS connectors (
  id TEXT PRIMARY KEY,
  type TEXT,
  spec TEXT,
  base_url TEXT,
  require_approval_json TEXT,
  require_login_json TEXT,
  auth_json TEXT,
  mcp_json TEXT,
  execution_callback_url TEXT
);
CREATE TABLE IF NOT EXISTS tools (
  name TEXT PRIMARY KEY,
  connector_id TEXT,
  source TEXT,
  enabled INTEGER,
  title TEXT,
  description TEXT,
  description_custom INTEGER,
  method TEXT,
  path TEXT,
  input_schema_json TEXT,
  require_login INTEGER,
  require_approval INTEGER,
  operation_id TEXT,
  export_mode TEXT
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS inbox_deliveries (
  channel_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  delivery_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  body_hash TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(channel_id, idempotency_key)
);
CREATE TABLE IF NOT EXISTS inbox_threads (
  channel_id TEXT NOT NULL,
  external_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  UNIQUE(channel_id, external_id)
);
CREATE TABLE IF NOT EXISTS webhook_outbox (
  id TEXT PRIMARY KEY,
  delivery_key TEXT NOT NULL UNIQUE,
  run_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  event_index INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  target_url TEXT NOT NULL,
  headers_json TEXT,
  attempt INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 5,
  status TEXT NOT NULL,
  last_error TEXT,
  next_retry_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_webhook_outbox_pending ON webhook_outbox(status, next_retry_at);
CREATE TABLE IF NOT EXISTS channel_outbox (
  id TEXT PRIMARY KEY,
  delivery_key TEXT NOT NULL UNIQUE,
  channel TEXT NOT NULL,
  kind TEXT NOT NULL,
  peer_id TEXT,
  conversation_id TEXT,
  account TEXT,
  run_id TEXT,
  payload_json TEXT NOT NULL,
  blob_keys_json TEXT,
  target_url TEXT NOT NULL,
  attempt INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 5,
  status TEXT NOT NULL,
  last_error TEXT,
  next_retry_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_channel_outbox_pending ON channel_outbox(status, next_retry_at);
CREATE INDEX IF NOT EXISTS idx_channel_outbox_channel_status ON channel_outbox(channel, status);
CREATE TABLE IF NOT EXISTS mcp_export_identities (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  scheme TEXT,
  headers_json TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS mcp_export_keys (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  identity_id TEXT NOT NULL,
  key_hash TEXT NOT NULL UNIQUE,
  prefix TEXT NOT NULL,
  revoked_at TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_mcp_export_keys_identity ON mcp_export_keys(identity_id);
CREATE TABLE IF NOT EXISTS model_profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  provider TEXT,
  base_url TEXT,
  model TEXT,
  api_key TEXT,
  api_key_env TEXT,
  disable_thinking INTEGER,
  thinking_level TEXT NOT NULL DEFAULT 'medium',
  thinking_dialect TEXT NOT NULL DEFAULT 'auto',
  supports_vision INTEGER,
  context_tokens INTEGER NOT NULL DEFAULT 128000,
  auto_tier TEXT NOT NULL DEFAULT 'standard',
  created_at TEXT,
  updated_at TEXT
);
`

// SQLStore is a SQL-backed Store shared by sqlite and postgres drivers.
type SQLStore struct {
	db         *sql.DB
	dialect    SQLDialect
	mu         sync.RWMutex
	agents     map[string]Agent
	connectors map[string]Connector
	tools      map[string]Tool
}

func (s *SQLStore) q(query string) string {
	if s.dialect == DialectPostgres {
		return dbutil.RebindPostgres(query)
	}
	return query
}

func (s *SQLStore) exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(s.q(query), args...)
}

func (s *SQLStore) query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(s.q(query), args...)
}

func (s *SQLStore) queryRow(query string, args ...any) *sql.Row {
	return s.db.QueryRow(s.q(query), args...)
}

// sqliteTimeLayout is a fixed-width (9 fractional digits) RFC3339 layout. SQLite
// compares TEXT timestamps byte-wise, so variable-length fractions (RFC3339Nano
// trims trailing zeros) invert order within the same whole second: e.g.
// ".25Z" < ".2Z" even though 0.250s > 0.200s. Fixed width keeps lexicographic
// order identical to chronological order and still parses back to time.Time.
const sqliteTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

// formatSQLiteTime renders t as a fixed-width RFC3339 string for TEXT/TIMESTAMP
// columns compared lexicographically on SQLite.
func formatSQLiteTime(t time.Time) string {
	return t.UTC().Format(sqliteTimeLayout)
}

// timeArg binds a timestamp for the active dialect. Postgres columns are real
// TIMESTAMPTZ (native time.Time); SQLite stores them as fixed-width RFC3339 text.
func (s *SQLStore) timeArg(t time.Time) any {
	if s.dialect == DialectPostgres {
		return t
	}
	return formatSQLiteTime(t)
}

// OpenSQLite opens (or creates) a SQLite database at path.
// Parent directories are created automatically (e.g. ./data/baize.db).
func OpenSQLite(path string) (*SQLite, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite dir %q: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Serialize access: concurrent writers with modernc/sqlite otherwise hit
	// "database is locked", leaving runs stuck in running with zero events.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pragma busy_timeout: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("pragma journal_mode: %w", err)
	}
	if _, err := db.Exec(sqliteSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	if err := migrateRunsColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate runs columns: %w", err)
	}
	if err := migrateToolsColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate tools columns: %w", err)
	}
	if err := migrateConnectorsColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate connectors columns: %w", err)
	}
	if err := migrateMCPExportKeys(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate mcp export keys: %w", err)
	}
	if err := migrateModelProfilesColumns(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate model_profiles columns: %w", err)
	}
	s := &SQLStore{
		db:         db,
		dialect:    DialectSQLite,
		agents:     map[string]Agent{},
		connectors: map[string]Connector{},
		tools:      map[string]Tool{},
	}
	if err := s.loadConnectorsAndTools(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load connectors and tools: %w", err)
	}
	return s, nil
}

// migrateRunsColumns adds conversation_id / identity_id / passthrough_json to
// existing DBs. Duplicate-column errors from ALTER are ignored.
func migrateRunsColumns(db *sql.DB) error {
	for _, col := range []string{"conversation_id", "identity_id", "passthrough_json", "webhook_json", "model_profile_id", "thinking_level"} {
		_, err := db.Exec(`ALTER TABLE runs ADD COLUMN ` + col + ` TEXT`)
		if err == nil || isDuplicateColumnErr(err) {
			continue
		}
		return err
	}
	if _, err := db.Exec(`ALTER TABLE runs ADD COLUMN lease_until TIMESTAMP`); err != nil && !isDuplicateColumnErr(err) {
		return err
	}
	return nil
}

func migrateConnectorsColumns(db *sql.DB) error {
	for _, q := range []string{
		`ALTER TABLE connectors ADD COLUMN mcp_json TEXT`,
		`ALTER TABLE connectors ADD COLUMN execution_callback_url TEXT`,
		`ALTER TABLE connectors ADD COLUMN import_format TEXT`,
	} {
		_, err := db.Exec(q)
		if err == nil || isDuplicateColumnErr(err) {
			continue
		}
		return err
	}
	return nil
}

func migrateToolsColumns(db *sql.DB) error {
	alters := []string{
		`ALTER TABLE tools ADD COLUMN title TEXT`,
		`ALTER TABLE tools ADD COLUMN description_custom INTEGER`,
		`ALTER TABLE tools ADD COLUMN export_mode TEXT`,
	}
	for _, q := range alters {
		_, err := db.Exec(q)
		if err == nil || isDuplicateColumnErr(err) {
			continue
		}
		return err
	}
	return nil
}

func migrateModelProfilesColumns(db *sql.DB) error {
	if _, err := db.Exec(`ALTER TABLE model_profiles ADD COLUMN context_tokens INTEGER NOT NULL DEFAULT 128000`); err != nil && !isDuplicateColumnErr(err) {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE model_profiles ADD COLUMN auto_tier TEXT NOT NULL DEFAULT 'standard'`); err != nil && !isDuplicateColumnErr(err) {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE model_profiles ADD COLUMN thinking_level TEXT NOT NULL DEFAULT 'medium'`); err != nil && !isDuplicateColumnErr(err) {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE model_profiles ADD COLUMN thinking_dialect TEXT NOT NULL DEFAULT 'auto'`); err != nil && !isDuplicateColumnErr(err) {
		return err
	}
	// Backfill: legacy disable_thinking=1 → off; ALTER DEFAULT already set medium/auto.
	if _, err := db.Exec(`UPDATE model_profiles SET thinking_level='off' WHERE disable_thinking=1`); err != nil {
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

// Close closes the underlying database.
func (s *SQLStore) Close() error {
	return s.db.Close()
}

// SQLBackend exposes the shared SQL database for sibling stores (conversation, identity, artifacts).
type SQLBackend interface {
	DB() *sql.DB
	Dialect() SQLDialect
}

// DB returns the underlying *sql.DB so sibling stores (conversation / identity)
// can share the same connection pool and pragmas configured by OpenSQLite.
// Callers must not Close the returned handle; close the SQLStore instead.
func (s *SQLStore) DB() *sql.DB {
	return s.db
}

func (s *SQLStore) Dialect() SQLDialect {
	return s.dialect
}
