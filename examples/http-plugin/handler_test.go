package httppluginex_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httppluginex "github.com/rebornace/baize/examples/http-plugin"
)

func withProto(r *http.Request) {
	r.Header.Set("X-Baize-Protocol", "v0")
}

func TestProtocolHeaderRequired(t *testing.T) {
	h := httppluginex.NewHandler()

	paths := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/healthz"},
		{http.MethodGet, "/v0/tools"},
		{http.MethodPost, "/v0/tools/echo/invoke"},
	}
	for _, tc := range paths {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "protocol_unsupported") {
				t.Fatalf("body=%s", rr.Body.String())
			}
			var body map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatalf("json: %v body=%s", err, rr.Body.String())
			}
			errObj, _ := body["error"].(map[string]any)
			if errObj == nil || errObj["code"] != "protocol_unsupported" {
				t.Fatalf("error=%v", body["error"])
			}
		})
	}
}

func TestEchoAndCreateTicket(t *testing.T) {
	h := httppluginex.NewHandler()

	// GET /healthz with proto → ok
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	withProto(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status=%d body=%s", rr.Code, rr.Body.String())
	}
	var health map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &health); err != nil {
		t.Fatal(err)
	}
	if health["status"] != "ok" {
		t.Fatalf("health=%v", health)
	}

	// GET /v0/tools → names echo, create_ticket
	req = httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	withProto(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("tools status=%d body=%s", rr.Code, rr.Body.String())
	}
	var toolsBody struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &toolsBody); err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, tool := range toolsBody.Tools {
		names[tool.Name] = tool.Description
	}
	if _, ok := names["echo"]; !ok {
		t.Fatalf("missing echo: %+v", toolsBody.Tools)
	}
	if _, ok := names["login"]; !ok {
		t.Fatalf("missing login: %+v", toolsBody.Tools)
	}
	desc, ok := names["create_ticket"]
	if !ok {
		t.Fatalf("missing create_ticket: %+v", toolsBody.Tools)
	}
	if !strings.Contains(strings.ToLower(desc), "write") &&
		!strings.Contains(desc, "写") &&
		!strings.Contains(strings.ToLower(desc), "creat") {
		t.Fatalf("create_ticket description should indicate write side effect: %q", desc)
	}

	// POST invoke echo arguments {"ping":1} → content 含 ping
	echoBody, _ := json.Marshal(map[string]any{
		"arguments": map[string]any{"ping": 1},
	})
	req = httptest.NewRequest(http.MethodPost, "/v0/tools/echo/invoke", bytes.NewReader(echoBody))
	withProto(req)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("echo status=%d body=%s", rr.Code, rr.Body.String())
	}
	var echoResp struct {
		Content map[string]any `json:"content"`
		IsError bool           `json:"is_error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &echoResp); err != nil {
		t.Fatal(err)
	}
	if echoResp.IsError {
		t.Fatalf("echo is_error: %+v", echoResp)
	}
	if echoResp.Content["ping"] != float64(1) {
		t.Fatalf("echo content=%v", echoResp.Content)
	}

	// POST invoke create_ticket {"title":"x"} → content.id 非空
	createBody, _ := json.Marshal(map[string]any{
		"arguments": map[string]any{"title": "x"},
	})
	req = httptest.NewRequest(http.MethodPost, "/v0/tools/create_ticket/invoke", bytes.NewReader(createBody))
	withProto(req)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create_ticket status=%d body=%s", rr.Code, rr.Body.String())
	}
	var createResp struct {
		Content map[string]any `json:"content"`
		IsError bool           `json:"is_error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &createResp); err != nil {
		t.Fatal(err)
	}
	if createResp.IsError {
		t.Fatalf("create is_error: %+v", createResp)
	}
	id, _ := createResp.Content["id"].(string)
	if id == "" {
		t.Fatalf("content=%v", createResp.Content)
	}
	if createResp.Content["title"] != "x" {
		t.Fatalf("content=%v", createResp.Content)
	}
}

func TestLoginInvoke(t *testing.T) {
	h := httppluginex.NewHandler()
	body, _ := json.Marshal(map[string]any{"arguments": map[string]any{}})
	req := httptest.NewRequest(http.MethodPost, "/v0/tools/login/invoke", bytes.NewReader(body))
	withProto(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Content map[string]any `json:"content"`
		IsError bool           `json:"is_error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.IsError {
		t.Fatalf("login is_error: %+v", resp)
	}
	token, _ := resp.Content["accessToken"].(string)
	if token == "" {
		t.Fatalf("content=%v", resp.Content)
	}
}

func TestCreateTicketMissingTitle(t *testing.T) {
	h := httppluginex.NewHandler()
	body, _ := json.Marshal(map[string]any{"arguments": map[string]any{}})
	req := httptest.NewRequest(http.MethodPost, "/v0/tools/create_ticket/invoke", bytes.NewReader(body))
	withProto(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		IsError bool `json:"is_error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.IsError {
		t.Fatalf("want is_error body=%s", rr.Body.String())
	}
}

func TestUnknownTool(t *testing.T) {
	h := httppluginex.NewHandler()
	body, _ := json.Marshal(map[string]any{"arguments": map[string]any{}})
	req := httptest.NewRequest(http.MethodPost, "/v0/tools/no_such_tool/invoke", bytes.NewReader(body))
	withProto(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"error"`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestListTicketsOptional(t *testing.T) {
	h := httppluginex.NewHandler()

	createBody, _ := json.Marshal(map[string]any{
		"arguments": map[string]any{"title": "side-effect"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v0/tools/create_ticket/invoke", bytes.NewReader(createBody))
	withProto(req)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create status=%d", rr.Code)
	}

	// GET /tickets 可不校验协议头
	req = httptest.NewRequest(http.MethodGet, "/tickets", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("tickets status=%d body=%s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["title"] != "side-effect" {
		t.Fatalf("list=%v", list)
	}
}
