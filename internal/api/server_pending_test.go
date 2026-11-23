package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func pendingTestServer(t *testing.T) (*Server, *store.Memory) {
	t.Helper()
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.AdminToken = "adm-token"
	srv.Operators = []controlplane.Operator{
		{ID: "alice", Token: "alice-token"},
		{ID: "bob", Token: "bob-token"},
	}
	srv.Messages = msgs
	srv.DefaultAgentID = "a"
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	return srv, st
}

func seedWaitingRun(t *testing.T, srv *Server, st store.Store, convID, owner, text string, payload *store.HITLPayload) string {
	t.Helper()
	ms, ok := srv.Messages.(conversation.MetaStore)
	if !ok {
		t.Fatal("messages store must implement MetaStore")
	}
	if err := ms.EnsureMeta(conversation.Meta{
		ID:        convID,
		OwnerID:   owner,
		Source:    "ui",
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ensure meta: %v", err)
	}
	if _, err := srv.Messages.Append(convID, conversation.Message{
		Role:    conversation.RoleUser,
		Content: text,
	}); err != nil {
		t.Fatalf("append message: %v", err)
	}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID:        "a",
		Input:          text,
		ConversationID: convID,
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := st.UpdateRun(r.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if payload != nil {
		if err := st.SetHITL(r.ID, payload); err != nil {
			t.Fatalf("set hitl: %v", err)
		}
	}
	return r.ID
}

func getPending(t *testing.T, srv *Server, token string) (int, []pendingApprovalJSON) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/hitl/pending", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET pending: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Count   int                   `json:"count"`
		Pending []pendingApprovalJSON `json:"pending"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Count != len(body.Pending) {
		t.Fatalf("count=%d len=%d", body.Count, len(body.Pending))
	}
	return body.Count, body.Pending
}

func TestPendingApprovalsAdminSeesAllWithDetails(t *testing.T) {
	srv, st := pendingTestServer(t)
	since := time.Date(2026, 11, 21, 3, 4, 5, 0, time.UTC)
	seedWaitingRun(t, srv, st, "c-alice", "alice", "排查 VPN 连接", &store.HITLPayload{
		Kind:         store.HITLKindApproval,
		ToolName:     "send_email",
		Prompt:       "发送邮件给客户？",
		Arguments:    map[string]any{"to": "customer@example.com"},
		WaitingSince: since,
		Workflow:     &store.WorkflowState{Skill: "ticket-triage", StepID: "notify"},
	})
	seedWaitingRun(t, srv, st, "c-bob", "bob", "登录后台导出报表", &store.HITLPayload{
		Kind:   store.HITLKindLogin,
		Prompt: "请完成登录",
	})

	count, items := getPending(t, srv, "adm-token")
	if count != 2 {
		t.Fatalf("count=%d want 2 (%+v)", count, items)
	}
	var alice *pendingApprovalJSON
	for i := range items {
		if items[i].ConversationID == "c-alice" {
			alice = &items[i]
		}
	}
	if alice == nil {
		t.Fatalf("missing alice item: %+v", items)
	}
	if alice.ConversationTitle != "排查 VPN 连接" {
		t.Fatalf("title=%q", alice.ConversationTitle)
	}
	if alice.Kind != store.HITLKindApproval || alice.ToolName != "send_email" || alice.Prompt != "发送邮件给客户？" {
		t.Fatalf("alice item=%+v", alice)
	}
	if alice.WaitingSince != since.Format(time.RFC3339) {
		t.Fatalf("waiting_since=%q", alice.WaitingSince)
	}
	if alice.Arguments["to"] != "customer@example.com" {
		t.Fatalf("arguments=%+v", alice.Arguments)
	}
	if alice.WorkflowSkill != "ticket-triage" || alice.WorkflowStep != "notify" {
		t.Fatalf("workflow=%+v", alice)
	}
}

func TestPendingApprovalsOperatorSeesOwnConversations(t *testing.T) {
	srv, st := pendingTestServer(t)
	seedWaitingRun(t, srv, st, "c-alice", "alice", "alice 的会话", &store.HITLPayload{Kind: store.HITLKindApproval})
	seedWaitingRun(t, srv, st, "c-bob", "bob", "bob 的会话", &store.HITLPayload{Kind: store.HITLKindApproval})

	count, items := getPending(t, srv, "alice-token")
	if count != 1 || len(items) != 1 || items[0].ConversationID != "c-alice" {
		t.Fatalf("alice sees %d items: %+v", count, items)
	}
}

func TestPendingApprovalsEmpty(t *testing.T) {
	srv, _ := pendingTestServer(t)
	count, items := getPending(t, srv, "adm-token")
	if count != 0 || len(items) != 0 {
		t.Fatalf("count=%d items=%+v", count, items)
	}
}
