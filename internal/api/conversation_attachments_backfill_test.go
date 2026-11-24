package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

func TestConversationAttachmentsBackfillFromMessages(t *testing.T) {
	srv, _, _, h, _ := attachmentsServer(t, false)
	created := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	if _, err := srv.Messages.Append("c1", conversation.Message{
		Role:      conversation.RoleUser,
		CreatedAt: created,
		Content: "旧附件\n" +
			"![图片](/v0/channels/media/c1/a.png?name=old.png)\n" +
			"[file:doc.pdf](/v0/channels/media/c1/b.pdf)",
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/attachments", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("list=%d %s", res.Code, res.Body.String())
	}
	var body struct {
		Items []struct {
			Object    string `json:"object"`
			Filename  string `json:"filename"`
			MediaType string `json:"media_type"`
			CreatedAt string `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items=%+v", body.Items)
	}
	byName := map[string]struct {
		MediaType string
		CreatedAt string
	}{}
	for _, it := range body.Items {
		byName[it.Filename] = struct {
			MediaType string
			CreatedAt string
		}{it.MediaType, it.CreatedAt}
	}
	if got := byName["old.png"]; got.MediaType != "image/png" {
		t.Fatalf("old.png=%+v items=%+v", got, body.Items)
	}
	if got := byName["doc.pdf"]; got.MediaType != "application/pdf" {
		t.Fatalf("doc.pdf=%+v", got)
	}
	if byName["old.png"].CreatedAt == "" || byName["doc.pdf"].CreatedAt == "" {
		t.Fatalf("created_at missing: %+v", byName)
	}
}
