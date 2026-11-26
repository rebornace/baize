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

func TestBatchMoveConversations(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	lab, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"c1", "c2"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, Source: "ui"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c2",
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/conversations/batch/workspace",
		bytes.NewBufferString(`{"conversation_ids":["c1","c2"],"workspace_id":"`+lab.ID+`"}`))
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("batch move=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Moved   int             `json:"moved"`
		Skipped []batchSkipJSON `json:"skipped"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Moved != 1 || len(body.Skipped) != 1 || body.Skipped[0].ID != "c2" ||
		body.Skipped[0].Reason != "conversation_busy" {
		t.Fatalf("body=%+v", body)
	}
	meta, err := ms.GetMeta("c1")
	if err != nil || meta.WorkspaceID != lab.ID {
		t.Fatalf("c1 meta=%+v err=%v", meta, err)
	}
}

func TestBatchDeleteConversations(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	for _, id := range []string{"c1", "c2"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, Source: "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(id, conversation.Message{
			Role: conversation.RoleUser, Content: "hi",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c3",
	}); err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c3", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/conversations/batch/delete",
		bytes.NewBufferString(`{"conversation_ids":["c1","c2","c3"]}`))
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("batch delete=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Deleted int             `json:"deleted"`
		Skipped []batchSkipJSON `json:"skipped"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Deleted != 2 || len(body.Skipped) != 1 || body.Skipped[0].ID != "c3" {
		t.Fatalf("body=%+v", body)
	}
	for _, id := range []string{"c1", "c2"} {
		if _, err := ms.GetMeta(id); err == nil {
			t.Fatalf("%s meta survived", id)
		}
		if left := ms.List(id); len(left) != 0 {
			t.Fatalf("%s messages survived: %d", id, len(left))
		}
	}
}

func TestBatchConversationsValidate(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	for _, payload := range []string{
		`{"conversation_ids":[]}`,
		`{"conversation_ids":[""]}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v0/conversations/batch/delete",
			bytes.NewBufferString(payload))
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("%s=%d", payload, rr.Code)
		}
	}
}

func TestBatchArchiveConversations(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	for _, id := range []string{"c1", "c2"} {
		if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: "admin", Source: "ui"}); err != nil {
			t.Fatal(err)
		}
	}
	post := func(archived bool) (int, []batchSkipJSON) {
		payload := `{"conversation_ids":["c1","c2","missing"],"archived":` +
			map[bool]string{true: "true", false: "false"}[archived] + `}`
		req := httptest.NewRequest(http.MethodPost, "/v0/conversations/batch/archived",
			bytes.NewBufferString(payload))
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("batch archive=%d %s", rr.Code, rr.Body.String())
		}
		var body struct {
			Updated  int             `json:"updated"`
			Archived bool            `json:"archived"`
			Skipped  []batchSkipJSON `json:"skipped"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Archived != archived {
			t.Fatalf("archived flag=%v want %v", body.Archived, archived)
		}
		return body.Updated, body.Skipped
	}

	updated, skipped := post(true)
	if updated != 2 || len(skipped) != 1 || skipped[0].ID != "missing" ||
		skipped[0].Reason != "not_found" {
		t.Fatalf("updated=%d skipped=%+v", updated, skipped)
	}
	for _, id := range []string{"c1", "c2"} {
		m, err := ms.GetMeta(id)
		if err != nil || m.ArchivedAt.IsZero() {
			t.Fatalf("%s archived=%+v err=%v", id, m, err)
		}
	}

	if updated, _ := post(false); updated != 2 {
		t.Fatalf("unarchive updated=%d", updated)
	}
	for _, id := range []string{"c1", "c2"} {
		m, _ := ms.GetMeta(id)
		if !m.ArchivedAt.IsZero() {
			t.Fatalf("%s still archived", id)
		}
	}
}
