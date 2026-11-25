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

func TestMessageRetentionSettingsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	get := httptest.NewRequest(http.MethodGet, "/v0/settings/message-retention", nil)
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
	put := httptest.NewRequest(http.MethodPut, "/v0/settings/message-retention",
		bytes.NewBufferString(`{"days":30}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if got := srv.messageRetentionDays(); got != 30 {
		t.Fatalf("days=%d", got)
	}
	for _, payload := range []string{`{"days":-1}`, `{"days":3651}`} {
		bad := httptest.NewRequest(http.MethodPut, "/v0/settings/message-retention",
			bytes.NewBufferString(payload))
		bad.Header.Set("Authorization", "Bearer adm")
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, bad)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("bad %s=%d", payload, rr.Code)
		}
	}
}

func TestConversationMessageRetentionAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	if err := srv.Store.UpsertSetting(messageRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	get := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/message-retention", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Days      int `json:"days"`
		Effective int `json:"effective_days"`
		Global    int `json:"global_days"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Days != -1 || body.Effective != 30 || body.Global != 30 {
		t.Fatalf("default=%+v", body)
	}
	put := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/message-retention",
		bytes.NewBufferString(`{"days":7}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Days != 7 || body.Effective != 7 || body.Global != 30 {
		t.Fatalf("override=%+v", body)
	}
	bad := httptest.NewRequest(http.MethodPut, "/v0/conversations/c1/message-retention",
		bytes.NewBufferString(`{"days":-2}`))
	bad.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, bad)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad=%d", rr.Code)
	}
}

func TestMessageRetentionConversationOverride(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	msgs := conversation.NewMemoryStore()
	srv.Messages = msgs
	if err := st.UpsertSetting(messageRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(messageRetentionConvPrefix+"c-forever", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(messageRetentionConvPrefix+"c-short", []byte("5")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	seed := func(conv string) {
		for i := 0; i < 3; i++ {
			if _, err := msgs.Append(conv, conversation.Message{
				Role: conversation.RoleUser, Content: "old", CreatedAt: old.Add(time.Duration(i) * time.Minute),
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := msgs.Append(conv, conversation.Message{
			Role: conversation.RoleAssistant, Content: "recent", CreatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seed("c-global")
	seed("c-forever")
	seed("c-short")

	n, err := srv.cleanupExpiredMessages()
	if err != nil || n != 4 {
		t.Fatalf("cleanup n=%d err=%v", n, err)
	}
	if left := msgs.List("c-global"); len(left) != 2 {
		t.Fatalf("c-global left=%d", len(left))
	}
	if left := msgs.List("c-forever"); len(left) != 4 {
		t.Fatalf("c-forever left=%d", len(left))
	}
	if left := msgs.List("c-short"); len(left) != 2 {
		t.Fatalf("c-short left=%d", len(left))
	}

	if err := st.UpsertSetting(messageRetentionSettingKey, []byte("0")); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredMessages(); err != nil || n != 0 {
		t.Fatalf("global-off cleanup n=%d err=%v", n, err)
	}
	if left := msgs.List("c-forever"); len(left) != 4 {
		t.Fatalf("c-forever damaged with global off: %d", len(left))
	}
}

func TestMessageRetentionCleanup(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	msgs := conversation.NewMemoryStore()
	srv.Messages = msgs
	if err := st.UpsertSetting(messageRetentionSettingKey, []byte("30")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -40)
	for i := 0; i < 3; i++ {
		if _, err := msgs.Append("c-old", conversation.Message{
			Role: conversation.RoleUser, Content: "old", CreatedAt: old.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := msgs.Append("c-old", conversation.Message{
		Role: conversation.RoleAssistant, Content: "recent", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := msgs.Append("c-active", conversation.Message{
			Role: conversation.RoleUser, Content: "old", CreatedAt: old.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-active",
	}); err != nil {
		t.Fatal(err)
	}
	n, err := srv.cleanupExpiredMessages()
	if err != nil || n != 2 {
		t.Fatalf("cleanup n=%d err=%v", n, err)
	}
	if left := msgs.List("c-old"); len(left) != 2 {
		t.Fatalf("c-old left=%d", len(left))
	}
	if left := msgs.List("c-active"); len(left) != 3 {
		t.Fatalf("active conversation trimmed: %d", len(left))
	}
	if err := st.UpsertSetting(messageRetentionSettingKey, []byte("0")); err != nil {
		t.Fatal(err)
	}
	if n, err := srv.cleanupExpiredMessages(); err != nil || n != 0 {
		t.Fatalf("off cleanup n=%d err=%v", n, err)
	}
}
