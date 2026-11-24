package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestConversationAttachmentRetentionSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	if err := srv.Store.UpsertSetting(attachmentRetentionKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	get := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/attachment-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Days          int `json:"days"`
		EffectiveDays int `json:"effective_days"`
		GlobalDays    int `json:"global_days"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Days != -1 || body.EffectiveDays != 30 || body.GlobalDays != 30 {
		t.Fatalf("default=%+v", body)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/attachment-retention",
		bytes.NewBufferString(`{"days":7}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if got := srv.attachmentRetentionDaysFor("c1", 30); got != 7 {
		t.Fatalf("effective=%d", got)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/attachment-retention",
		bytes.NewBufferString(`{"days":-2}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestConversationRetentionOverrideCleanup(t *testing.T) {
	srv := gateServer(t, "op", "adm")
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
	add("c-keep", "keep.pdf", 40*24*time.Hour)
	add("c-global", "old.pdf", 40*24*time.Hour)
	add("c-forever", "forever.pdf", 400*24*time.Hour)
	add("c-short", "short.pdf", 2*24*time.Hour)
	if err := st.UpsertSetting(attachmentRetentionKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	set := func(conv string, days int) {
		t.Helper()
		raw, _ := json.Marshal(days)
		if err := st.UpsertSetting(attachmentRetentionConvPrefix+conv, raw); err != nil {
			t.Fatal(err)
		}
	}
	set("c-keep", 60)
	set("c-forever", 0)
	set("c-short", 1)

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
	if left("c-keep") != 1 || left("c-forever") != 1 {
		t.Fatalf("keep=%d forever=%d", left("c-keep"), left("c-forever"))
	}
	if left("c-global") != 0 || left("c-short") != 0 {
		t.Fatalf("global=%d short=%d", left("c-global"), left("c-short"))
	}
}
