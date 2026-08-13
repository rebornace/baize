package store_test

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestCreateAndGetRun(t *testing.T) {
	s := store.NewMemory()
	r, err := s.CreateRun(store.CreateRunInput{AgentID: "ticket-agent", Input: "创建工单"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "" || r.Status != store.StatusRunning {
		t.Fatalf("got %+v", r)
	}
	got, err := s.GetRun(r.ID)
	if err != nil || got.Input != "创建工单" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	s.AppendEvent(r.ID, store.Event{Type: "run.started"})
	evs, _ := s.ListEvents(r.ID)
	if len(evs) != 1 || evs[0].Type != "run.started" {
		t.Fatalf("events=%+v", evs)
	}
}

func TestCreateRunPersistsConversationAndIdentity(t *testing.T) {
	s := store.NewMemory()
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

func TestStatusWaitingHuman(t *testing.T) {
	if store.StatusWaitingHuman != "waiting_human" {
		t.Fatalf("StatusWaitingHuman=%q", store.StatusWaitingHuman)
	}

	s := store.NewMemory()
	r, err := s.CreateRun(store.CreateRunInput{AgentID: "ticket-agent", Input: "需要审批"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRun(r.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("status=%q want waiting_human", got.Status)
	}
}

func TestSetAndGetHITL(t *testing.T) {
	s := store.NewMemory()
	r, err := s.CreateRun(store.CreateRunInput{AgentID: "ticket-agent", Input: "HITL"})
	if err != nil {
		t.Fatal(err)
	}

	payload := &store.HITLPayload{
		Prompt:   "确认创建工单？",
		ToolName: "create_ticket",
		Arguments: map[string]any{
			"title":    "VPN 故障",
			"priority": "high",
		},
	}
	if err := s.SetHITL(r.ID, payload); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetHITL(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected HITL payload")
	}
	if got.Prompt != payload.Prompt || got.ToolName != payload.ToolName {
		t.Fatalf("got %+v", got)
	}
	if got.Arguments["title"] != "VPN 故障" || got.Arguments["priority"] != "high" {
		t.Fatalf("arguments=%+v", got.Arguments)
	}

	if err := s.SetHITL(r.ID, nil); err != nil {
		t.Fatal(err)
	}
	cleared, err := s.GetHITL(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared != nil {
		t.Fatalf("expected nil after clear, got %+v", cleared)
	}
}

func TestGetHITLMissingRun(t *testing.T) {
	s := store.NewMemory()
	_, err := s.GetHITL("run_missing")
	if err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestCreateRunPersistsPassthroughHeaders(t *testing.T) {
	s := store.NewMemory()
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
		t.Fatalf("%+v", got)
	}
}

func TestSetPassthroughHeadersOverwrites(t *testing.T) {
	s := store.NewMemory()
	r, err := s.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi",
		PassthroughHeaders: map[string]string{"Authorization": "Bearer OLD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPassthroughHeaders(r.ID, map[string]string{"Authorization": "Bearer NEW"}); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRun(r.ID)
	if got.PassthroughHeaders["Authorization"] != "Bearer NEW" {
		t.Fatalf("%+v", got)
	}
}

func TestSetPassthroughHeadersMissingRun(t *testing.T) {
	s := store.NewMemory()
	if err := s.SetPassthroughHeaders("run_missing", map[string]string{"Authorization": "x"}); err == nil {
		t.Fatal("expected error for missing run")
	}
}

func TestConnectorStoresAuthConfig(t *testing.T) {
	s := store.NewMemory()
	s.UpsertConnector(store.Connector{
		ID: "c", Type: "openapi", Spec: "x.yaml", BaseURL: "http://x",
		Auth: store.ConnectorAuth{Mode: "vault_ref", VaultRef: store.VaultRefAuth{
			Headers: map[string]string{"Authorization": "env:TOK"},
		}},
	})
	c, err := s.GetConnector("c")
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Mode != "vault_ref" || c.Auth.VaultRef.Headers["Authorization"] != "env:TOK" {
		t.Fatalf("%+v", c.Auth)
	}
}
