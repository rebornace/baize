package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY, agent_id TEXT, input TEXT, status TEXT,
  output TEXT, error TEXT, created_at TEXT, hitl_json TEXT,
  conversation_id TEXT, identity_id TEXT
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
  operation_id TEXT
);
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL
);
`

// SQLite is a file-backed Store implementation.
type SQLite struct {
	db         *sql.DB
	mu         sync.RWMutex
	agents     map[string]Agent
	connectors map[string]Connector
	tools      map[string]Tool
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
	s := &SQLite{
		db:         db,
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

// loadConnectorsAndTools reads existing connector and tool rows from the DB
// into the in-memory maps so reads can stay lock-free and consistent with the
// existing runs/events pattern.
func (s *SQLite) loadConnectorsAndTools() error {
	rows, err := s.db.Query(`SELECT id, type, spec, base_url, require_approval_json, require_login_json, auth_json, mcp_json, execution_callback_url FROM connectors`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c Connector
		var requireApproval, requireLogin, auth, mcp, execCallback sql.NullString
		if err := rows.Scan(&c.ID, &c.Type, &c.Spec, &c.BaseURL, &requireApproval, &requireLogin, &auth, &mcp, &execCallback); err != nil {
			rows.Close()
			return err
		}
		if requireApproval.Valid && requireApproval.String != "" && requireApproval.String != "null" {
			if err := json.Unmarshal([]byte(requireApproval.String), &c.RequireApproval); err != nil {
				rows.Close()
				return fmt.Errorf("parse require_approval_json: %w", err)
			}
		}
		if requireLogin.Valid && requireLogin.String != "" && requireLogin.String != "null" {
			if err := json.Unmarshal([]byte(requireLogin.String), &c.RequireLogin); err != nil {
				rows.Close()
				return fmt.Errorf("parse require_login_json: %w", err)
			}
		}
		if auth.Valid && auth.String != "" && auth.String != "null" {
			if err := json.Unmarshal([]byte(auth.String), &c.Auth); err != nil {
				rows.Close()
				return fmt.Errorf("parse auth_json: %w", err)
			}
		}
		if mcp.Valid && mcp.String != "" && mcp.String != "null" {
			if err := json.Unmarshal([]byte(mcp.String), &c.MCP); err != nil {
				rows.Close()
				return fmt.Errorf("parse mcp_json: %w", err)
			}
		}
		if execCallback.Valid {
			c.ExecutionCallbackURL = execCallback.String
		}
		s.connectors[c.ID] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	trows, err := s.db.Query(`SELECT name, connector_id, source, enabled, title, description, description_custom, method, path, input_schema_json, require_login, require_approval, operation_id FROM tools`)
	if err != nil {
		return err
	}
	for trows.Next() {
		var t Tool
		var enabled, requireLogin, requireApproval int
		var descriptionCustom sql.NullInt64
		var title, description, method, path, inputSchema, operationID sql.NullString
		if err := trows.Scan(&t.Name, &t.ConnectorID, &t.Source, &enabled, &title, &description, &descriptionCustom, &method, &path, &inputSchema, &requireLogin, &requireApproval, &operationID); err != nil {
			trows.Close()
			return err
		}
		t.Enabled = enabled != 0
		t.RequireLogin = requireLogin != 0
		t.RequireApproval = requireApproval != 0
		t.Title = title.String
		t.Description = description.String
		t.DescriptionCustom = descriptionCustom.Valid && descriptionCustom.Int64 != 0
		t.Method = method.String
		t.Path = path.String
		t.OperationID = operationID.String
		if inputSchema.Valid && inputSchema.String != "" && inputSchema.String != "null" {
			if err := json.Unmarshal([]byte(inputSchema.String), &t.InputSchema); err != nil {
				trows.Close()
				return fmt.Errorf("parse input_schema_json: %w", err)
			}
		}
		s.tools[t.Name] = t
	}
	trows.Close()
	return trows.Err()
}

// migrateRunsColumns adds conversation_id / identity_id / passthrough_json to
// existing DBs. Duplicate-column errors from ALTER are ignored.
func migrateRunsColumns(db *sql.DB) error {
	for _, col := range []string{"conversation_id", "identity_id", "passthrough_json", "webhook_json"} {
		_, err := db.Exec(`ALTER TABLE runs ADD COLUMN ` + col + ` TEXT`)
		if err == nil || isDuplicateColumnErr(err) {
			continue
		}
		return err
	}
	return nil
}

func migrateConnectorsColumns(db *sql.DB) error {
	for _, q := range []string{
		`ALTER TABLE connectors ADD COLUMN mcp_json TEXT`,
		`ALTER TABLE connectors ADD COLUMN execution_callback_url TEXT`,
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

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "duplicate column")
}

// Close closes the underlying database.
func (s *SQLite) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB so sibling stores (conversation / identity)
// can share the same connection pool and pragmas configured by OpenSQLite.
// Callers must not Close the returned handle; close the *SQLite instead.
func (s *SQLite) DB() *sql.DB {
	return s.db
}

func (s *SQLite) UpsertAgent(a Agent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.Skills != nil {
		a.Skills = append([]string(nil), a.Skills...)
	}
	s.agents[a.ID] = a
}

func (s *SQLite) ListAgents() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Agent, 0, len(ids))
	for _, id := range ids {
		out = append(out, cloneAgent(s.agents[id]))
	}
	return out
}

func (s *SQLite) GetAgent(id string) (Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.agents[id]
	if !ok {
		return Agent{}, fmt.Errorf("agent not found")
	}
	return cloneAgent(a), nil
}

func (s *SQLite) UpsertConnector(c Connector) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connectors[c.ID] = c
	var requireApproval, requireLogin, auth, mcp sql.NullString
	if len(c.RequireApproval) > 0 {
		if b, err := json.Marshal(c.RequireApproval); err == nil {
			requireApproval = sql.NullString{String: string(b), Valid: true}
		}
	}
	if len(c.RequireLogin) > 0 {
		if b, err := json.Marshal(c.RequireLogin); err == nil {
			requireLogin = sql.NullString{String: string(b), Valid: true}
		}
	}
	if c.Auth.Mode != "" {
		if b, err := json.Marshal(c.Auth); err == nil {
			auth = sql.NullString{String: string(b), Valid: true}
		}
	}
	if c.MCP.Transport != "" {
		if b, err := json.Marshal(c.MCP); err == nil {
			mcp = sql.NullString{String: string(b), Valid: true}
		}
	}
	_, _ = s.db.Exec(
		`INSERT INTO connectors (id, type, spec, base_url, require_approval_json, require_login_json, auth_json, mcp_json, execution_callback_url)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET type=excluded.type, spec=excluded.spec, base_url=excluded.base_url,
		   require_approval_json=excluded.require_approval_json,
		   require_login_json=excluded.require_login_json,
		   auth_json=excluded.auth_json,
		   mcp_json=excluded.mcp_json,
		   execution_callback_url=excluded.execution_callback_url`,
		c.ID, c.Type, c.Spec, c.BaseURL, requireApproval, requireLogin, auth, mcp, c.ExecutionCallbackURL,
	)
}

func (s *SQLite) GetConnector(id string) (Connector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.connectors[id]
	if !ok {
		return Connector{}, fmt.Errorf("connector not found")
	}
	return c, nil
}

func (s *SQLite) ListConnectors() []Connector {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, 0, len(s.connectors))
	for id := range s.connectors {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Connector, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.connectors[id])
	}
	return out
}

func (s *SQLite) UpsertTool(t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools[t.Name] = t
	var inputSchema sql.NullString
	if t.InputSchema != nil {
		if b, err := json.Marshal(t.InputSchema); err == nil {
			inputSchema = sql.NullString{String: string(b), Valid: true}
		}
	}
	enabled := 0
	if t.Enabled {
		enabled = 1
	}
	requireLogin := 0
	if t.RequireLogin {
		requireLogin = 1
	}
	requireApproval := 0
	if t.RequireApproval {
		requireApproval = 1
	}
	descriptionCustom := 0
	if t.DescriptionCustom {
		descriptionCustom = 1
	}
	_, _ = s.db.Exec(
		`INSERT INTO tools (name, connector_id, source, enabled, title, description, description_custom, method, path, input_schema_json, require_login, require_approval, operation_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET connector_id=excluded.connector_id, source=excluded.source,
		   enabled=excluded.enabled, title=excluded.title, description=excluded.description,
		   description_custom=excluded.description_custom, method=excluded.method,
		   path=excluded.path, input_schema_json=excluded.input_schema_json,
		   require_login=excluded.require_login, require_approval=excluded.require_approval,
		   operation_id=excluded.operation_id`,
		t.Name, t.ConnectorID, t.Source, enabled, t.Title, t.Description, descriptionCustom, t.Method, t.Path, inputSchema, requireLogin, requireApproval, t.OperationID,
	)
}

func (s *SQLite) GetTool(name string) (Tool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tools[name]
	if !ok {
		return Tool{}, fmt.Errorf("tool not found")
	}
	return t, nil
}

func (s *SQLite) ListTools() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.tools))
	for n := range s.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, s.tools[n])
	}
	return out
}

func (s *SQLite) ListToolsByConnector(id string) []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.tools))
	for n, t := range s.tools {
		if t.ConnectorID == id {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, s.tools[n])
	}
	return out
}

func (s *SQLite) DeleteTool(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[name]; !ok {
		return fmt.Errorf("tool not found")
	}
	delete(s.tools, name)
	_, _ = s.db.Exec(`DELETE FROM tools WHERE name = ?`, name)
	return nil
}

func (s *SQLite) ReplaceConnectorTools(connectorID string, tools []Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, t := range s.tools {
		if t.ConnectorID == connectorID {
			delete(s.tools, name)
		}
	}
	_, _ = s.db.Exec(`DELETE FROM tools WHERE connector_id = ?`, connectorID)
	for _, t := range tools {
		if t.ConnectorID == "" {
			t.ConnectorID = connectorID
		}
		s.tools[t.Name] = t
		var inputSchema sql.NullString
		if t.InputSchema != nil {
			if b, err := json.Marshal(t.InputSchema); err == nil {
				inputSchema = sql.NullString{String: string(b), Valid: true}
			}
		}
		enabled := 0
		if t.Enabled {
			enabled = 1
		}
		requireLogin := 0
		if t.RequireLogin {
			requireLogin = 1
		}
		requireApproval := 0
		if t.RequireApproval {
			requireApproval = 1
		}
		descriptionCustom := 0
		if t.DescriptionCustom {
			descriptionCustom = 1
		}
		_, _ = s.db.Exec(
			`INSERT INTO tools (name, connector_id, source, enabled, title, description, description_custom, method, path, input_schema_json, require_login, require_approval, operation_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET connector_id=excluded.connector_id, source=excluded.source,
			   enabled=excluded.enabled, title=excluded.title, description=excluded.description,
			   description_custom=excluded.description_custom, method=excluded.method,
			   path=excluded.path, input_schema_json=excluded.input_schema_json,
			   require_login=excluded.require_login, require_approval=excluded.require_approval,
			   operation_id=excluded.operation_id`,
			t.Name, t.ConnectorID, t.Source, enabled, t.Title, t.Description, descriptionCustom, t.Method, t.Path, inputSchema, requireLogin, requireApproval, t.OperationID,
		)
	}
}

func (s *SQLite) CreateRun(in CreateRunInput) (*Run, error) {
	id := "run_" + uuid.NewString()
	now := time.Now().UTC()
	r := &Run{
		ID:                 id,
		AgentID:            in.AgentID,
		Input:              in.Input,
		Status:             StatusRunning,
		CreatedAt:          now,
		ConversationID:     in.ConversationID,
		IdentityID:         in.IdentityID,
		PassthroughHeaders: cloneHeaders(in.PassthroughHeaders),
		WebhookConfig:      cloneWebhookConfig(in.WebhookConfig),
	}
	var passthroughSQL, webhookSQL sql.NullString
	if len(r.PassthroughHeaders) > 0 {
		b, err := json.Marshal(r.PassthroughHeaders)
		if err != nil {
			return nil, err
		}
		passthroughSQL = sql.NullString{String: string(b), Valid: true}
	}
	if r.WebhookConfig != nil {
		b, err := json.Marshal(r.WebhookConfig)
		if err != nil {
			return nil, err
		}
		webhookSQL = sql.NullString{String: string(b), Valid: true}
	}
	_, err := s.db.Exec(
		`INSERT INTO runs (id, agent_id, input, status, output, error, created_at, hitl_json, conversation_id, identity_id, passthrough_json, webhook_json)
		 VALUES (?, ?, ?, ?, '', '', ?, NULL, ?, ?, ?, ?)`,
		r.ID, r.AgentID, r.Input, string(r.Status), r.CreatedAt.Format(time.RFC3339Nano),
		r.ConversationID, r.IdentityID, passthroughSQL, webhookSQL,
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *SQLite) GetRun(id string) (*Run, error) {
	var r Run
	var status, createdAt string
	var conversationID, identityID, passthroughSQL, webhookSQL sql.NullString
	err := s.db.QueryRow(
		`SELECT id, agent_id, input, status, output, error, created_at, conversation_id, identity_id, passthrough_json, webhook_json FROM runs WHERE id = ?`,
		id,
	).Scan(&r.ID, &r.AgentID, &r.Input, &status, &r.Output, &r.Error, &createdAt, &conversationID, &identityID, &passthroughSQL, &webhookSQL)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	r.Status = Status(status)
	if conversationID.Valid {
		r.ConversationID = conversationID.String
	}
	if identityID.Valid {
		r.IdentityID = identityID.String
	}
	if passthroughSQL.Valid && passthroughSQL.String != "" && passthroughSQL.String != "null" {
		if err := json.Unmarshal([]byte(passthroughSQL.String), &r.PassthroughHeaders); err != nil {
			return nil, fmt.Errorf("parse passthrough_json: %w", err)
		}
	}
	if webhookSQL.Valid && webhookSQL.String != "" && webhookSQL.String != "null" {
		var wc WebhookConfig
		if err := json.Unmarshal([]byte(webhookSQL.String), &wc); err != nil {
			return nil, fmt.Errorf("parse webhook_json: %w", err)
		}
		r.WebhookConfig = &wc
	}
	ts, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
	}
	r.CreatedAt = ts
	return &r, nil
}

func (s *SQLite) SetPassthroughHeaders(id string, headers map[string]string) error {
	var passthroughSQL sql.NullString
	if len(headers) > 0 {
		b, err := json.Marshal(headers)
		if err != nil {
			return err
		}
		passthroughSQL = sql.NullString{String: string(b), Valid: true}
	}
	res, err := s.db.Exec(
		`UPDATE runs SET passthrough_json = ? WHERE id = ?`,
		passthroughSQL, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("run not found")
	}
	return nil
}

func (s *SQLite) UpdateRun(id string, status Status, output, errMsg string) error {
	res, err := s.db.Exec(
		`UPDATE runs SET status = ?, output = ?, error = ? WHERE id = ?`,
		string(status), output, errMsg, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("run not found")
	}
	return nil
}

func (s *SQLite) AppendEvent(runID string, ev Event) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	ev.Timestamp = time.Now().UTC()
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO events (run_id, type, timestamp, data_json) VALUES (?, ?, ?, ?)`,
		runID, ev.Type, ev.Timestamp.Format(time.RFC3339Nano), string(dataJSON),
	)
	return err
}

