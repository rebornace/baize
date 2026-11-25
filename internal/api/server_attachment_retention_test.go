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

type retentionFakeMedia struct {
	deletedConv   string
	deletedObject string
}

func (f *retentionFakeMedia) OpenMedia(context.Context, string, string) ([]byte, string, bool, error) {
	return nil, "", false, nil
}

func (f *retentionFakeMedia) DeleteMedia(_ context.Context, conv, object string) error {
	f.deletedConv, f.deletedObject = conv, object
	return nil
}

func TestAttachmentRetentionCleanup(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	media := &retentionFakeMedia{}
	srv.ChannelMedia = media
	st := srv.Store
	now := time.Now().UTC()
	old := store.ConversationAttachment{
		ConversationID: "c1", Object: "old.pdf", Filename: "old.pdf",
		MediaType: "application/pdf", CreatedAt: now.AddDate(0, 0, -40),
	}
	fresh := store.ConversationAttachment{
		ConversationID: "c1", Object: "new.pdf", Filename: "new.pdf",
		MediaType: "application/pdf", CreatedAt: now,
	}
	for _, a := range []store.ConversationAttachment{old, fresh} {
		if err := st.AddConversationAttachment(a); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.UpsertSetting(attachmentRetentionKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	n, err := srv.cleanupExpiredAttachments(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("cleanup n=%d err=%v", n, err)
	}
	if media.deletedObject != "old.pdf" {
		t.Fatalf("deleted object=%q", media.deletedObject)
	}
	left, err := st.ListConversationAttachments("c1", 10)
	if err != nil || len(left) != 1 || left[0].Object != "new.pdf" {
		t.Fatalf("left=%+v err=%v", left, err)
	}
}

func TestAttachmentRetentionPinsActiveRun(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.ChannelMedia = &retentionFakeMedia{}
	st := srv.Store
	if err := st.AddConversationAttachment(store.ConversationAttachment{
		ConversationID: "c1", Object: "old.pdf", Filename: "old.pdf",
		MediaType: "application/pdf", CreatedAt: time.Now().UTC().AddDate(0, 0, -40),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(attachmentRetentionKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{AgentID: "a1", Input: "x", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredAttachments(context.Background()); err != nil || n != 0 {
		t.Fatalf("active cleanup n=%d err=%v", n, err)
	}
	if left, _ := st.ListConversationAttachments("c1", 10); len(left) != 1 {
		t.Fatalf("attachment removed while run active: %+v", left)
	}
	if err := st.UpdateRun(runRec.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredAttachments(context.Background()); err != nil || n != 1 {
		t.Fatalf("settled cleanup n=%d err=%v", n, err)
	}
	if left, _ := st.ListConversationAttachments("c1", 10); len(left) != 0 {
		t.Fatalf("left=%+v", left)
	}
}

func TestAttachmentRetentionOffKeepsEverything(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.ChannelMedia = &retentionFakeMedia{}
	st := srv.Store
	if err := st.AddConversationAttachment(store.ConversationAttachment{
		ConversationID: "c1", Object: "old.pdf", Filename: "old.pdf",
		CreatedAt: time.Now().UTC().AddDate(0, 0, -400),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(attachmentRetentionKey, []byte("0")); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredAttachments(context.Background()); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if left, _ := st.ListConversationAttachments("c1", 10); len(left) != 1 {
		t.Fatalf("left=%+v", left)
	}
}

func TestAttachmentRetentionSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/attachment-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d", rr.Code)
	}
	var body struct {
		Days int `json:"days"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.Days != 0 {
		t.Fatalf("default=%+v err=%v", body, err)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/attachment-retention",
		bytes.NewBufferString(`{"days":30}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if got := srv.attachmentRetentionDays(); got != 30 {
		t.Fatalf("days=%d", got)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/attachment-retention",
		bytes.NewBufferString(`{"days":3651}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}
