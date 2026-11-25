package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestMoveConversationWorkspace(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	lab, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c1", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPatch, "/v0/conversations/c1/workspace",
		bytes.NewBufferString(`{"workspace_id":"`+lab.ID+`"}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("move=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		ConversationID string `json:"conversation_id"`
		WorkspaceID    string `json:"workspace_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ConversationID != "c1" || body.WorkspaceID != lab.ID {
		t.Fatalf("body=%+v", body)
	}
	got, err := ms.GetMeta("c1")
	if err != nil || got.WorkspaceID != lab.ID {
		t.Fatalf("meta=%+v err=%v", got, err)
	}

	missing := httptest.NewRequest(http.MethodPatch, "/v0/conversations/c1/workspace",
		bytes.NewBufferString(`{"workspace_id":"ws_nope"}`))
	missing.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, missing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown workspace=%d", rr.Code)
	}

	noMeta := httptest.NewRequest(http.MethodPatch, "/v0/conversations/c-missing/workspace",
		bytes.NewBufferString(`{"workspace_id":"`+lab.ID+`"}`))
	noMeta.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, noMeta)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown conversation=%d", rr.Code)
	}
}

func TestMoveConversationWorkspaceRejectsActiveRun(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-busy", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-busy",
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v0/conversations/c-busy/workspace",
		bytes.NewBufferString(`{"workspace_id":"default"}`))
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("active run=%d %s", rr.Code, rr.Body.String())
	}
}
