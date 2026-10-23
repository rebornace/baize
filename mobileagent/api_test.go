package mobileagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestPublicSendDeny(t *testing.T) {
	var n atomic.Int32
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"choices": []map[string]any{{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []map[string]any{{
							"id": "x", "type": "function",
							"function": map[string]any{
								"name":      "http_get",
								"arguments": `{"url":"https://example.invalid/"}`,
							},
						}},
					},
				}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "no"}}},
		})
	}))
	t.Cleanup(llm.Close)
	Reset()
	_ = Configure(llm.URL, "k", "m")
	raw := Send("fetch")
	if !strings.Contains(raw, "needs_approval") {
		t.Fatal(raw)
	}
	raw = Deny("x")
	if !strings.Contains(raw, `"done"`) {
		t.Fatal(raw)
	}
}
