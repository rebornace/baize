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

func viewerServer(t *testing.T) *Server {
	t.Helper()
	srv := gateServer(t, "op", "adm")
	srv.ViewerToken = "view"
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	now := time.Now().UTC()
	for _, id := range []string{"c-admin", "c-viewer"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: "admin", Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: id, CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return srv
}

func TestViewerReadsAcrossOwnersButCannotWrite(t *testing.T) {
	srv := viewerServer(t)
	h := srv.Handler()

	list := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	list.Header.Set("Authorization", "Bearer view")
	rr := httptest.NewRecorder()
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
	if len(body.Conversations) != 2 {
		t.Fatalf("viewer should see all conversations: %+v", body.Conversations)
	}

	write := httptest.NewRequest(http.MethodPut, "/v0/conversations/c-admin/pin",
		bytes.NewBufferString(`{"pinned":true}`))
	write.Header.Set("Authorization", "Bearer view")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, write)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("viewer pin=%d", rr.Code)
	}

	run := httptest.NewRequest(http.MethodPost, "/v0/runs",
		bytes.NewBufferString(`{"agent_id":"a","input":"x"}`))
	run.Header.Set("Authorization", "Bearer view")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, run)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("viewer run=%d", rr.Code)
	}

	audit := httptest.NewRequest(http.MethodGet, "/v0/audit/events", nil)
	audit.Header.Set("Authorization", "Bearer view")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, audit)
	if rr.Code != http.StatusOK {
		t.Fatalf("viewer audit=%d %s", rr.Code, rr.Body.String())
	}

	settings := httptest.NewRequest(http.MethodGet, "/v0/settings/credentials", nil)
	settings.Header.Set("Authorization", "Bearer view")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, settings)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("viewer credentials=%d", rr.Code)
	}
}

func TestScopedViewer(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.ViewerToken = "view"
	srv.ViewerWorkspace = "ws1"
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	now := time.Now().UTC()
	for id, ws := range map[string]string{"c-ws1": "ws1", "c-ws2": "ws2"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: "admin", WorkspaceID: ws, Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: id, CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	h := srv.Handler()
	list := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	list.Header.Set("Authorization", "Bearer view")
	rr := httptest.NewRecorder()
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
	if len(body.Conversations) != 1 || body.Conversations[0].ID != "c-ws1" {
		t.Fatalf("scoped list=%+v", body.Conversations)
	}

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v0/conversations/c-ws2/messages"},
		{http.MethodGet, "/v0/audit/events"},
		{http.MethodGet, "/v0/settings/cost-summary"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer view")
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s %s=%d", tc.method, tc.path, rr.Code)
		}
	}

	me := httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	me.Header.Set("Authorization", "Bearer view")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, me)
	var meBody struct {
		Workspace string `json:"viewer_workspace"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &meBody); err != nil || meBody.Workspace != "ws1" {
		t.Fatalf("me=%+v err=%v", meBody, err)
	}
}
