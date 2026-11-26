package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestWorkspaceRetentionSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-retention",
		bytes.NewBufferString(`{"workspace_id":"default","message_days":7,"run_days":0}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/workspace-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		GlobalMessageDays int                      `json:"global_message_days"`
		GlobalRunDays     int                      `json:"global_run_days"`
		Items             []workspaceRetentionJSON `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].MessageDays != 7 || body.Items[0].RunDays != 0 {
		t.Fatalf("items=%+v", body.Items)
	}
	if body.Items[0].EffectiveMessageDays != 7 || body.Items[0].EffectiveRunDays != 0 {
		t.Fatalf("effective=%+v", body.Items[0])
	}

	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-retention",
		bytes.NewBufferString(`{"workspace_id":"default","message_days":-2}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestMessageRetentionWorkspaceOverride(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-ws", WorkspaceID: "ws1", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i, when := range []time.Time{
		now.AddDate(0, 0, -4), now.AddDate(0, 0, -3), now.AddDate(0, 0, -2), now,
	} {
		if _, err := ms.Append("c-ws", conversation.Message{
			Role: conversation.RoleUser, Content: string(rune('a' + i)), CreatedAt: when,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := srv.Store.UpsertSetting(messageRetentionSettingKey, []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := srv.Store.UpsertSetting(messageRetentionWorkspacePrefix+"ws1", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredMessages(); err != nil || n != 0 {
		t.Fatalf("workspace 0 should keep everything: n=%d err=%v", n, err)
	}
	if err := srv.Store.UpsertSetting(messageRetentionWorkspacePrefix+"ws1", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredMessages(); err != nil || n != 2 {
		t.Fatalf("deleted=%d err=%v want 2", n, err)
	}
}

func TestRunRetentionWorkspacePrecedence(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-ws", WorkspaceID: "ws1", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -40)
	mk := func(conv string) *store.Run {
		t.Helper()
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: "a1", Input: "x", ConversationID: conv, CreatedAt: old,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		return r
	}
	wsRun := mk("c-ws")
	globalRun := mk("c-other")
	if err := st.UpsertSetting(runRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionAgentPrefix+"a1", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionWorkspacePrefix+"ws1", []byte("7")); err != nil {
		t.Fatal(err)
	}

	if _, err := srv.cleanupExpiredRuns(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetRun(wsRun.ID); err == nil {
		t.Fatal("workspace 7 should win over agent 0")
	}
	if _, err := st.GetRun(globalRun.ID); err != nil {
		t.Fatalf("agent 0 should keep the unassigned run: %v", err)
	}
}

func TestRetentionPreviewAndRunNow(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	if err := ms.EnsureMeta(conversation.Meta{ID: "c1", WorkspaceID: "ws1", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, when := range []time.Time{now.AddDate(0, 0, -5), now.AddDate(0, 0, -4), now} {
		if _, err := ms.Append("c1", conversation.Message{
			Role: conversation.RoleUser, Content: "m", CreatedAt: when,
		}); err != nil {
			t.Fatal(err)
		}
	}
	old := now.AddDate(0, 0, -40)
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c1", CreatedAt: old,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(messageRetentionSettingKey, []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(runRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}

	preview := httptest.NewRequest(http.MethodGet, "/v0/settings/retention/preview", nil)
	preview.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, preview)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview=%d %s", rr.Code, rr.Body.String())
	}
	var body retentionPreview
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Messages.Total != 1 || body.Runs.Total != 1 {
		t.Fatalf("preview=%+v", body)
	}

	run := httptest.NewRequest(http.MethodPost, "/v0/settings/retention/run", nil)
	run.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, run)
	if rr.Code != http.StatusOK {
		t.Fatalf("run=%d %s", rr.Code, rr.Body.String())
	}
	var result struct {
		MessagesDeleted int `json:"messages_deleted"`
		RunsDeleted     int `json:"runs_deleted"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.MessagesDeleted != 1 || result.RunsDeleted != 1 {
		t.Fatalf("result=%+v", result)
	}
}
