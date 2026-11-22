package mobileagent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

const readonlySpecJSON = `{
  "openapi": "3.0.3",
  "servers": [{"url": "https://api.example.com/base"}],
  "paths": {
    "/tickets": {
      "parameters": [
        {"name": "tenant", "in": "query", "required": true, "description": "Tenant id", "schema": {"type": "string"}},
        {"name": "X-Trace", "in": "header", "schema": {"type": "string"}},
        {"$ref": "#/components/parameters/ignored"}
      ],
      "get": {
        "operationId": "list_tickets",
        "summary": "List tickets",
        "description": "List tickets for a tenant",
        "parameters": [
          {"name": "status", "in": "query", "schema": {"type": "string"}, "description": "open or closed"},
          {"name": "limit", "in": "query", "schema": {"type": "integer"}}
        ],
        "responses": {"200": {"description": "ok"}}
      },
      "post": {
        "operationId": "create_ticket",
        "responses": {"201": {"description": "created"}}
      }
    },
    "/tickets/{id}": {
      "get": {
        "operationId": "get_ticket",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}, "description": "Ticket id"},
          {"name": "expand", "in": "query", "schema": {"type": "boolean"}}
        ],
        "responses": {"200": {"description": "ok"}}
      }
    }
  }
}`

const writeSpecJSON = `{
  "openapi": "3.0.3",
  "paths": {
    "/tickets": {
      "get": {"operationId": "list_tickets"},
      "post": {
        "operationId": "create_ticket",
        "summary": "Create ticket",
        "parameters": [
          {"name": "notify", "in": "query", "schema": {"type": "boolean"}, "description": "Notify the assignee"}
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"$ref": "#/components/schemas/CreateTicket"}
            }
          }
        }
      }
    },
    "/tickets/{id}": {
      "put": {
        "operationId": "replace_ticket",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["title"],
                "properties": {
                  "title": {"type": "string", "description": "New title"},
                  "priority": {"type": "string"}
                }
              }
            }
          }
        }
      },
      "patch": {
        "operationId": "patch_ticket",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {"status": {"type": "string"}}
              }
            }
          }
        }
      },
      "delete": {"operationId": "delete_ticket"}
    },
    "/bulk": {
      "post": {
        "operationId": "bulk_create",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {"type": "array", "items": {"type": "string"}}
            }
          }
        }
      }
    }
  },
  "components": {
    "schemas": {
      "CreateTicket": {
        "type": "object",
        "required": ["title"],
        "properties": {
          "title": {"type": "string"},
          "priority": {"type": "string"},
          "meta": {"type": "object", "description": "Extra fields"}
        }
      }
    }
  }
}`

const formSpecJSON = `{
  "openapi": "3.0.3",
  "paths": {
    "/tickets": {
      "post": {
        "operationId": "create_ticket_form",
        "requestBody": {
          "required": true,
          "content": {
            "application/x-www-form-urlencoded": {
              "schema": {
                "type": "object",
                "required": ["title"],
                "properties": {
                  "title": {"type": "string"},
                  "priority": {"type": "string"}
                }
              }
            }
          }
        }
      }
    }
  }
}`

const multipartSpecJSON = `{
  "openapi": "3.0.3",
  "paths": {
    "/upload": {
      "post": {
        "operationId": "upload_file",
        "requestBody": {
          "required": true,
          "content": {
            "multipart/form-data": {
              "schema": {
                "type": "object",
                "required": ["file"],
                "properties": {
                  "file": {"type": "string", "format": "binary", "description": "PNG bytes"},
                  "note": {"type": "string"}
                }
              }
            }
          }
        }
      }
    }
  }
}`

