package mobileagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicBindAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "bound-ok"}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	Reset()
	if err := Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	out, err := ChatOnce("hi")
	if err != nil {
		t.Fatal(err)
	}
	if out != "bound-ok" {
		t.Fatalf("got %q", out)
	}
}
