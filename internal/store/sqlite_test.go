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