func parseConnectorAPIs(t *testing.T, cfg readOnlyConnectorConfig) *readonlyAPI {
	t.Helper()
	cfg.BaseURL = "https://api.example.com"
	api, err := parseReadOnlyAPIs([]readOnlyConnectorConfig{cfg})
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func parseWriteAPI(spec string, allowWrites bool) (*readonlyAPI, error) {
	return parseReadOnlyAPIs([]readOnlyConnectorConfig{{
		BaseURL:     "https://api.example.com",
		Spec:        spec,
		AllowWrites: allowWrites,
	}})
}

func parseOneReadOnlyAPI(base, authH, authV, spec string) (*readonlyAPI, error) {
	return parseReadOnlyAPIs([]readOnlyConnectorConfig{{
		BaseURL:    base,
		AuthHeader: authH,
		AuthValue:  authV,
		Spec:       spec,
	}})
}

func TestParseReadOnlyAPI(t *testing.T) {
	cfg, err := parseOneReadOnlyAPI("https://api.example.com/base", "Authorization", "Bearer tok", readonlySpecJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.tools) == 0 || cfg.tools[0].BaseURL != "https://api.example.com/base" {
		t.Fatalf("base=%+v", cfg.tools)
	}
	if len(cfg.tools) != 2 {
		t.Fatalf("tools=%+v", cfg.tools)
	}
	list := cfg.tools[0]
	if list.Name != "list_tickets" || !strings.Contains(list.Description, "List tickets") {
		t.Fatalf("list=%+v", list)
	}
	if len(list.Params) != 3 {
		t.Fatalf("params=%+v", list.Params)
	}
	if list.Params[0].Name != "tenant" || !list.Params[0].Required {
		t.Fatalf("tenant=%+v", list.Params[0])
	}
	if list.AuthHeader != "Authorization" || list.AuthValue != "Bearer tok" {
		t.Fatalf("auth=%q %q", list.AuthHeader, list.AuthValue)
	}
	get := cfg.tools[1]
	if get.Name != "get_ticket" || len(get.Params) != 2 {
		t.Fatalf("get=%+v", get)
	}
	if get.Params[0].In != "path" || !get.Params[0].Required || get.Params[0].Type != "string" {
		t.Fatalf("path param=%+v", get.Params[0])
	}
	if get.Params[1].Type != "boolean" {
		t.Fatalf("query param=%+v", get.Params[1])
	}
}

func TestParseReadOnlyAPIYAMLAndServerFallback(t *testing.T) {
	spec := `
openapi: 3.0.3
servers:
  - url: https://yaml.example.com
paths:
  /health:
    get:
      operationId: health
      responses:
        "200":
          description: ok
`
	cfg, err := parseOneReadOnlyAPI("", "", "", spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.tools) != 1 || cfg.tools[0].Name != "health" || cfg.tools[0].BaseURL != "https://yaml.example.com" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

const readonlyHealthSpecJSON = `{"openapi":"3.0.3","paths":{"/health":{"get":{"operationId":"health"}}}}`

func TestParseReadOnlyAPIsCombinesConnectors(t *testing.T) {
	cfg, err := parseReadOnlyAPIs([]readOnlyConnectorConfig{
		{ID: "tickets", BaseURL: "https://tickets.example.com", AuthHeader: "X-Key", AuthValue: "k1", Spec: readonlySpecJSON},
		{ID: "health", BaseURL: "https://health.example.com", Spec: readonlyHealthSpecJSON},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.connectors != 2 || len(cfg.tools) != 3 {
		t.Fatalf("connectors=%d tools=%d", cfg.connectors, len(cfg.tools))
	}
	want := map[string]string{
		"list_tickets": "https://tickets.example.com",
		"get_ticket":   "https://tickets.example.com",
		"health":       "https://health.example.com",
	}
	for _, tool := range cfg.tools {
		if want[tool.Name] != tool.BaseURL {
			t.Fatalf("tool %s base=%q want %q", tool.Name, tool.BaseURL, want[tool.Name])
		}
		if tool.Name == "list_tickets" && (tool.AuthHeader != "X-Key" || tool.AuthValue != "k1") {
			t.Fatalf("auth=%q %q", tool.AuthHeader, tool.AuthValue)
		}
	}
}

func TestParseReadOnlyAPIsRejectsCrossConnectorCollisions(t *testing.T) {
	dup := `{"openapi":"3.0.3","paths":{"/x":{"get":{"operationId":"dup"}}}}`
	_, err := parseReadOnlyAPIs([]readOnlyConnectorConfig{
		{ID: "a", BaseURL: "https://a.example.com", Spec: dup},
		{ID: "b", BaseURL: "https://b.example.com", Spec: dup},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate") ||
		!strings.Contains(err.Error(), "a") || !strings.Contains(err.Error(), "b") {
		t.Fatalf("err=%v", err)
	}

	reserved := `{"openapi":"3.0.3","paths":{"/x":{"get":{"operationId":"http_get"}}}}`
	_, err = parseReadOnlyAPIs([]readOnlyConnectorConfig{{BaseURL: "https://a.example.com", Spec: reserved}})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "reserved") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseReadOnlyAPIsLimits(t *testing.T) {
	tooMany := make([]readOnlyConnectorConfig, maxReadOnlyConnectors+1)
	for i := range tooMany {
		tooMany[i] = readOnlyConnectorConfig{BaseURL: "https://a.example.com", Spec: readonlyHealthSpecJSON}
	}
	if _, err := parseReadOnlyAPIs(tooMany); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("connectors err=%v", err)
	}
	if _, err := parseReadOnlyAPIs(nil); err == nil || !strings.Contains(err.Error(), "at least") {
		t.Fatalf("empty err=%v", err)
	}
}

func TestRemoteToolExecutionUsesOwnConnector(t *testing.T) {
	var aPath, aKey, bPath, bAuth string
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		aPath, aKey = r.URL.Path, r.Header.Get("X-Key")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srvA.Close)
	srvB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bPath, bAuth = r.URL.Path, r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srvB.Close)

	specA := `{"openapi":"3.0.3","paths":{"/a":{"get":{"operationId":"tool_a"}}}}`
	specB := `{"openapi":"3.0.3","paths":{"/b":{"get":{"operationId":"tool_b"}}}}`
	configs := fmt.Sprintf(
		`[{"id":"a","base_url":%q,"auth_header":"X-Key","auth_value":"ka","spec":%q},`+
			`{"id":"b","base_url":%q,"auth_header":"Authorization","auth_value":"Bearer kb","spec":%q}]`,
		srvA.URL, specA, srvB.URL, specB,
	)
	a := New()
	if err := a.ConfigureReadOnlyAPIs(configs); err != nil {
		t.Fatal(err)
	}
	if _, err := a.execRemoteTool(a.remoteToolLocked("tool_a"), `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := a.execRemoteTool(a.remoteToolLocked("tool_b"), `{}`); err != nil {
		t.Fatal(err)
	}
	if aPath != "/a" || aKey != "ka" {
		t.Fatalf("a path=%q key=%q", aPath, aKey)
	}
	if bPath != "/b" || bAuth != "Bearer kb" {
		t.Fatalf("b path=%q auth=%q", bPath, bAuth)
	}
}

func TestConfigureReadOnlyAPIsKeepsOldOnError(t *testing.T) {
	a := New()
	if err := a.ConfigureReadOnlyAPI("https://api.example.com", "", "", readonlySpecJSON); err != nil {
		t.Fatal(err)
	}
	dup := `{"openapi":"3.0.3","paths":{"/x":{"get":{"operationId":"dup"}}}}`
	err := a.ConfigureReadOnlyAPIs(fmt.Sprintf(
		`[{"id":"a","base_url":"https://a.example.com","spec":%q},`+
			`{"id":"b","base_url":"https://b.example.com","spec":%q}]`, dup, dup,
	))
	if err == nil {
		t.Fatal("want duplicate error")
	}
	if a.RemoteToolCount() != 2 {
		t.Fatalf("old tools must remain, got %d", a.RemoteToolCount())
	}
	if err := a.ConfigureReadOnlyAPIs("{"); err == nil {
		t.Fatal("want invalid json error")
	}
	if a.RemoteToolCount() != 2 {
		t.Fatalf("old tools must remain, got %d", a.RemoteToolCount())
	}
}

func TestParseReadOnlyAPISkipsUnsupported(t *testing.T) {
	spec := `{"openapi":"3.0.3","paths":{"/x":{"post":{"operationId":"write"},"get":{"operationId":"read","parameters":[{"$ref":"#/p"}]}}}}`
	cfg, err := parseOneReadOnlyAPI("https://api.example.com", "", "", spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.tools) != 1 || cfg.tools[0].Name != "read" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestParseWriteToolsOptIn(t *testing.T) {
	readOnly, err := parseWriteAPI(writeSpecJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(readOnly.tools) != 1 || readOnly.tools[0].Name != "list_tickets" {
		t.Fatalf("default tools=%+v", readOnly.tools)
	}
	if writes := writeToolCount(readOnly); writes != 0 {
		t.Fatalf("writes=%d", writes)
	}

	cfg, err := parseWriteAPI(writeSpecJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	want := "bulk_create, list_tickets, create_ticket, replace_ticket, patch_ticket"
	if names := toolNames(cfg); names != want {
		t.Fatalf("names=%q want %q", names, want)
	}
	for _, tool := range cfg.tools {
		if tool.Name == "delete_ticket" {
			t.Fatal("DELETE must stay unsupported")
		}
	}
	if writes := writeToolCount(cfg); writes != 4 {
		t.Fatalf("writes=%d", writes)
	}
}

func TestParseWriteToolRequestBodies(t *testing.T) {
	cfg, err := parseWriteAPI(writeSpecJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]remoteTool{}
	for _, tool := range cfg.tools {
		byName[tool.Name] = tool
	}

	create := byName["create_ticket"]
	if create.Method != http.MethodPost || create.WholeBody {
		t.Fatalf("create=%+v", create)
	}
	params := map[string]remoteToolParam{}
	for _, p := range create.Params {
		params[p.Name] = p
	}
	if params["notify"].In != "query" || params["notify"].Type != "boolean" {
		t.Fatalf("notify=%+v", params["notify"])
	}
	if p := params["title"]; p.In != "body" || !p.Required || p.Type != "string" {
		t.Fatalf("title=%+v", p)
	}
	if p := params["meta"]; p.In != "body" || p.Required || p.Type != "object" {
		t.Fatalf("meta=%+v", p)
	}
	if !strings.Contains(create.Description, "Create ticket") {
		t.Fatalf("description=%q", create.Description)
	}

	replace := byName["replace_ticket"]
	if replace.Method != http.MethodPut || replace.WholeBody {
		t.Fatalf("replace=%+v", replace)
	}
	for _, p := range replace.Params {
		if p.Name == "title" && !p.Required {
			t.Fatalf("title must be required: %+v", p)
		}
	}

	bulk := byName["bulk_create"]
	if bulk.Method != http.MethodPost || !bulk.WholeBody {
		t.Fatalf("bulk=%+v", bulk)
	}
	if len(bulk.Params) != 1 || bulk.Params[0].Name != "body" || bulk.Params[0].Type != "array" || !bulk.Params[0].Required {
		t.Fatalf("bulk params=%+v", bulk.Params)
	}
	body, _, err := bulk.bodyPayload(map[string]any{"body": []any{"a", "b"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `["a","b"]` {
		t.Fatalf("bulk body=%q", body)
	}
	if _, _, err := bulk.bodyPayload(map[string]any{}, true); err == nil ||
		!strings.Contains(err.Error(), "missing request body") {
		t.Fatalf("missing body err=%v", err)
	}
}

func TestWriteToolRejectsBodyParamCollision(t *testing.T) {
	spec := `{
  "openapi": "3.0.3",
  "paths": {
    "/tickets/{id}": {
      "put": {
        "operationId": "put_ticket",
        "parameters": [
          {"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {"id": {"type": "string"}}
              }
            }
          }
        }
      }
    }
  }
}`
	_, err := parseWriteAPI(spec, true)
	if err == nil || !strings.Contains(err.Error(), "conflicts") {
		t.Fatalf("err=%v", err)
	}
}

func TestWriteToolKeepsGETBodyFree(t *testing.T) {
	spec := `{
  "openapi": "3.0.3",
  "paths": {
    "/search": {
      "get": {
        "operationId": "search",
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {"type": "object", "properties": {"q": {"type": "string"}}}
            }
          }
        }
      }
    }
  }
}`
	cfg, err := parseWriteAPI(spec, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.tools) != 1 || len(cfg.tools[0].Params) != 0 || cfg.tools[0].WholeBody {
		t.Fatalf("GET must ignore requestBody: %+v", cfg.tools)
	}
}

func TestWriteToolRequiredObjectBodySendsEmptyJSON(t *testing.T) {
	spec := `{
  "openapi": "3.0.3",
  "paths": {
    "/actions/refresh": {
      "post": {
        "operationId": "refresh",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {"note": {"type": "string"}}
              }
            }
          }
        }
      }
    }
  }
}`
	cfg, err := parseWriteAPI(spec, true)
	if err != nil {
		t.Fatal(err)
	}
	tool := cfg.tools[0]
	if !tool.BodyRequired {
		t.Fatal("required request body must be tracked")
	}
	body, _, err := tool.bodyPayload(map[string]any{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "{}" {
		t.Fatalf("body=%q", body)
	}
}

func TestWriteToolSpecExposesBodyFields(t *testing.T) {
	cfg, err := parseWriteAPI(writeSpecJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	var create remoteTool
	for _, tool := range cfg.tools {
		if tool.Name == "create_ticket" {
			create = tool
		}
	}
	spec := create.spec()
	props := spec.Function.Parameters["properties"].(map[string]any)
	if props["notify"] == nil || props["title"] == nil || props["meta"] == nil {
		t.Fatalf("props=%+v", props)
	}
	required := spec.Function.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "title" {
		t.Fatalf("required=%+v", required)
	}
}

func TestWriteToolSummaryShowsMethodAndBody(t *testing.T) {
	cfg, err := parseWriteAPI(writeSpecJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	var create remoteTool
	for _, tool := range cfg.tools {
		if tool.Name == "create_ticket" {
			create = tool
		}
	}
	summary := create.summary(`{"notify":"true","title":"VPN 故障","priority":"high"}`)
	for _, want := range []string{"POST /tickets", "notify=true", `"title":"VPN 故障"`} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary=%q missing %q", summary, want)
		}
	}
}

func TestWriteToolRequiresApprovalAndSendsJSON(t *testing.T) {
	var hits atomic.Int32
	var gotMethod, gotPath, gotQuery, gotAuth, gotContentType, gotBody string
	business := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("X-API-Key")
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"T-2001"}`))
	}))
	t.Cleanup(business.Close)

	var llmCalls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if llmCalls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id": "c-create", "type": "function",
							"function": map[string]any{
								"name":      "create_ticket",
								"arguments": `{"notify":"true","title":"VPN 故障","priority":"high"}`,
							},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "created"}}},
		})
	}))
	t.Cleanup(llm.Close)

	a := New()
	_ = a.Configure(llm.URL, "k", "m")
	configs := fmt.Sprintf(
		`[{"id":"tickets","base_url":%q,"auth_header":"X-API-Key","auth_value":"write-key","spec":%q,"allow_writes":true}]`,
		business.URL, writeSpecJSON,
	)
	if err := a.ConfigureReadOnlyAPIs(configs); err != nil {
		t.Fatal(err)
	}
	raw := a.Send("create")
	var pause map[string]any
	_ = json.Unmarshal([]byte(raw), &pause)
	if pause["status"] != "needs_approval" || pause["method"] != "POST" || pause["write"] != true {
		t.Fatalf("pause=%v", pause)
	}
	if summary, _ := pause["summary"].(string); !strings.Contains(summary, "POST /tickets") {
		t.Fatalf("summary=%q", summary)
	}
	if hits.Load() != 0 || a.RemoteHits() != 0 {
		t.Fatal("write must not run before approval")
	}

	raw = a.Approve("c-create")
	var done map[string]any
	_ = json.Unmarshal([]byte(raw), &done)
	if done["status"] != "done" || done["text"] != "created" {
		t.Fatalf("done=%v raw=%s", done, raw)
	}
	if hits.Load() != 1 || a.RemoteHits() != 1 {
		t.Fatalf("hits=%d remote=%d", hits.Load(), a.RemoteHits())
	}
	if gotMethod != http.MethodPost || gotPath != "/tickets" || gotQuery != "notify=true" {
		t.Fatalf("method=%q path=%q query=%q", gotMethod, gotPath, gotQuery)
	}
	if gotAuth != "write-key" || !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("auth=%q content-type=%q", gotAuth, gotContentType)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body=%q err=%v", gotBody, err)
	}
	if body["title"] != "VPN 故障" || body["priority"] != "high" || body["notify"] != nil {
		t.Fatalf("body=%v", body)
	}
}

