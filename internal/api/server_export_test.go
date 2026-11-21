package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func exportTestServer(t *testing.T) *Server {
	t.Helper()
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.AdminToken = "adm-token"
	srv.Operators = []controlplane.Operator{
		{ID: "alice", Token: "alice-token"},
		{ID: "bob", Token: "bob-token"},
	}
	srv.Messages = msgs
	if err := msgs.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "alice"}); err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Append("c1", conversation.Message{
		Role: conversation.RoleUser, Content: "你好",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Append("c1", conversation.Message{
		Role: conversation.RoleAssistant, Content: "回答",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Append("c1", conversation.Message{
		Role: conversation.RoleUser, Content: "看图\n![图片](/v0/channels/media/c1/x.png)",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Append("c1", conversation.Message{
		Role: conversation.RoleAssistant, Content: "公开内容", Thinking: "secret thinking",
	}); err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestExportConversationMarkdown(t *testing.T) {
	srv := exportTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/export", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(req, "alice-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/markdown") {
		t.Fatalf("content-type=%q", ct)
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	for _, want := range []string{"# 你好", "## User", "## Assistant", "你好", "回答", "公开内容"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"secret thinking", "/v0/channels/media/"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("leaked %q in:\n%s", forbidden, text)
		}
	}
}

func TestExportConversationACL(t *testing.T) {
	srv := exportTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/export", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(req, "bob-token"))
	if rec.Code == http.StatusOK {
		t.Fatalf("bob must not export alice's conversation: %s", rec.Body.String())
	}
}
