package api

import (
	"encoding/json"
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
	if _, err := msgs.Append("c1", conversation.Message{
		Role: conversation.RoleUser, Content: "<b>加粗</b>",
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

func TestExportConversationJSON(t *testing.T) {
	srv := exportTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/export?format=json", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(req, "alice-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type=%q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, ".json") {
		t.Fatalf("content-disposition=%q", cd)
	}
	var doc exportDocument
	if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.ConversationID != "c1" || doc.IncludeThinking || len(doc.Messages) != 5 {
		t.Fatalf("doc=%+v", doc)
	}
	var attachmentSeen bool
	for _, msg := range doc.Messages {
		if msg.Thinking != "" {
			t.Fatal("thinking must be omitted by default")
		}
		for _, ref := range msg.Attachments {
			if ref.Filename == "x.png" {
				attachmentSeen = true
			}
		}
	}
	if !attachmentSeen {
		t.Fatalf("attachments missing: %+v", doc.Messages)
	}
}

func TestExportConversationJSONIncludesThinking(t *testing.T) {
	srv := exportTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/export?format=json&include_thinking=1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(req, "alice-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var doc exportDocument
	if err := json.NewDecoder(rec.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if !doc.IncludeThinking {
		t.Fatal("include_thinking not reflected")
	}
	found := false
	for _, msg := range doc.Messages {
		if strings.Contains(msg.Content, "公开内容") {
			found = msg.Thinking == "secret thinking"
		}
	}
	if !found {
		t.Fatalf("thinking missing: %+v", doc.Messages)
	}
}

func TestExportConversationHTML(t *testing.T) {
	srv := exportTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/export?format=html", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(req, "alice-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, ".html") {
		t.Fatalf("content-disposition=%q", cd)
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	for _, want := range []string{"<article", "x.png", "&lt;b&gt;加粗&lt;/b&gt;", "Attachments:"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"secret thinking", "/v0/channels/media/", "<b>加粗</b>"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("leaked %q", forbidden)
		}
	}
}

func TestExportConversationMarkdownIncludesThinking(t *testing.T) {
	srv := exportTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/export?format=markdown&include_thinking=1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(req, "alice-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "secret thinking") || !strings.Contains(string(body), "Thinking") {
		t.Fatalf("thinking missing: %s", body)
	}
}

func TestExportConversationRejectsBadFormat(t *testing.T) {
	srv := exportTestServer(t)
	for _, raw := range []string{"?format=xml", "?include_thinking=maybe"} {
		req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/export"+raw, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, withNamedOp(req, "alice-token"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d body=%s", raw, rec.Code, rec.Body.String())
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