func TestWriteToolDenyDoesNotSend(t *testing.T) {
	var hits atomic.Int32
	business := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(business.Close)

	var llmCalls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if llmCalls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id": "c-write-deny", "type": "function",
							"function": map[string]any{
								"name":      "create_ticket",
								"arguments": `{"title":"do not send"}`,
							},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "denied"}}},
		})
	}))
	t.Cleanup(llm.Close)

	a := New()
	_ = a.Configure(llm.URL, "k", "m")
	configs := fmt.Sprintf(`[{"base_url":%q,"spec":%q,"allow_writes":true}]`, business.URL, writeSpecJSON)
	if err := a.ConfigureReadOnlyAPIs(configs); err != nil {
		t.Fatal(err)
	}
	raw := a.Send("create")
	var pause map[string]any
	_ = json.Unmarshal([]byte(raw), &pause)
	if pause["status"] != "needs_approval" {
		t.Fatalf("pause=%v", pause)
	}
	raw = a.Deny("c-write-deny")
	var done map[string]any
	_ = json.Unmarshal([]byte(raw), &done)
	if done["status"] != "done" || done["text"] != "denied" {
		t.Fatalf("done=%v", done)
	}
	if hits.Load() != 0 || a.RemoteHits() != 0 {
		t.Fatal("denied write must not send")
	}
}

func TestWriteToolDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
	}))
	t.Cleanup(target.Close)
	business := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusTemporaryRedirect)
	}))
	t.Cleanup(business.Close)

	a := New()
	configs := fmt.Sprintf(
		`[{"base_url":%q,"spec":%q,"allow_writes":true}]`,
		business.URL, writeSpecJSON,
	)
	if err := a.ConfigureReadOnlyAPIs(configs); err != nil {
		t.Fatal(err)
	}
	tool := a.remoteToolLocked("create_ticket")
	if tool == nil {
		t.Fatal("missing create_ticket")
	}
	out, err := a.execRemoteTool(tool, `{"title":"no redirect"}`)
	if err != nil {
		t.Fatal(err)
	}
	var resp map[string]any
	_ = json.Unmarshal([]byte(out), &resp)
	if resp["status"] != float64(http.StatusTemporaryRedirect) {
		t.Fatalf("resp=%v", resp)
	}
	if redirected.Load() != 0 {
		t.Fatal("write redirect must not be followed")
	}
}

func TestWriteToolBodySizeLimit(t *testing.T) {
	cfg, err := parseWriteAPI(writeSpecJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	var create remoteTool
	for _, tool := range cfg.tools {
		if tool.Name == "create_ticket" {
			create = tool
		}
	}
	big := strings.Repeat("x", maxRemoteBodyBytes+1)
	if _, _, err := create.bodyPayload(map[string]any{"title": big}, true); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err=%v", err)
	}
}

