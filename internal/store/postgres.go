package store

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

const postgresSchema = `
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY, agent_id TEXT, input TEXT, status TEXT,
  output TEXT, error TEXT, created_at TEXT, hitl_json TEXT,
  conversation_id TEXT, identity_id TEXT,
  passthrough_json TEXT, webhook_json TEXT
);
CREATE TABLE IF NOT EXISTS events (
  id BIGSERIAL PRIMARY KEY,
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
  execution_callback_url TEXT,
  import_format TEXT
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
  operation_id TEXT
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
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  run_id TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_conv_created ON messages(conversation_id, created_at);
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
CREATE TABLE IF NOT EXISTS artifacts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  created_at BIGINT NOT NULL
);
`

func init() {
	RegisterDriver("postgres", func(opts OpenOptions) (Store, error) {
		return OpenPostgres(opts.DSN)
	})
}

// OpenPostgres opens a PostgreSQL-backed store at dsn.
func OpenPostgres(dsn string) (*SQLStore, error) {
	if dsn == "" {
		return nil, fmt.Errorf("postgres dsn is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	if _, err := db.Exec(postgresSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate postgres schema: %w", err)
	}
	s := &SQLStore{
		db:         db,
		dialect:    DialectPostgres,
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
