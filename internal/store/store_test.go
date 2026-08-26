package store_test

import (
	"strings"
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

func TestToolSourceMCP(t *testing.T) {
	if store.ToolSourceMCP != "mcp" {
		t.Fatalf("ToolSourceMCP=%q", store.ToolSourceMCP)
	}
}

func TestConnectorStoresMCPConfig(t *testing.T) {
	s := store.NewMemory()
	s.UpsertConnector(store.Connector{
		ID:   "m1",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "stdio",
			Command:   "echo",
			Args:      []string{"ok"},
			Env:       map[string]string{"K": "v"},
		},
	})
	c, err := s.GetConnector("m1")
	if err != nil {
		t.Fatal(err)
	}
	if c.MCP.Transport != "stdio" || c.MCP.Command != "echo" || c.MCP.Env["K"] != "v" {
		t.Fatalf("%+v", c.MCP)
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

func TestDeleteConnectorCascadesTools(t *testing.T) {
	testDeleteConnectorCascadesTools(t, store.NewMemory())
}

func testDeleteConnectorCascadesTools(t *testing.T, s store.Store) {
	s.UpsertConnector(store.Connector{ID: "c1", Type: "openapi", Spec: "s.yaml", BaseURL: "http://x"})
	s.UpsertTool(store.Tool{ConnectorID: "c1", Name: "tool_a", Source: store.ToolSourceSpec, Enabled: true})
	s.UpsertTool(store.Tool{ConnectorID: "c1", Name: "tool_b", Source: store.ToolSourceSpec, Enabled: true})
	s.UpsertConnector(store.Connector{ID: "c2", Type: "openapi", Spec: "s2.yaml", BaseURL: "http://y"})
	s.UpsertTool(store.Tool{ConnectorID: "c2", Name: "keep_tool", Source: store.ToolSourceSpec, Enabled: true})

	if err := s.DeleteConnector("c1"); err != nil {
		t.Fatalf("DeleteConnector: %v", err)
	}
	if _, err := s.GetConnector("c1"); err == nil {
		t.Fatal("expected connector not found after delete")
	}
	if byC := s.ListToolsByConnector("c1"); len(byC) != 0 {
		t.Fatalf("tools for c1=%+v want empty", byC)
	}
	if _, err := s.GetTool("tool_a"); err == nil {
		t.Fatal("tool_a should be deleted")
	}
	if _, err := s.GetTool("keep_tool"); err != nil {
		t.Fatalf("keep_tool should remain: %v", err)
	}
	if err := s.DeleteConnector("missing"); err == nil {
		t.Fatal("expected error deleting missing connector")
	} else if !strings.Contains(err.Error(), "connector not found") {
		t.Fatalf("DeleteConnector(missing) err=%q want connector not found", err)
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

func TestAgentSkillsRoundTrip(t *testing.T) {
	s := store.NewMemory()
	s.UpsertAgent(store.Agent{ID: "ticket-agent", System: "sys", Skills: []string{"ticket-triage"}})
	got, err := s.GetAgent("ticket-agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "ticket-triage" {
		t.Fatalf("%+v", got)
	}
	all := s.ListAgents()
	if len(all) != 1 {
		t.Fatalf("list=%v", all)
	}
}

func TestAgentSkillsReadCopyIsolated(t *testing.T) {
	s := store.NewMemory()
	s.UpsertAgent(store.Agent{ID: "a", System: "sys", Skills: []string{"one", "two"}})

	got, err := s.GetAgent("a")
	if err != nil {
		t.Fatal(err)
	}
	got.Skills[0] = "mutated"
	got2, err := s.GetAgent("a")
	if err != nil {
		t.Fatal(err)
	}
	if got2.Skills[0] != "one" {
		t.Fatalf("GetAgent after mutate: %+v", got2.Skills)
	}

	list := s.ListAgents()
	list[0].Skills[1] = "mutated"
	got3, _ := s.GetAgent("a")
	if got3.Skills[1] != "two" {
		t.Fatalf("ListAgents mutate leaked: %+v", got3.Skills)
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

func TestSettingsRoundTrip(t *testing.T) {
	s := store.NewMemory()
	raw := []byte(`{"url":"https://example.com","headers":{"Authorization":"env:TOK"}}`)
	if err := s.UpsertSetting(store.SettingKeyEventsWebhook, raw); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetSetting(store.SettingKeyEventsWebhook)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if string(got) != string(raw) {
		t.Fatalf("got %s", got)
	}
}

func TestCreateRunPersistsWebhookConfig(t *testing.T) {
	s := store.NewMemory()
	r, err := s.CreateRun(store.CreateRunInput{
		AgentID: "a1",
		Input:   "hi",
		WebhookConfig: &store.WebhookConfig{
			URL:     "https://hook.example/run",
			Headers: map[string]string{"X-Hook": "env:SECRET"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRun(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.WebhookConfig == nil || got.WebhookConfig.URL != "https://hook.example/run" {
		t.Fatalf("webhook=%+v", got.WebhookConfig)
	}
	if got.WebhookConfig.Headers["X-Hook"] != "env:SECRET" {
		t.Fatalf("headers=%+v", got.WebhookConfig.Headers)
	}
}
