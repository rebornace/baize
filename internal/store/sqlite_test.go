package store_test

import (
	"database/sql"
	"io"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/store"
	_ "modernc.org/sqlite"
)

func TestSQLiteCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "data", "baize.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatalf("Open should create parent dirs: %v", err)
	}
	if c, ok := s.(io.Closer); ok {
		_ = c.Close()
	}
}

func TestSQLiteConcurrentCreateAndAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})

	const n = 32
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			r, err := s.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
			if err != nil {
				errCh <- err
				return
			}
			if err := s.AppendEvent(r.ID, store.Event{Type: "run.started"}); err != nil {
				errCh <- err
				return
			}
			evs, err := s.ListEvents(r.ID)
			if err != nil {
				errCh <- err
				return
			}
			if len(evs) == 0 {
				errCh <- errEmptyEvents
				return
			}
			errCh <- nil
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent sqlite op: %v", err)
		}
	}
}

var errEmptyEvents = errString("expected run.started event")

type errString string

func (e errString) Error() string { return string(e) }

func TestSQLitePersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baize.db")

	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateRun(store.CreateRunInput{AgentID: "ticket-agent", Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRun(r.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(r.ID, store.Event{
		Type: "hitl.waiting",
		Data: map[string]any{"tool_name": "create_ticket"},
	}); err != nil {
		t.Fatal(err)
	}
	payload := &store.HITLPayload{
		Prompt:   "确认创建工单？",
		ToolName: "create_ticket",
		Arguments: map[string]any{
			"title": "VPN 故障",
		},
	}
	if err := s.SetHITL(r.ID, payload); err != nil {
		t.Fatal(err)
	}
	runID := r.ID
	if c, ok := s.(io.Closer); ok {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		t.Fatal("sqlite store must implement io.Closer")
	}

	s2, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s2.(io.Closer); ok {
			_ = c.Close()
		}
	})

	got, err := s2.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%q want waiting_human", got.Status)
	}
	if got.Input != "创建工单" || got.AgentID != "ticket-agent" {
		t.Fatalf("run=%+v", got)
	}

	evs, err := s2.ListEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "hitl.waiting" {
		t.Fatalf("events=%+v", evs)
	}
	if evs[0].Data["tool_name"] != "create_ticket" {
		t.Fatalf("event data=%+v", evs[0].Data)
	}

	hitl, err := s2.GetHITL(runID)
	if err != nil {
		t.Fatal(err)
	}
	if hitl == nil || hitl.Prompt != payload.Prompt || hitl.ToolName != payload.ToolName {
		t.Fatalf("hitl=%+v", hitl)
	}
	if hitl.Arguments["title"] != "VPN 故障" {
		t.Fatalf("arguments=%+v", hitl.Arguments)
	}
}

func TestSQLiteCreateRunPersistsPassthroughHeaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baize.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})

	r, err := s.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "hi",
		PassthroughHeaders: map[string]string{"Authorization": "Bearer SECRET"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PassthroughHeaders["Authorization"] != "Bearer SECRET" {
		t.Fatalf("%+v", got.PassthroughHeaders)
	}

	if err := s.SetPassthroughHeaders(r.ID, map[string]string{"Authorization": "Bearer NEW"}); err != nil {
		t.Fatal(err)
	}
	got2, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.PassthroughHeaders["Authorization"] != "Bearer NEW" {
		t.Fatalf("%+v", got2.PassthroughHeaders)
	}
}

func TestSQLiteCreateRunPersistsConversationAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baize.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})

	r, err := s.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "hi", ConversationID: "conv_1", IdentityID: "idt_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID != "conv_1" || got.IdentityID != "idt_1" {
		t.Fatalf("%+v", got)
	}
}

