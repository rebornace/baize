package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

func listSummaries(t *testing.T, srv *Server, token string) []conversation.Summary {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Conversations
}

func TestConversationReadState(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	base := time.Now().UTC().Add(-2 * time.Second)
	for _, id := range []string{"c1", "c2"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: "admin", Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: id, CreatedAt: base,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// No read marker yet: existing conversations stay quiet.
	for _, sum := range listSummaries(t, srv, "adm") {
		if sum.Unread {
			t.Fatalf("unexpected unread before any marker: %+v", sum)
		}
	}

	mark := func(id string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPut, "/v0/conversations/"+id+"/read", nil)
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("mark %s=%d %s", id, rr.Code, rr.Body.String())
		}
	}
	mark("c1")
	// New activity after c1's marker; c2 has no marker and stays quiet.
	if _, err := ms.Append("c2", conversation.Message{
		Role: conversation.RoleUser, Content: "new", CreatedAt: time.Now().UTC().Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got := listSummaries(t, srv, "adm")
	byID := map[string]conversation.Summary{}
	for _, sum := range got {
		byID[sum.ID] = sum
	}
	if byID["c1"].Unread {
		t.Fatalf("c1 should be read: %+v", byID["c1"])
	}
	if byID["c2"].Unread {
		t.Fatalf("c2 without marker should stay quiet: %+v", byID["c2"])
	}

	mark("c2")
	if _, err := ms.Append("c1", conversation.Message{
		Role: conversation.RoleUser, Content: "newer", CreatedAt: time.Now().UTC().Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	got = listSummaries(t, srv, "adm")
	for _, sum := range got {
		if sum.ID == "c1" && !sum.Unread {
			t.Fatalf("c1 should be unread after new activity: %+v", sum)
		}
	}
}

func TestMarkAllRead(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	base := time.Now().UTC().Add(-time.Minute)
	for _, id := range []string{"c1", "c2"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: "admin", Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: id, CreatedAt: base,
		}); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/conversations/read-all", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("read-all=%d %s", rr.Code, rr.Body.String())
	}
	for _, sum := range listSummaries(t, srv, "adm") {
		if sum.Unread {
			t.Fatalf("all should be read: %+v", sum)
		}
	}
	if _, err := ms.Append("c2", conversation.Message{
		Role: conversation.RoleUser, Content: "new", CreatedAt: time.Now().UTC().Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	for _, sum := range listSummaries(t, srv, "adm") {
		if sum.ID == "c2" && !sum.Unread {
			t.Fatalf("c2 should be unread after new activity: %+v", sum)
		}
		if sum.ID == "c1" && sum.Unread {
			t.Fatalf("c1 should stay read: %+v", sum)
		}
	}
}
