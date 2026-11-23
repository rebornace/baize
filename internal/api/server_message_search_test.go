package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func messageSearchServer(t *testing.T) *Server {
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
	for _, meta := range []conversation.Meta{
		{ID: "c-alice", OwnerID: "alice", Source: "ui"},
		{ID: "c-bob", OwnerID: "bob", Source: "ui"},
	} {
		if err := msgs.EnsureMeta(meta); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := msgs.Append("c-alice", conversation.Message{
		Role: conversation.RoleUser, Content: "帮我查 ORD-4242 的物流",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Append("c-bob", conversation.Message{
		Role: conversation.RoleAssistant, Content: "ORD-4242 已签收",
	}); err != nil {
		t.Fatal(err)
	}
	return srv
}

func getMessageSearchRaw(t *testing.T, srv *Server, token, query string) (int, []messageSearchHitJSON) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/search?"+query, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(req, token))
	var body struct {
		Hits []messageSearchHitJSON `json:"hits"`
	}
	if rec.Code == http.StatusOK {
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, body.Hits
}

func getMessageSearch(t *testing.T, srv *Server, token, query string) []messageSearchHitJSON {
	t.Helper()
	code, hits := getMessageSearchRaw(t, srv, token, "q="+query)
	if code != http.StatusOK {
		t.Fatalf("search status=%d", code)
	}
	return hits
}

func TestMessageSearchAdminSeesAll(t *testing.T) {
	srv := messageSearchServer(t)
	hits := getMessageSearch(t, srv, "adm-token", "ORD-4242")
	if len(hits) != 2 {
		t.Fatalf("hits=%+v", hits)
	}
	for _, hit := range hits {
		if hit.Snippet == "" || hit.MessageID == "" || hit.ConversationID == "" {
			t.Fatalf("hit=%+v", hit)
		}
		if hit.ConversationID == "c-alice" && hit.ConversationTitle == "" {
			t.Fatalf("missing title: %+v", hit)
		}
	}
}

func TestMessageSearchOperatorOnlyOwnConversations(t *testing.T) {
	srv := messageSearchServer(t)
	hits := getMessageSearch(t, srv, "alice-token", "ORD-4242")
	if len(hits) != 1 || hits[0].ConversationID != "c-alice" {
		t.Fatalf("alice hits=%+v", hits)
	}
}

func TestMessageSearchEmptyQuery(t *testing.T) {
	srv := messageSearchServer(t)
	hits := getMessageSearch(t, srv, "adm-token", "")
	if len(hits) != 0 {
		t.Fatalf("hits=%+v", hits)
	}
}

func TestMessageSearchRoleFilter(t *testing.T) {
	srv := messageSearchServer(t)
	code, hits := getMessageSearchRaw(t, srv, "adm-token", "q=ORD-4242&role=user")
	if code != http.StatusOK || len(hits) != 1 || hits[0].Role != conversation.RoleUser {
		t.Fatalf("admin user role code=%d hits=%+v", code, hits)
	}
	code, hits = getMessageSearchRaw(t, srv, "adm-token", "q=ORD-4242&role=assistant")
	if code != http.StatusOK || len(hits) != 1 || hits[0].Role != conversation.RoleAssistant {
		t.Fatalf("admin assistant role code=%d hits=%+v", code, hits)
	}
	code, hits = getMessageSearchRaw(t, srv, "alice-token", "q=ORD-4242&role=assistant")
	if code != http.StatusOK || len(hits) != 0 {
		t.Fatalf("alice assistant role code=%d hits=%+v", code, hits)
	}
	code, _ = getMessageSearchRaw(t, srv, "adm-token", "q=ORD-4242&role=bogus")
	if code != http.StatusBadRequest {
		t.Fatalf("invalid role code=%d", code)
	}
}

func TestMessageSearchConversationFilter(t *testing.T) {
	srv := messageSearchServer(t)
	code, hits := getMessageSearchRaw(t, srv, "adm-token", "q=ORD-4242&conversation_id=c-bob")
	if code != http.StatusOK || len(hits) != 1 || hits[0].ConversationID != "c-bob" {
		t.Fatalf("admin conversation code=%d hits=%+v", code, hits)
	}
	code, hits = getMessageSearchRaw(t, srv, "alice-token", "q=ORD-4242&conversation_id=c-bob")
	if code != http.StatusOK || len(hits) != 0 {
		t.Fatalf("alice must not search bob's conversation: code=%d hits=%+v", code, hits)
	}
}

func TestMessageSearchDaysFilter(t *testing.T) {
	srv := messageSearchServer(t)
	code, hits := getMessageSearchRaw(t, srv, "adm-token", "q=ORD-4242&days=1")
	if code != http.StatusOK || len(hits) != 2 {
		t.Fatalf("days code=%d hits=%+v", code, hits)
	}
	for _, raw := range []string{"days=0", "days=-1", "days=abc", "days=99999"} {
		if code, _ := getMessageSearchRaw(t, srv, "adm-token", "q=ORD-4242&"+raw); code != http.StatusBadRequest {
			t.Fatalf("%s code=%d", raw, code)
		}
	}
}
