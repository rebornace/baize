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

func TestToolCatalogCRUD(t *testing.T) {
	s := store.NewMemory()
	s.UpsertConnector(store.Connector{ID: "c1", Type: "openapi", Spec: "s.yaml", BaseURL: "http://x"})
	row := store.Tool{
		ConnectorID:       "c1",
		Name:              "create_ticket",
		Source:            store.ToolSourceSpec,
		Enabled:           true,
		Title:             "建工单",
		DescriptionCustom: true,
		Method:            "POST",
		Path:              "/tickets",
		Description:       "create",
		InputSchema:       map[string]any{"type": "object"},
	}
	s.UpsertTool(row)
	got, err := s.GetTool("create_ticket")
	if err != nil || !got.Enabled || got.Source != store.ToolSourceSpec {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if got.Title != "建工单" || !got.DescriptionCustom {
		t.Fatalf("title/custom: got=%+v", got)
	}
	list := s.ListTools()
	if len(list) != 1 || list[0].Name != "create_ticket" {
		t.Fatalf("list=%+v", list)
	}
	byC := s.ListToolsByConnector("c1")
	if len(byC) != 1 {
		t.Fatalf("byC=%+v", byC)
	}
	row.Enabled = false
	s.UpsertTool(row)
	got, _ = s.GetTool("create_ticket")
	if got.Enabled {
		t.Fatal("expected disabled")
	}
	if err := s.DeleteTool("create_ticket"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTool("create_ticket"); err == nil {
		t.Fatal("expected not found")
	}
	cs := s.ListConnectors()
	if len(cs) != 1 || cs[0].ID != "c1" {
		t.Fatalf("connectors=%+v", cs)
	}
}

func TestReplaceConnectorToolsKeepsOthers(t *testing.T) {
	s := store.NewMemory()
	s.UpsertTool(store.Tool{ConnectorID: "a", Name: "keep", Source: store.ToolSourceSpec, Enabled: true})
	s.UpsertTool(store.Tool{ConnectorID: "b", Name: "gone", Source: store.ToolSourceSpec, Enabled: true})
	s.ReplaceConnectorTools("b", []store.Tool{
		{ConnectorID: "b", Name: "new", Source: store.ToolSourceExtra, Enabled: true, Method: "GET", Path: "/n"},
	})
	if _, err := s.GetTool("gone"); err == nil {
		t.Fatal("gone should be replaced away")
	}
	if _, err := s.GetTool("keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTool("new"); err != nil {
		t.Fatal(err)
	}
}
