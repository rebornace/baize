package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterRouteMountsHandler(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.RegisterRoute("POST /v0/channels/demo/inbound", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("hit"))
	}))
	req := httptest.NewRequest("POST", "/v0/channels/demo/inbound", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req) // Handler() returns the gated mux; gate off with no tokens
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
}
