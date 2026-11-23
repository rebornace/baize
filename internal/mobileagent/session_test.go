package mobileagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionSnapshotRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`)
	}))
	defer srv.Close()

	a := newTestAgent()
	if err := a.Configure(srv.URL, "super-secret-key", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.Send("first"); !strings.Contains(got, `"done"`) {
		t.Fatalf("send=%s", got)
	}
	raw := a.ExportSession()
	if strings.Contains(raw, "super-secret-key") || strings.Contains(raw, srv.URL) {
		t.Fatalf("snapshot leaked secrets: %s", raw)
	}
	var snap sessionSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.MessageCount != 2 || len(snap.Messages) != 2 {
		t.Fatalf("snapshot=%+v", snap)
	}

	b := New()
	if got := b.ImportSession(raw); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("import=%s", got)
	}
	if b.MessageCount() != 2 {
		t.Fatalf("restored messages=%d", b.MessageCount())
	}
	b.mu.Lock()
	restored := append([]chatMessage(nil), b.msgs...)
	b.mu.Unlock()
	if restored[0].Content != "first" || restored[1].Content != "hello" {
		t.Fatalf("restored=%+v", restored)
	}
}

func TestSessionSnapshotToolCallRoundTrip(t *testing.T) {
	a := New()
	raw := `{"version":1,"messages":[
	  {"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"get_time","arguments":"{}"}}]},
	  {"role":"tool","tool_call_id":"c1","content":"2026-11-22T00:00:00Z"},
	  {"role":"assistant","content":"done"}
	]}`
	if got := a.ImportSession(raw); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("import=%s", got)
	}
	out := a.ExportSession()
	if !strings.Contains(out, `"tool_call_id":"c1"`) || !strings.Contains(out, `"name":"get_time"`) {
		t.Fatalf("roundtrip lost tool call: %s", out)
	}
}

func TestSessionSnapshotDropsPendingToolCall(t *testing.T) {
	a := New()
	a.msgs = []chatMessage{
		{Role: "user", Content: "create"},
		{Role: "assistant", ToolCalls: []toolCall{{ID: "c1", Type: "function"}}},
	}
	raw := a.ExportSession()
	if strings.Contains(raw, `"c1"`) {
		t.Fatalf("pending call leaked into snapshot: %s", raw)
	}
	b := New()
	if got := b.ImportSession(raw); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("import pending snapshot=%s", got)
	}
	if b.MessageCount() != 1 {
		t.Fatalf("restored=%d want 1", b.MessageCount())
	}
}

func TestSessionSnapshotRejectsInvalidHistory(t *testing.T) {
	cases := []string{
		`{"version":1,"messages":[{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"x","arguments":"{}"}}]}]}`,
		`{"version":1,"messages":[{"role":"tool","tool_call_id":"missing","content":"x"}]}`,
		`{"version":1,"messages":[{"role":"wizard","content":"x"}]}`,
		`{"version":1,"messages":[{"role":"user","content":""}]}`,
		`{"version":9,"messages":[]}`,
	}
	for _, raw := range cases {
		a := New()
		if got := a.ImportSession(raw); !strings.Contains(got, `"status":"error"`) {
			t.Fatalf("import %s => %s, want error", raw, got)
		}
	}
}
