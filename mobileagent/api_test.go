package mobileagent

import (
	"encoding/json"
	"fmt"
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

func TestPublicReadOnlyAPIConfig(t *testing.T) {
	Reset()
	spec := `{"openapi":"3.0.3","servers":[{"url":"https://api.example.com"}],"paths":{"/x":{"get":{"operationId":"read_x"}}}}`
	if err := ConfigureReadOnlyAPI("", "", "", spec); err != nil {
		t.Fatal(err)
	}
	if RemoteToolCount() != 1 {
		t.Fatalf("tools=%d", RemoteToolCount())
	}
	if got := RemoteToolNames(); got != "read_x" {
		t.Fatalf("names=%q", got)
	}
	ClearReadOnlyAPI()
	if RemoteToolCount() != 0 {
		t.Fatalf("clear tools=%d", RemoteToolCount())
	}
	if got := RemoteToolNames(); got != "" {
		t.Fatalf("clear names=%q", got)
	}
}

func TestPublicReadOnlyAPIsConfig(t *testing.T) {
	Reset()
	specA := `{"openapi":"3.0.3","servers":[{"url":"https://a.example.com"}],"paths":{"/a":{"get":{"operationId":"read_a"}}}}`
	specB := `{"openapi":"3.0.3","servers":[{"url":"https://b.example.com"}],"paths":{"/b":{"get":{"operationId":"read_b"}}}}`
	raw := fmt.Sprintf(`[{"id":"a","spec":%q},{"id":"b","spec":%q}]`, specA, specB)
	if err := ConfigureReadOnlyAPIs(raw); err != nil {
		t.Fatal(err)
	}
	if RemoteToolCount() != 2 {
		t.Fatalf("tools=%d", RemoteToolCount())
	}
	if names := RemoteToolNames(); names != "read_a, read_b" {
		t.Fatalf("names=%q", names)
	}
	ClearReadOnlyAPI()
	if RemoteToolCount() != 0 {
		t.Fatalf("clear tools=%d", RemoteToolCount())
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
