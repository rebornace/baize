package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func ownerTestServer(t *testing.T) (*Server, conversation.Store, http.Handler) {
	t.Helper()
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.Messages = msgs
	srv.AdminToken = "adm"
	srv.Operators = []controlplane.Operator{
		{ID: "alice", Token: "ta"},
		{ID: "bob", Token: "tb"},
	}
	st.UpsertAgent(store.Agent{ID: "a1", System: "sys"})
	srv.DefaultAgentID = "a1"
	return srv, msgs, srv.Handler()
}

func seedOwnedConv(t *testing.T, msgs conversation.Store, id, owner, content string) {
	t.Helper()
	ms, ok := msgs.(interface {
		EnsureMeta(conversation.Meta) error
	})
	if !ok {
		t.Fatal("message store must support EnsureMeta")
	}
	now := time.Now().UTC()
	if err := ms.EnsureMeta(conversation.Meta{ID: id, OwnerID: owner, Source: "ui", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := msgs.Append(id, conversation.Message{Role: conversation.RoleUser, Content: content}); err != nil {
		t.Fatal(err)
	}
}

func TestListConversationsFiltersByOwner(t *testing.T) {
	_, msgs, h := ownerTestServer(t)
	seedOwnedConv(t, msgs, "c-alice", "alice", "alice hello")
	seedOwnedConv(t, msgs, "c-bob", "bob", "bob hello")

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	req.Header.Set("Authorization", "Bearer ta")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("alice list=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) != 1 || body.Conversations[0].ID != "c-alice" {
		t.Fatalf("alice should see only own: %+v", body.Conversations)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/conversations?scope=all", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("admin list=%d %s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) != 2 {
		t.Fatalf("admin scope=all want 2 got %+v", body.Conversations)
	}
}

func TestCrossOwnerMessagesForbidden(t *testing.T) {
	_, msgs, h := ownerTestServer(t)
	seedOwnedConv(t, msgs, "c-alice", "alice", "secret")

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c-alice/messages", nil)
	req.Header.Set("Authorization", "Bearer tb")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("bob read alice messages want 403 got %d %s", rr.Code, rr.Body.String())
	}
}

func TestCreateRunEnsuresMetaOwner(t *testing.T) {
	srv, msgs, h := ownerTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(
		`{"agent_id":"a1","input":"hi","conversation_id":"c-new"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer ta")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("create run=%d %s", rr.Code, rr.Body.String())
	}
	ms, ok := msgs.(interface {
		GetMeta(string) (conversation.Meta, bool)
	})
	if !ok {
		t.Fatal("missing GetMeta")
	}
	meta, found := ms.GetMeta("c-new")
	if !found || meta.OwnerID != "alice" || meta.Source != "ui" {
		t.Fatalf("EnsureMeta missing/wrong: found=%v %+v", found, meta)
	}
	_ = srv
}

func TestGateOffListUnfilteredAndLocalDevOwner(t *testing.T) {
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.Messages = msgs
	st.UpsertAgent(store.Agent{ID: "a1", System: "sys"})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(
		`{"agent_id":"a1","input":"hi","conversation_id":"c-dev"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("run=%d %s", rr.Code, rr.Body.String())
	}
	meta, ok := msgs.GetMeta("c-dev")
	if !ok || meta.OwnerID != "local-dev" {
		t.Fatalf("gate-off owner=%+v ok=%v", meta, ok)
	}

	seedOwnedConv(t, msgs, "c-other", "alice", "x")
	req = httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("list=%d", rr.Code)
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) < 2 {
		t.Fatalf("gate off must not filter: %+v", body.Conversations)
	}
}
