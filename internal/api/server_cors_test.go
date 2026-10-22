package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSPreflightAndReflect(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.CORSAllowOrigins = []string{"https://localhost", "capacitor://localhost"}
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodOptions, "/v0/me", nil)
	req.Header.Set("Origin", "https://localhost")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status: got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://localhost" {
		t.Fatalf("ACAO: got %q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req2.Header.Set("Origin", "https://evil.example")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected ACAO for disallowed origin")
	}
}

func TestCORSStarReflects(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.CORSAllowOrigins = []string{"*"}
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "https://anywhere.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://anywhere.example" {
		t.Fatalf("ACAO: got %q", got)
	}
}
