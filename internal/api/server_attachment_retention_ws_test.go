package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestWorkspaceAttachmentRetentionSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	lab, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Store.UpsertSetting(attachmentRetentionKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/workspace-attachment-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		GlobalDays int                                `json:"global_days"`
		Items      []workspaceAttachmentRetentionJSON `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.GlobalDays != 30 {
		t.Fatalf("global=%d", body.GlobalDays)
	}
	found := false
	for _, item := range body.Items {
		if item.WorkspaceID != lab.ID {
			continue
		}
		found = true
		if item.Name != "Lab" || item.Days != -1 || item.EffectiveDays != 30 {
			t.Fatalf("item=%+v", item)
		}
	}
	if !found {
		t.Fatalf("lab missing from %+v", body.Items)
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-attachment-retention",
		bytes.NewBufferString(`{"workspace_id":"`+lab.ID+`","days":7}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if got := srv.attachmentRetentionWorkspaceOverride(lab.ID); got != 7 {
		t.Fatalf("override=%d", got)
	}
	if got := srv.attachmentRetentionDaysFor("c-no-meta", 30); got != 30 {
		t.Fatalf("default workspace effective=%d, want 30", got)
	}

	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-attachment-retention",
		bytes.NewBufferString(`{"workspace_id":"`+lab.ID+`","days":-2}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad days=%d", rr.Code)
	}

	missing := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-attachment-retention",
		bytes.NewBufferString(`{"workspace_id":"ws_nope","days":7}`))
	missing.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, missing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing workspace=%d", rr.Code)
	}
}

func TestWorkspaceAttachmentRetentionCleanup(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	lab, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-lab", WorkspaceID: lab.ID, Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-lab-keep", WorkspaceID: lab.ID, Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	srv.ChannelMedia = &retentionFakeMedia{}
	st := srv.Store
	now := time.Now().UTC()
	add := func(conv, object string, age time.Duration) {
		t.Helper()
		if err := st.AddConversationAttachment(store.ConversationAttachment{
			ConversationID: conv, Object: object, Filename: object,
			CreatedAt: now.Add(-age),
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("c-lab", "lab-old.pdf", 10*24*time.Hour)
	add("c-lab", "lab-new.pdf", 2*24*time.Hour)
	add("c-lab-keep", "keep.pdf", 10*24*time.Hour)
	add("c-default", "default-old.pdf", 10*24*time.Hour)
	if err := st.UpsertSetting(attachmentRetentionKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(attachmentRetentionWorkspacePrefix+lab.ID, []byte("7")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(attachmentRetentionConvPrefix+"c-lab-keep", []byte("0")); err != nil {
		t.Fatal(err)
	}

	if _, err := srv.cleanupExpiredAttachments(context.Background()); err != nil {
		t.Fatal(err)
	}
	left := func(conv string) int {
		t.Helper()
		list, err := st.ListConversationAttachments(conv, 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(list)
	}
	if left("c-lab") != 1 {
		t.Fatalf("c-lab left=%d, want 1", left("c-lab"))
	}
	if left("c-lab-keep") != 1 {
		t.Fatalf("conversation override must win: c-lab-keep left=%d", left("c-lab-keep"))
	}
	if left("c-default") != 1 {
		t.Fatalf("default workspace must stay on global 30: left=%d", left("c-default"))
	}
}
