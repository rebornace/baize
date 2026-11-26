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

func TestPinConversation(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	now := time.Now().UTC()
	for id, when := range map[string]time.Time{
		"c-old": now.Add(-time.Hour),
		"c-new": now,
	} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: id, CreatedAt: when,
		}); err != nil {
			t.Fatal(err)
		}
	}
	h := srv.Handler()

	pin := httptest.NewRequest(http.MethodPut, "/v0/conversations/c-old/pin",
		bytes.NewBufferString(`{"pinned":true}`))
	pin.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, pin)
	if rr.Code != http.StatusOK {
		t.Fatalf("pin=%d %s", rr.Code, rr.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	list.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, list)
	if rr.Code != http.StatusOK {
		t.Fatalf("list=%d", rr.Code)
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) != 2 || body.Conversations[0].ID != "c-old" ||
		!body.Conversations[0].Pinned {
		t.Fatalf("conversations=%+v", body.Conversations)
	}

	unpin := httptest.NewRequest(http.MethodPut, "/v0/conversations/c-old/pin",
		bytes.NewBufferString(`{"pinned":false}`))
	unpin.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, unpin)
	if rr.Code != http.StatusOK {
		t.Fatalf("unpin=%d", rr.Code)
	}
	list = httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	list.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, list)
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Conversations[0].ID != "c-new" || body.Conversations[1].Pinned {
		t.Fatalf("after unpin=%+v", body.Conversations)
	}
}
