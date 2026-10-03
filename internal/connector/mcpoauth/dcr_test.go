package mcpoauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterClientPrefersAuthMethodNone(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/register" {
			http.NotFound(w, r)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":     "dcr-client-1",
			"client_secret": "dcr-secret-1",
		})
	}))
	defer srv.Close()

	reg, err := RegisterClient(context.Background(), srv.Client(), srv.URL+"/register", "https://app.example/callback")
	if err != nil {
		t.Fatal(err)
	}
	if reg.ClientID != "dcr-client-1" {
		t.Fatalf("client_id=%q", reg.ClientID)
	}
	if reg.ClientSecret != "dcr-secret-1" {
		t.Fatalf("client_secret=%q", reg.ClientSecret)
	}
	if gotBody["token_endpoint_auth_method"] != "none" {
		t.Fatalf("auth_method=%v want none", gotBody["token_endpoint_auth_method"])
	}
	uris, _ := gotBody["redirect_uris"].([]any)
	if len(uris) != 1 || uris[0] != "https://app.example/callback" {
		t.Fatalf("redirect_uris=%v", gotBody["redirect_uris"])
	}
}
