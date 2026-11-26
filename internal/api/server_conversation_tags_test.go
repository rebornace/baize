package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

func TestConversationTagsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.ViewerToken = "view"
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	now := time.Now().UTC()
	for _, id := range []string{"c1", "c2"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: "admin", Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: id, CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	put := func(id, tags, token string) int {
		req := httptest.NewRequest(http.MethodPut, "/v0/conversations/"+id+"/tags",
			bytes.NewBufferString(tags))
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		return rr.Code
	}
	if code := put("c1", `{"tags":["Work","Urgent","work"]}`, "adm"); code != http.StatusOK {
		t.Fatalf("c1 tags=%d", code)
	}
	if code := put("c2", `{"tags":["Work"]}`, "adm"); code != http.StatusOK {
		t.Fatalf("c2 tags=%d", code)
	}
	if code := put("c1", `{"tags":["this-tag-is-longer-than-24-characters"]}`, "adm"); code != http.StatusBadRequest {
		t.Fatalf("long tag=%d", code)
	}
	if code := put("c1", `{"tags":["X"]}`, "view"); code != http.StatusForbidden {
		t.Fatalf("viewer tag write=%d", code)
	}

	list := httptest.NewRequest(http.MethodGet, "/v0/conversations?tag=urgent", nil)
	list.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, list)
	if rr.Code != http.StatusOK {
		t.Fatalf("list=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) != 1 || body.Conversations[0].ID != "c1" {
		t.Fatalf("filtered=%+v", body.Conversations)
	}
	if tags := body.Conversations[0].Tags; len(tags) != 2 || tags[0] != "Work" {
		t.Fatalf("tags=%+v", tags)
	}
}
