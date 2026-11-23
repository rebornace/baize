package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func choiceTestServer(t *testing.T) (*httptest.Server, func() []byte) {
	t.Helper()
	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	return srv, func() []byte { return capturedBody }
}

// TestChatWithChoiceSendsToolChoice verifies the DP-2b path serializes a
// tool_choice field and uses the supplied enum discipline.
func TestChatWithChoiceSendsToolChoice(t *testing.T) {
	srv, body := choiceTestServer(t)
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	ch, ok := any(p).(llm.Chooser)
	if !ok {
		t.Fatal("OpenAI must implement Chooser")
	}
	_, err := ch.ChatWithChoice(context.Background(),
		[]llm.Message{{Role: llm.RoleUser, Content: "go"}},
		[]llm.ToolSpec{{Name: "t1", Description: "d"}},
		llm.ToolChoice{Type: llm.ToolChoiceRequired})
	if err != nil {
		t.Fatalf("chat with choice: %v", err)
	}
	if !strings.Contains(string(body()), `"tool_choice":"required"`) {
		t.Fatalf("missing tool_choice: %s", body())
	}
}

// TestChatWithoutChoiceOmitsToolChoice verifies the ordinary path stays
// byte-identical to today: no tool_choice key is emitted.
func TestChatWithoutChoiceOmitsToolChoice(t *testing.T) {
	srv, body := choiceTestServer(t)
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	_, err := p.Chat(context.Background(),
		[]llm.Message{{Role: llm.RoleUser, Content: "go"}},
		[]llm.ToolSpec{{Name: "t1", Description: "d"}})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if strings.Contains(string(body()), "tool_choice") {
		t.Fatalf("ordinary call must not send tool_choice: %s", body())
	}
}

// TestChatWithChoiceNamed verifies a named tool_choice renders the object form.
func TestChatWithChoiceNamed(t *testing.T) {
	srv, body := choiceTestServer(t)
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	_, err := p.ChatWithChoice(context.Background(),
		[]llm.Message{{Role: llm.RoleUser, Content: "go"}},
		[]llm.ToolSpec{{Name: "t1", Description: "d"}},
		llm.ToolChoice{Type: llm.ToolChoiceNamed, Name: "t1"})
	if err != nil {
		t.Fatalf("chat named: %v", err)
	}
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body(), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tc, ok := req["tool_choice"]
	if !ok {
		t.Fatalf("missing tool_choice: %s", body())
	}
	if !strings.Contains(string(tc), `"name":"t1"`) {
		t.Fatalf("named tool_choice wrong: %s", tc)
	}
}
