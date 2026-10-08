package systemoneenable

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeSystemOneOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req["model"] != DefaultModel {
			t.Fatalf("model=%v", req["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"probe": map[string]any{"noul": 0.9}},
		})
	}))
	defer srv.Close()

	if err := ProbeSystemOne(context.Background(), srv.URL, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestProbeSystemOneStripsPathSuffix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{"probe": map[string]any{"noul": 1}},
		})
	}))
	defer srv.Close()

	if err := ProbeSystemOne(context.Background(), srv.URL+"/v1/systemone", "tev1", ""); err != nil {
		t.Fatal(err)
	}
}

func TestProbeSystemOneHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()

	if err := ProbeSystemOne(context.Background(), srv.URL, "tev1", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeSystemOneBase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://127.0.0.1:11434", "http://127.0.0.1:11434"},
		{"http://127.0.0.1:11434/", "http://127.0.0.1:11434"},
		{"http://h/v1/systemone", "http://h"},
		{"http://h/v1/systemone/", "http://h"},
		{"", DefaultOllamaBaseURL},
	}
	for _, c := range cases {
		if got := NormalizeSystemOneBase(c.in); got != c.want {
			t.Fatalf("NormalizeSystemOneBase(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