func toolNames(cfg *readonlyAPI) string {
	names := make([]string, 0, len(cfg.tools))
	for _, tool := range cfg.tools {
		names = append(names, tool.Name)
	}
	return strings.Join(names, ", ")
}

func TestConnectorMethodsAllowlist(t *testing.T) {
	cfg := parseConnectorAPIs(t, readOnlyConnectorConfig{Spec: writeSpecJSON, Methods: []string{"GET", "PATCH"}})
	if names := toolNames(cfg); names != "list_tickets, patch_ticket" {
		t.Fatalf("names=%q", names)
	}
	if writes := writeToolCount(cfg); writes != 1 {
		t.Fatalf("writes=%d", writes)
	}
	if _, err := parseReadOnlyAPIs([]readOnlyConnectorConfig{{
		BaseURL: "https://api.example.com", Spec: writeSpecJSON, Methods: []string{"DELETE"},
	}}); err == nil || !strings.Contains(err.Error(), "unsupported method") {
		t.Fatalf("DELETE err=%v", err)
	}
	cfg = parseConnectorAPIs(t, readOnlyConnectorConfig{Spec: writeSpecJSON, Methods: []string{"post"}})
	if names := toolNames(cfg); names != "bulk_create, create_ticket" {
		t.Fatalf("POST-only names=%q", names)
	}
}

