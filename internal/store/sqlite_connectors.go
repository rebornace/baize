package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
)

// loadConnectorsAndTools reads existing connector and tool rows from the DB
// into the in-memory maps so reads can stay lock-free and consistent with the
// existing runs/events pattern.
func (s *SQLStore) loadConnectorsAndTools() error {
	rows, err := s.query(`SELECT id, type, spec, base_url, require_approval_json, require_login_json, auth_json, mcp_json, execution_callback_url, import_format FROM connectors`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c Connector
		var requireApproval, requireLogin, auth, mcp, execCallback, importFormat sql.NullString
		if err := rows.Scan(&c.ID, &c.Type, &c.Spec, &c.BaseURL, &requireApproval, &requireLogin, &auth, &mcp, &execCallback, &importFormat); err != nil {
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
		if importFormat.Valid {
			c.ImportFormat = importFormat.String
		}
		s.connectors[c.ID] = c
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	trows, err := s.query(`SELECT name, connector_id, source, enabled, title, description, description_custom, method, path, input_schema_json, require_login, require_approval, operation_id, export_mode FROM tools`)
	if err != nil {
		return err
	}
	for trows.Next() {
		var t Tool
		var enabled, requireLogin, requireApproval int
		var descriptionCustom sql.NullInt64
		var title, description, method, path, inputSchema, operationID, exportMode sql.NullString
		if err := trows.Scan(&t.Name, &t.ConnectorID, &t.Source, &enabled, &title, &description, &descriptionCustom, &method, &path, &inputSchema, &requireLogin, &requireApproval, &operationID, &exportMode); err != nil {
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
		t.Export = exportMode.String
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

func (s *SQLStore) UpsertConnector(c Connector) {
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
	_, _ = s.exec(
		`INSERT INTO connectors (id, type, spec, base_url, require_approval_json, require_login_json, auth_json, mcp_json, execution_callback_url, import_format)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET type=excluded.type, spec=excluded.spec, base_url=excluded.base_url,
		   require_approval_json=excluded.require_approval_json,
		   require_login_json=excluded.require_login_json,
		   auth_json=excluded.auth_json,
		   mcp_json=excluded.mcp_json,
		   execution_callback_url=excluded.execution_callback_url,
		   import_format=excluded.import_format`,
		c.ID, c.Type, c.Spec, c.BaseURL, requireApproval, requireLogin, auth, mcp, c.ExecutionCallbackURL, c.ImportFormat,
	)
}

func (s *SQLStore) GetConnector(id string) (Connector, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.connectors[id]
	if !ok {
		return Connector{}, fmt.Errorf("connector not found")
	}
	return c, nil
}

func (s *SQLStore) DeleteConnector(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM tools WHERE connector_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM connectors WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("connector not found")
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	delete(s.connectors, id)
	for name, t := range s.tools {
		if t.ConnectorID == id {
			delete(s.tools, name)
		}
	}
	return nil
}

func (s *SQLStore) ListConnectors() []Connector {
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

func (s *SQLStore) UpsertTool(t Tool) {
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
	_, _ = s.exec(
		`INSERT INTO tools (name, connector_id, source, enabled, title, description, description_custom, method, path, input_schema_json, require_login, require_approval, operation_id, export_mode)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET connector_id=excluded.connector_id, source=excluded.source,
		   enabled=excluded.enabled, title=excluded.title, description=excluded.description,
		   description_custom=excluded.description_custom, method=excluded.method,
		   path=excluded.path, input_schema_json=excluded.input_schema_json,
		   require_login=excluded.require_login, require_approval=excluded.require_approval,
		   operation_id=excluded.operation_id, export_mode=excluded.export_mode`,
		t.Name, t.ConnectorID, t.Source, enabled, t.Title, t.Description, descriptionCustom, t.Method, t.Path, inputSchema, requireLogin, requireApproval, t.OperationID, t.Export,
	)
}

func (s *SQLStore) GetTool(name string) (Tool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tools[name]
	if !ok {
		return Tool{}, fmt.Errorf("tool not found")
	}
	return t, nil
}

func (s *SQLStore) ListTools() []Tool {
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

func (s *SQLStore) ListToolsByConnector(id string) []Tool {
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

func (s *SQLStore) DeleteTool(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[name]; !ok {
		return fmt.Errorf("tool not found")
	}
	delete(s.tools, name)
	_, _ = s.exec(`DELETE FROM tools WHERE name = ?`, name)
	return nil
}

func (s *SQLStore) ReplaceConnectorTools(connectorID string, tools []Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, t := range s.tools {
		if t.ConnectorID == connectorID {
			delete(s.tools, name)
		}
	}
	_, _ = s.exec(`DELETE FROM tools WHERE connector_id = ?`, connectorID)
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
		_, _ = s.exec(
			`INSERT INTO tools (name, connector_id, source, enabled, title, description, description_custom, method, path, input_schema_json, require_login, require_approval, operation_id, export_mode)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(name) DO UPDATE SET connector_id=excluded.connector_id, source=excluded.source,
			   enabled=excluded.enabled, title=excluded.title, description=excluded.description,
			   description_custom=excluded.description_custom, method=excluded.method,
			   path=excluded.path, input_schema_json=excluded.input_schema_json,
			   require_login=excluded.require_login, require_approval=excluded.require_approval,
			   operation_id=excluded.operation_id, export_mode=excluded.export_mode`,
			t.Name, t.ConnectorID, t.Source, enabled, t.Title, t.Description, descriptionCustom, t.Method, t.Path, inputSchema, requireLogin, requireApproval, t.OperationID, t.Export,
		)
	}
}
