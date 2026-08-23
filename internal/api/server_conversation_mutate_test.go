package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func conversationMutateServer(t *testing.T) (*Server, conversation.Store, http.Handler) {
	t.Helper()
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Messages = msgs
	srv.DefaultAgentID = "default-agent"
	st.UpsertAgent(store.Agent{ID: "default-agent", System: "sys"})
	return srv, msgs, srv.Handler()
}

func TestRollbackTruncatesMessages(t *testing.T) {
	_, msgs, h := conversationMutateServer(t)
	_, _ = msgs.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "u1"})
	m2, _ := msgs.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "a1"})
	_, _ = msgs.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "u2"})

	req := httptest.NewRequest(
		http.MethodPost,
		"/v0/conversations/conv1/messages/"+m2.ID+"/rollback",
		jsonBodyAPI(t, map[string]any{}),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["deleted_count"] != float64(2) {
		t.Fatalf("deleted_count=%v want 2", body["deleted_count"])
	}
	rawMsgs, ok := body["messages"].([]any)
	if !ok || len(rawMsgs) != 1 {
		t.Fatalf("messages=%v want 1", body["messages"])
	}
}

func TestForkCopiesPrefix(t *testing.T) {
	_, msgs, h := conversationMutateServer(t)
	m1, _ := msgs.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "u1"})
	_, _ = msgs.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "a1"})

	req := httptest.NewRequest(
		http.MethodPost,
		"/v0/conversations/conv1/fork",
		jsonBodyAPI(t, map[string]any{"through_message_id": m1.ID}),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	newID, _ := body["conversation_id"].(string)
	if newID == "" || newID == "conv1" {
		t.Fatalf("conversation_id=%q", newID)
	}
	if body["copied_count"] != float64(1) {
		t.Fatalf("copied_count=%v", body["copied_count"])
	}
}

func TestRollbackBusyConversation(t *testing.T) {
	srv, msgs, h := conversationMutateServer(t)
	st := srv.Store
	m1, _ := msgs.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "u1"})
	_, _ = st.CreateRun(store.CreateRunInput{
		AgentID:        "default-agent",
		Input:          "x",
		ConversationID: "conv1",
	})

	req := httptest.NewRequest(
		http.MethodPost,
		"/v0/conversations/conv1/messages/"+m1.ID+"/rollback",
		jsonBodyAPI(t, map[string]any{}),
	)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d want 409 body=%s", rr.Code, rr.Body.String())
	}
}
