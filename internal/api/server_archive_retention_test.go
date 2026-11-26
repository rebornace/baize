package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestArchiveRetentionCleanupAndPreview(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	old := time.Now().UTC().AddDate(0, 0, -40)
	for _, m := range []conversation.Meta{
		{ID: "c-old", Source: "ui", ArchivedAt: old},
		{ID: "c-new", Source: "ui", ArchivedAt: time.Now().UTC()},
		{ID: "c-active", Source: "ui", ArchivedAt: old},
	} {
		if err := ms.EnsureMeta(m); err != nil {
			t.Fatal(err)
		}
		if _, err := ms.Append(m.ID, conversation.Message{
			Role: conversation.RoleUser, Content: "x", CreatedAt: old,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.Store.UpsertSetting(archiveRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/retention/preview", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("preview=%d %s", rr.Code, rr.Body.String())
	}
	var body retentionPreview
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Archived.Total != 1 || body.Archived.Days != 30 {
		t.Fatalf("archived preview=%+v", body.Archived)
	}

	n, err := srv.cleanupExpiredArchived()
	if err != nil || n != 1 {
		t.Fatalf("deleted=%d err=%v", n, err)
	}
	if _, err := ms.GetMeta("c-old"); err == nil {
		t.Fatal("c-old should be deleted")
	}
	if len(ms.List("c-old")) != 0 {
		t.Fatal("c-old messages should be deleted")
	}
	if _, err := ms.GetMeta("c-new"); err != nil {
		t.Fatalf("c-new should remain: %v", err)
	}
	if _, err := ms.GetMeta("c-active"); err != nil {
		t.Fatalf("active conversation should remain: %v", err)
	}
}
