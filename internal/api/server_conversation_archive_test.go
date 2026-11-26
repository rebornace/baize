package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
)

func TestArchiveConversation(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	now := time.Now().UTC()
	for id, when := range map[string]time.Time{
		"c-old": now.Add(-time.Hour),
		"c-new": now,
	} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: "admin", Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: id, CreatedAt: when,
		}); err != nil {
			t.Fatal(err)
		}
	}
	h := srv.Handler()

	archive := httptest.NewRequest(http.MethodPut, "/v0/conversations/c-old/archived",
		bytes.NewBufferString(`{"archived":true}`))
	archive.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, archive)
	if rr.Code != http.StatusOK {
		t.Fatalf("archive=%d %s", rr.Code, rr.Body.String())
	}

	list := func(query string) []conversation.Summary {
		req := httptest.NewRequest(http.MethodGet, "/v0/conversations"+query, nil)
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("list%s=%d %s", query, rr.Code, rr.Body.String())
		}
		var body struct {
			Conversations []conversation.Summary `json:"conversations"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Conversations
	}

	if got := list(""); len(got) != 1 || got[0].ID != "c-new" {
		t.Fatalf("default list=%+v", got)
	}
	only := list("?archived=only")
	if len(only) != 1 || only[0].ID != "c-old" || !only[0].Archived {
		t.Fatalf("archived list=%+v", only)
	}
	if all := list("?archived=all"); len(all) != 2 {
		t.Fatalf("all list=%+v", all)
	}

	// New activity auto-unarchives.
	ctx := controlplane.WithRole(context.Background(), controlplane.RoleAdmin)
	if err := srv.prepareConversationMeta(ctx, "c-old", ""); err != nil {
		t.Fatal(err)
	}
	meta, err := ms.GetMeta("c-old")
	if err != nil || !meta.ArchivedAt.IsZero() {
		t.Fatalf("auto-unarchive meta=%+v err=%v", meta, err)
	}
	if got := list(""); len(got) != 2 {
		t.Fatalf("after unarchive list=%+v", got)
	}
}

func TestArchiveConversationRequiresMeta(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.Messages = conversation.NewMemoryStore()
	req := httptest.NewRequest(http.MethodPut, "/v0/conversations/missing/archived",
		bytes.NewBufferString(`{"archived":true}`))
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d %s", rr.Code, rr.Body.String())
	}
}
