package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

func TestRenameConversation(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	if err := ms.EnsureMeta(conversation.Meta{ID: "c1", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ms.Append("c1", conversation.Message{
		Role: conversation.RoleUser, Content: "hello", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPatch, "/v0/conversations/c1",
		bytes.NewBufferString(`{"title":"  新标题  "}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("rename=%d %s", rr.Code, rr.Body.String())
	}
	meta, err := ms.GetMeta("c1")
	if err != nil || meta.Title != "新标题" {
		t.Fatalf("meta=%+v err=%v", meta, err)
	}

	list := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	list.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, list)
	if rr.Code != http.StatusOK {
		t.Fatalf("list=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range body.Conversations {
		if c.ID == "c1" {
			found = true
			if c.Title != "新标题" {
				t.Fatalf("list title=%q", c.Title)
			}
		}
	}
	if !found {
		t.Fatalf("c1 missing from %+v", body.Conversations)
	}

	bad := httptest.NewRequest(http.MethodPatch, "/v0/conversations/c1",
		bytes.NewBufferString(`{"title":"`+strings.Repeat("a", 101)+`"}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("too long=%d", rr.Code)
	}

	missing := httptest.NewRequest(http.MethodPatch, "/v0/conversations/c-missing",
		bytes.NewBufferString(`{"title":"x"}`))
	missing.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, missing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing=%d", rr.Code)
	}
}
