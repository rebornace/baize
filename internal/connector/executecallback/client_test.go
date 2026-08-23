package executecallback_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/connector/executecallback"
	"github.com/rebornace/baize/internal/connector/httpplugin"
)

func TestClientInvoke(t *testing.T) {
	var got struct {
		Tool            string         `json:"tool"`
		Arguments       map[string]any `json:"arguments"`
		RunID           string         `json:"run_id"`
		AgentID         string         `json:"agent_id"`
		IdempotencyKey  string         `json:"idempotency_key"`
	}
	var protocol, runHdr, authHdr string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocol = r.Header.Get(httpplugin.HeaderProtocol)
		runHdr = r.Header.Get(httpplugin.HeaderRunID)
		authHdr = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": map[string]any{"echo": true},
			"is_error": false,
		})
	}))
	defer srv.Close()

	client := executecallback.NewClient(srv.URL)
	out, err := client.Invoke(context.Background(), "echo", map[string]any{"x": 1}, executecallback.InvokeMeta{
		RunID:          "run_1",
		AgentID:        "agent_1",
		IdempotencyKey: "call_abc",
		Headers:        map[string]string{"Authorization": "Bearer tok"},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if out.IsError || got.Tool != "echo" || got.RunID != "run_1" || got.AgentID != "agent_1" || got.IdempotencyKey != "call_abc" {
		t.Fatalf("body=%+v out=%+v", got, out)
	}
	if protocol != httpplugin.ProtocolV0 || runHdr != "run_1" || authHdr != "Bearer tok" {
		t.Fatalf("headers protocol=%s run=%s auth=%s", protocol, runHdr, authHdr)
	}
}