func TestSQLiteMigratesExistingRunsTable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")

	// Seed a pre-migration schema without conversation_id / identity_id.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE runs (
  id TEXT PRIMARY KEY, agent_id TEXT, input TEXT, status TEXT,
  output TEXT, error TEXT, created_at TEXT, hitl_json TEXT
);
INSERT INTO runs (id, agent_id, input, status, output, error, created_at, hitl_json)
VALUES ('run_legacy', 'a', 'old', 'running', '', '', '2026-01-01T00:00:00Z', NULL);
`)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatalf("Open should ALTER legacy schema: %v", err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})

	got, err := s.GetRun("run_legacy")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConversationID != "" || got.IdentityID != "" {
		t.Fatalf("legacy run should have empty ids, got %+v", got)
	}

	r, err := s.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "new", ConversationID: "conv_x", IdentityID: "idt_x",
	})
	if err != nil {
		t.Fatal(err)
	}
	got2, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got2.ConversationID != "conv_x" || got2.IdentityID != "idt_x" {
		t.Fatalf("%+v", got2)
	}
}

func TestOpenMemory(t *testing.T) {
	s, err := store.Open("memory", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*store.Memory); !ok {
		t.Fatalf("want *Memory, got %T", s)
	}
}

func TestOpenUnknownDriver(t *testing.T) {
	_, err := store.Open("postgres", "")
	if err == nil {
		t.Fatal("expected error for unknown driver")
	}
}

func TestSQLiteDeleteConnectorCascadesTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delete-connector.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	testDeleteConnectorCascadesTools(t, s)
	if c, ok := s.(io.Closer); ok {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}

	s2, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s2.(io.Closer); ok {
			_ = c.Close()
		}
	})
	if _, err := s2.GetConnector("c1"); err == nil {
		t.Fatal("connector should not persist after delete")
	}
	if byC := s2.ListToolsByConnector("c1"); len(byC) != 0 {
		t.Fatalf("tools for c1=%+v want empty after reopen", byC)
	}
	if _, err := s2.GetTool("keep_tool"); err != nil {
		t.Fatalf("keep_tool should persist: %v", err)
	}
}

func TestSQLiteToolCatalogRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s.UpsertConnector(store.Connector{ID: "c1", Type: "openapi", Spec: "s.yaml", BaseURL: "http://x"})
	s.UpsertTool(store.Tool{
		ConnectorID: "c1",
		Name:        "create_ticket",
		Source:      store.ToolSourceSpec,
		Enabled:     true,
		Method:      "POST",
		Path:        "/tickets",
		Description: "create",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}}},
	})
	if c, ok := s.(io.Closer); ok {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}

	s2, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s2.(io.Closer); ok {
			_ = c.Close()
		}
	})

	got, err := s2.GetTool("create_ticket")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Source != store.ToolSourceSpec || got.Method != "POST" || got.Path != "/tickets" {
		t.Fatalf("got=%+v", got)
	}
	if got.Description != "create" {
		t.Fatalf("description=%q", got.Description)
	}
	if got.InputSchema == nil || got.InputSchema["type"] != "object" {
		t.Fatalf("input_schema=%+v", got.InputSchema)
	}
	props, ok := got.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties not a map: %+v", got.InputSchema["properties"])
	}
	if _, ok := props["title"].(map[string]any); !ok {
		t.Fatalf("title prop missing: %+v", props["title"])
	}

	cs := s2.ListConnectors()
	if len(cs) != 1 || cs[0].ID != "c1" {
		t.Fatalf("connectors=%+v", cs)
	}
	byC := s2.ListToolsByConnector("c1")
	if len(byC) != 1 || byC[0].Name != "create_ticket" {
		t.Fatalf("byC=%+v", byC)
	}
}

func TestSQLiteToolTitleAndCustomRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools-title.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s.UpsertConnector(store.Connector{ID: "c1", Type: "openapi", BaseURL: "http://x"})
	s.UpsertTool(store.Tool{
		ConnectorID:       "c1",
		Name:              "create_ticket",
		Source:            store.ToolSourceSpec,
		Enabled:           true,
		Title:             "建工单",
		Description:       "人改的说明",
		DescriptionCustom: true,
		Method:            "POST",
		Path:              "/tickets",
	})
	if c, ok := s.(io.Closer); ok {
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
	s2, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s2.(io.Closer); ok {
			_ = c.Close()
		}
	})
	got, err := s2.GetTool("create_ticket")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "建工单" || !got.DescriptionCustom || got.Description != "人改的说明" {
		t.Fatalf("got=%+v", got)
	}
}

func TestSQLiteToolColumnsMigrateFromOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-tools.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE tools (
		name TEXT PRIMARY KEY, connector_id TEXT, source TEXT, enabled INTEGER,
		description TEXT, method TEXT, path TEXT, input_schema_json TEXT,
		require_login INTEGER, require_approval INTEGER, operation_id TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO tools (name, connector_id, source, enabled, description, method, path, require_login, require_approval, operation_id)
		VALUES ('create_ticket', 'c1', 'spec', 1, 'from spec', 'POST', '/tickets', 0, 0, 'create_ticket')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})
	got, err := s.GetTool("create_ticket")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "" || got.DescriptionCustom || got.Description != "from spec" {
		t.Fatalf("migrated=%+v", got)
	}
}

func TestSQLiteConnectorMCPJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.db")
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := store.Connector{
		ID:   "m1",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "stdio",
			Command:   "echo",
			Args:      []string{"ok"},
			Env:       map[string]string{"K": "v"},
		},
	}
	s.UpsertConnector(c)
	got, err := s.GetConnector("m1")
	if err != nil {
		t.Fatal(err)
	}
	if got.MCP.Transport != "stdio" || got.MCP.Command != "echo" {
		t.Fatalf("mcp=%+v", got.MCP)
	}
}
