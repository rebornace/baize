package store_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestSQLitePersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baize.db")

	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateRun("ticket-agent", "创建工单")
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