func TestFormToolExecution(t *testing.T) {
	var gotCT string
	var gotForm url.Values
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		gotCT = r.Header.Get("Content-Type")
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)

	a := New()
	configs := fmt.Sprintf(`[{"base_url":%q,"spec":%q,"methods":["POST"]}]`, srv.URL, formSpecJSON)
	if err := a.ConfigureReadOnlyAPIs(configs); err != nil {
		t.Fatal(err)
	}
	tool := a.remoteToolLocked("create_ticket_form")
	if tool == nil || tool.BodyKind != bodyKindForm {
		t.Fatalf("tool=%+v", tool)
	}
	if _, err := a.execRemoteTool(tool, `{"title":"VPN 故障","priority":"high"}`); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || !strings.HasPrefix(gotCT, "application/x-www-form-urlencoded") {
		t.Fatalf("method=%q content-type=%q", method, gotCT)
	}
	if gotForm.Get("title") != "VPN 故障" || gotForm.Get("priority") != "high" {
		t.Fatalf("form=%v", gotForm)
	}
}

func TestMultipartToolExecution(t *testing.T) {
	var gotCT, gotNote, gotName string
	var gotBytes []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotNote = r.FormValue("note")
		file, hdr, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer file.Close()
		gotName = hdr.Filename
		gotBytes, _ = io.ReadAll(file)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a := New()
	configs := fmt.Sprintf(`[{"base_url":%q,"spec":%q,"methods":["POST"]}]`, srv.URL, multipartSpecJSON)
	if err := a.ConfigureReadOnlyAPIs(configs); err != nil {
		t.Fatal(err)
	}
	tool := a.remoteToolLocked("upload_file")
	if tool == nil || tool.BodyKind != bodyKindMultipart {
		t.Fatalf("tool=%+v", tool)
	}
	args := fmt.Sprintf(`{"note":"hi","file":%q,"file_filename":"x.png"}`,
		base64.StdEncoding.EncodeToString([]byte("png-bytes")))
	if _, err := a.execRemoteTool(tool, args); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data; boundary=") {
		t.Fatalf("content-type=%q", gotCT)
	}
	if gotNote != "hi" || gotName != "x.png" || string(gotBytes) != "png-bytes" {
		t.Fatalf("note=%q name=%q bytes=%q", gotNote, gotName, gotBytes)
	}

	if _, err := a.execRemoteTool(tool, `{"note":"missing"}`); err == nil ||
		!strings.Contains(err.Error(), "missing file field file") {
		t.Fatalf("missing file err=%v", err)
	}
}

