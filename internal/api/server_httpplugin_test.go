package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPutHTTPConnectorRegistersTools(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sidecar.Close()

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/side",
		jsonBody(t, map[string]any{"type": "http", "base_url": sidecar.URL}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"spec":`) && strings.Contains(rr.Body.String(), `"examples`) {
		t.Fatal("http connector should not require spec")
	}
	var body struct {
		ID    string      `json:"id"`
		Type  string      `json:"type"`
		Spec  string      `json:"spec"`
		Tools []tool.Info `json:"tools"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "side" || body.Type != "http" || body.Spec != "" {
		t.Fatalf("body=%+v", body)
	}
	if len(body.Tools) != 1 || body.Tools[0].Name != "echo" {
		t.Fatalf("tools=%+v", body.Tools)
	}
	if len(reg.List()) != 1 || reg.List()[0].Name != "echo" {
		t.Fatalf("registry=%+v", reg.List())
	}
	c, err := st.GetConnector("side")
	if err != nil || c.Type != "http" || c.Spec != "" || c.BaseURL != sidecar.URL {
		t.Fatalf("store %+v err=%v", c, err)
	}
}

func TestPutHTTPMissingBaseURL(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/side",
		jsonBody(t, map[string]any{"type": "http"}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_request" {
		t.Fatalf("code=%q", wrap.Error.Code)
	}
	if !strings.Contains(wrap.Error.Message, "base_url is required") {
		t.Fatalf("message=%q", wrap.Error.Message)
	}
	if len(reg.List()) != 0 {
		t.Fatalf("registry polluted: %+v", reg.List())
	}
}

func TestPutHTTPInvalidPlugin(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sidecar.Close()

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/side",
		jsonBody(t, map[string]any{"type": "http", "base_url": sidecar.URL}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_plugin" {
		t.Fatalf("code=%q", wrap.Error.Code)
	}
	if len(reg.List()) != 0 {
		t.Fatalf("registry polluted: %+v", reg.List())
	}
}

func TestPutOpenAPIStillRequiresSpec(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})

	for _, body := range []map[string]any{
		{"type": "openapi", "base_url": "http://x"},
		{"base_url": "http://x"},
	} {
		req := httptest.NewRequest(http.MethodPut, "/v0/connectors/ticket", jsonBody(t, body))
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("body=%v status=%d resp=%s", body, rr.Code, rr.Body.String())
		}
		var wrap struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
			t.Fatal(err)
		}
		if wrap.Error.Code != "invalid_request" || !strings.Contains(wrap.Error.Message, "spec is required") {
			t.Fatalf("body=%v error=%+v", body, wrap.Error)
		}
	}
}

func TestPutUnsupportedConnectorType(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/side",
		jsonBody(t, map[string]any{"type": "grpc", "base_url": "http://x"}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_request" {
		t.Fatalf("code=%q", wrap.Error.Code)
	}
	if !strings.Contains(strings.ToLower(wrap.Error.Message), "unsupported connector type") {
		t.Fatalf("message=%q", wrap.Error.Message)
	}
}