func (s *SQLite) ListEvents(runID string) ([]Event, error) {
	if _, err := s.GetRun(runID); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT type, timestamp, data_json FROM events WHERE run_id = ? ORDER BY id ASC`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var ev Event
		var ts, dataJSON string
		if err := rows.Scan(&ev.Type, &ts, &dataJSON); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, ts)
			if err != nil {
				return nil, fmt.Errorf("parse event timestamp: %w", err)
			}
		}
		ev.Timestamp = parsed
		if dataJSON != "" && dataJSON != "null" {
			if err := json.Unmarshal([]byte(dataJSON), &ev.Data); err != nil {
				return nil, err
			}
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Event{}
	}
	return out, nil
}

func (s *SQLite) SetHITL(runID string, payload *HITLPayload) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	var hitlSQL sql.NullString
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		hitlSQL = sql.NullString{String: string(b), Valid: true}
	}
	_, err := s.db.Exec(`UPDATE runs SET hitl_json = ? WHERE id = ?`, hitlSQL, runID)
	return err
}

func (s *SQLite) GetHITL(runID string) (*HITLPayload, error) {
	var hitlSQL sql.NullString
	err := s.db.QueryRow(`SELECT hitl_json FROM runs WHERE id = ?`, runID).Scan(&hitlSQL)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	if !hitlSQL.Valid || hitlSQL.String == "" || hitlSQL.String == "null" {
		return nil, nil
	}
	var p HITLPayload
	if err := json.Unmarshal([]byte(hitlSQL.String), &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *SQLite) GetSetting(key string) ([]byte, bool, error) {
	var value sql.NullString
	err := s.db.QueryRow(`SELECT value_json FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !value.Valid || value.String == "" {
		return nil, false, nil
	}
	return []byte(value.String), true, nil
}

func (s *SQLite) UpsertSetting(key string, jsonRaw []byte) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value_json) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json`,
		key, string(jsonRaw),
	)
	return err
}