func TestMultipartFileSizeLimit(t *testing.T) {
	cfg := parseConnectorAPIs(t, readOnlyConnectorConfig{Spec: multipartSpecJSON, Methods: []string{"POST"}})
	tool := cfg.tools[0]
	big := base64.StdEncoding.EncodeToString(make([]byte, maxRemoteFileBytes+1))
	_, _, err := tool.bodyPayload(map[string]any{"file": big}, true)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseReadOnlyAPIRejectsBadConfig(t *testing.T) {
	cases := []struct {
		name  string
		base  string
		authH string
		authV string
		spec  string
		msg   string
	}{
		{"bad-json", "https://x", "", "", "{", "invalid"},
		{"not-openapi3", "https://x", "", "", `{"swagger":"2.0","paths":{}}`, "OpenAPI 3"},
		{"no-get", "https://x", "", "", `{"openapi":"3.0.3","paths":{"/x":{"post":{"operationId":"x"}}}}`, "no GET"},
		{"bad-base", "ftp://x", "", "", readonlySpecJSON, "http"},
		{"auth-half", "https://x", "Authorization", "", `{"openapi":"3.0.3","paths":{"/x":{"get":{"operationId":"x"}}}}`, "both"},
		{"reserved", "https://x", "", "", `{"openapi":"3.0.3","paths":{"/x":{"get":{"operationId":"http_get"}}}}`, "reserved"},
		{"duplicate", "https://x", "", "", `{"openapi":"3.0.3","paths":{"/a":{"get":{"operationId":"dup"}},"/b":{"get":{"operationId":"dup"}}}}`, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseOneReadOnlyAPI(tc.base, tc.authH, tc.authV, tc.spec)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.msg)) {
				t.Fatalf("err=%v want %q", err, tc.msg)
			}
		})
	}
}

func TestRemoteToolSpec(t *testing.T) {
	cfg, err := parseOneReadOnlyAPI("https://api.example.com", "", "", readonlySpecJSON)
	if err != nil {
		t.Fatal(err)
	}
	spec := cfg.tools[0].spec()
	if spec.Type != "function" || spec.Function.Name != "list_tickets" {
		t.Fatalf("spec=%+v", spec)
	}
	props := spec.Function.Parameters["properties"].(map[string]any)
	if props["tenant"] == nil || props["status"] == nil {
		t.Fatalf("props=%+v", props)
	}
	required := spec.Function.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "tenant" {
		t.Fatalf("required=%+v", required)
	}
}

func TestConfigureReadOnlyAPIInvalidKeepsOld(t *testing.T) {
	a := New()
	if err := a.ConfigureReadOnlyAPI("https://api.example.com", "", "", readonlySpecJSON); err != nil {
		t.Fatal(err)
	}
	if a.RemoteToolCount() != 2 {
		t.Fatalf("tools=%d", a.RemoteToolCount())
	}
	if err := a.ConfigureReadOnlyAPI("ftp://bad", "", "", readonlySpecJSON); err == nil {
		t.Fatal("want error")
	}
	if a.RemoteToolCount() != 2 {
		t.Fatalf("old tools must remain, got %d", a.RemoteToolCount())
	}
	a.ClearReadOnlyAPI()
	if a.RemoteToolCount() != 0 {
		t.Fatalf("clear=%d", a.RemoteToolCount())
	}
}

