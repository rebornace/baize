package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/demo"
)

func TestConnectorContractListReplaceAndBadSpec(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "ticket-agent"
	cfg.Agent.System = "你是企业工单助手，只能通过工具访问工单系统。"
	cfg.Connector.ID = "ticket-api"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = filepath.Join("..", "..", "examples", "mock-ticket", "openapi.yaml")
	cfg.Connector.RequireApproval = []string{"create_ticket"}
	cfg.Run.MaxSteps = 8

	runtimeURL, ticketURL, shutdown := demo.StartForTest(t, cfg)
	defer shutdown()

	tools := getTools(t, runtimeURL)
	if len(tools) != 4 {
		t.Fatalf("GET /v0/tools: len=%d want 4 (demo mock-ticket openapi)", len(tools))
	}
	for _, tool := range tools {
		if tool.Method == "" || tool.Path == "" {
			t.Fatalf("tool %q missing method/path: %+v", tool.Name, tool)
		}
		if tool.OperationID == "" {
			t.Fatalf("tool %q missing operation_id: %+v", tool.Name, tool)
		}
		if tool.ConnectorID != "ticket-api" {
			t.Fatalf("tool %q connector_id=%q want ticket-api", tool.Name, tool.ConnectorID)
		}
	}

	conn := getConnector(t, runtimeURL, "ticket-api")
	if conn.ID != "ticket-api" || conn.Type != "openapi" {
		t.Fatalf("GET connector: %+v", conn)
	}
	if len(conn.Tools) != 4 {
		t.Fatalf("GET connector tools=%d want 4", len(conn.Tools))
	}

	reducedSpec := filepath.Join(t.TempDir(), "reduced.yaml")
	if err := os.WriteFile(reducedSpec, []byte(`openapi: 3.0.3
info:
  title: Mock Ticket API (reduced)
  version: 0.1.0
paths:
  /tickets:
    get:
      operationId: list_tickets
      responses:
        "200":
          description: ok
    post:
      operationId: create_ticket
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [title]
              properties:
                title: { type: string }
      responses:
        "201":
          description: created
`), 0o644); err != nil {
		t.Fatal(err)
	}

	putConnector(t, runtimeURL, "ticket-api", map[string]any{
		"type":             "openapi",
		"spec":             reducedSpec,
		"base_url":         ticketURL,
		"require_approval": []string{"create_ticket"},
	}, http.StatusOK)

	afterReplace := getTools(t, runtimeURL)
	if len(afterReplace) != 2 {
		t.Fatalf("after replace tools=%d want 2", len(afterReplace))
	}
	names := map[string]bool{}
	for _, tool := range afterReplace {
		names[tool.Name] = true
		if tool.Method == "" || tool.Path == "" || tool.OperationID == "" {
			t.Fatalf("tool %q missing method/path/operation_id after replace: %+v", tool.Name, tool)
		}
		if tool.ConnectorID != "ticket-api" {
			t.Fatalf("tool %q connector_id=%q want ticket-api", tool.Name, tool.ConnectorID)
		}
	}
	if !names["list_tickets"] || !names["create_ticket"] {
		t.Fatalf("after replace names=%v want list_tickets+create_ticket", names)
	}

	connAfterReplace := getConnector(t, runtimeURL, "ticket-api")
	if connAfterReplace.Spec != reducedSpec {
		t.Fatalf("connector.spec after replace=%q want %q", connAfterReplace.Spec, reducedSpec)
	}
	if len(connAfterReplace.Tools) != 2 {
		t.Fatalf("GET connector after replace tools=%d want 2", len(connAfterReplace.Tools))
	}

	badSpec := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(badSpec, []byte("this: is: not: valid: openapi: [[[\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	putConnector(t, runtimeURL, "ticket-api", map[string]any{
		"type":     "openapi",
		"spec":     badSpec,
		"base_url": ticketURL,
	}, http.StatusBadRequest)

	afterBad := getTools(t, runtimeURL)
	if len(afterBad) != 2 {
		t.Fatalf("after bad spec tools=%d want unchanged 2", len(afterBad))
	}
	afterNames := map[string]bool{}
	for _, tool := range afterBad {
		afterNames[tool.Name] = true
	}
	for name := range names {
		if !afterNames[name] {
			t.Fatalf("after bad spec missing tool %q; got %v", name, afterNames)
		}
	}

	connAfterBad := getConnector(t, runtimeURL, "ticket-api")
	if connAfterBad.Spec != reducedSpec {
		t.Fatalf("connector store polluted: spec=%q want %q (reduced)", connAfterBad.Spec, reducedSpec)
	}
	if len(connAfterBad.Tools) != 2 {
		t.Fatalf("GET connector after bad spec tools=%d want 2", len(connAfterBad.Tools))
	}
}

type contractTool struct {
	Name        string `json:"name"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	OperationID string `json:"operation_id"`
	ConnectorID string `json:"connector_id"`
}

type contractConnector struct {
	ID    string         `json:"id"`
	Type  string         `json:"type"`
	Spec  string         `json:"spec"`
	Tools []contractTool `json:"tools"`
}

func getTools(t *testing.T, runtimeURL string) []contractTool {
	t.Helper()
	resp, err := http.Get(runtimeURL + "/v0/tools")
	if err != nil {
		t.Fatalf("GET /v0/tools: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v0/tools status=%d body=%s", resp.StatusCode, raw)
	}
	var body struct {
		Tools []contractTool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode tools: %v body=%s", err, raw)
	}
	return body.Tools
}

func getConnector(t *testing.T, runtimeURL, id string) contractConnector {
	t.Helper()
	resp, err := http.Get(runtimeURL + "/v0/connectors/" + id)
	if err != nil {
		t.Fatalf("GET /v0/connectors/%s: %v", id, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v0/connectors/%s status=%d body=%s", id, resp.StatusCode, raw)
	}
	var conn contractConnector
	if err := json.Unmarshal(raw, &conn); err != nil {
		t.Fatalf("decode connector: %v body=%s", err, raw)
	}
	return conn
}

func putConnector(t *testing.T, runtimeURL, id string, payload map[string]any, wantStatus int) {
	t.Helper()
	rawBody, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, runtimeURL+"/v0/connectors/"+id, strings.NewReader(string(rawBody)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /v0/connectors/%s: %v", id, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		t.Fatalf("PUT /v0/connectors/%s status=%d want %d body=%s", id, resp.StatusCode, wantStatus, raw)
	}
}