func TestRemoteToolNames(t *testing.T) {
	a := New()
	if got := a.RemoteToolNames(); got != "" {
		t.Fatalf("empty names=%q", got)
	}
	if err := a.ConfigureReadOnlyAPI("https://api.example.com", "", "", readonlySpecJSON); err != nil {
		t.Fatal(err)
	}
	if got := a.RemoteToolNames(); got != "list_tickets, get_ticket" {
		t.Fatalf("names=%q", got)
	}
	a.ClearReadOnlyAPI()
	if got := a.RemoteToolNames(); got != "" {
		t.Fatalf("cleared names=%q", got)
	}
}

func TestRemoteToolApproveWithQueryAndAuth(t *testing.T) {
	var businessHits atomic.Int32
	var gotPath, gotQuery, gotAuth string
	business := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		businessHits.Add(1)
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"tickets":[]}`))
	}))
	t.Cleanup(business.Close)

	var llmCalls atomic.Int32
	var sawTool bool
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if llmCalls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id": "c-list", "type": "function",
							"function": map[string]any{
								"name":      "list_tickets",
								"arguments": `{"tenant":"acme","status":"open"}`,
							},
						}},
					},
				}},
			})
			return
		}
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, spec := range req.Tools {
			if spec.Function.Name == "list_tickets" {
				sawTool = true
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "listed"}}},
		})
	}))
	t.Cleanup(llm.Close)

	a := New()
	_ = a.Configure(llm.URL, "k", "m")
	if err := a.ConfigureReadOnlyAPI(business.URL, "Authorization", "Bearer readonly", readonlySpecJSON); err != nil {
		t.Fatal(err)
	}
	raw := a.Send("list")
	var pause map[string]any
	_ = json.Unmarshal([]byte(raw), &pause)
	if pause["status"] != "needs_approval" || pause["name"] != "list_tickets" {
		t.Fatalf("pause=%v", pause)
	}
	if !strings.Contains(pause["summary"].(string), "tenant=acme") {
		t.Fatalf("summary=%v", pause["summary"])
	}
	if businessHits.Load() != 0 || a.RemoteHits() != 0 {
		t.Fatal("network must not run before approve")
	}

	raw = a.Approve("c-list")
	var done map[string]any
	_ = json.Unmarshal([]byte(raw), &done)
	if done["status"] != "done" || done["text"] != "listed" {
		t.Fatalf("done=%v", done)
	}
	if businessHits.Load() != 1 || a.RemoteHits() != 1 || !sawTool {
		t.Fatalf("hits=%d remote=%d sawTool=%v", businessHits.Load(), a.RemoteHits(), sawTool)
	}
	if gotPath != "/tickets" || gotAuth != "Bearer readonly" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	if !strings.Contains(gotQuery, "tenant=acme") || !strings.Contains(gotQuery, "status=open") {
		t.Fatalf("query=%q", gotQuery)
	}
}

func TestRemoteToolDenyDoesNotFetch(t *testing.T) {
	var hits atomic.Int32
	business := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	t.Cleanup(business.Close)
	var llmCalls atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if llmCalls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id": "c-deny", "type": "function",
							"function": map[string]any{
								"name":      "list_tickets",
								"arguments": `{"tenant":"acme"}`,
							},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "denied"}}},
		})
	}))
	t.Cleanup(llm.Close)

	a := New()
	_ = a.Configure(llm.URL, "k", "m")
	_ = a.ConfigureReadOnlyAPI(business.URL, "", "", readonlySpecJSON)
	raw := a.Send("list")
	var pause map[string]any
	_ = json.Unmarshal([]byte(raw), &pause)
	raw = a.Deny("c-deny")
	var done map[string]any
	_ = json.Unmarshal([]byte(raw), &done)
	if done["status"] != "done" || done["text"] != "denied" {
		t.Fatalf("done=%v", done)
	}
	if hits.Load() != 0 || a.RemoteHits() != 0 {
		t.Fatal("deny must not fetch")
	}
}

func TestRemoteToolPathParameter(t *testing.T) {
	var gotPath string
	business := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"id":"T/1"}`))
	}))
	t.Cleanup(business.Close)
	a := New()
	if err := a.ConfigureReadOnlyAPI(business.URL, "", "", readonlySpecJSON); err != nil {
		t.Fatal(err)
	}
	tool := a.remoteToolLocked("get_ticket")
	built, err := tool.buildURL(`{"id":"T/1","expand":"true"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(built, "/tickets/T%2F1?expand=true") {
		t.Fatalf("url=%q", built)
	}
	_, _ = a.execRemoteTool(tool, `{"id":"T/1","expand":"true"}`)
	if gotPath != "/tickets/T/1" {
		t.Fatalf("server path=%q", gotPath)
	}
}
