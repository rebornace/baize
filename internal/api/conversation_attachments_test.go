package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/workspace"
)

func TestConversationAttachmentsListAndBulkDelete(t *testing.T) {
	srv, _, _, h, _ := attachmentsServer(t, false)
	ws, ok := srv.Workspace.(*workspace.Service)
	if !ok {
		t.Fatal("workspace not configured on test server")
	}
	putAgent(t, h, "a1")

	rr := postRun(t, h, map[string]any{
		"agent_id":        "a1",
		"input":           "look at these",
		"conversation_id": "c1",
		"attachments": []map[string]any{
			{
				"filename":       "note.txt",
				"media_type":     "text/plain",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("first")),
			},
			{
				"filename":       "note.txt",
				"media_type":     "text/plain",
				"content_base64": base64.StdEncoding.EncodeToString([]byte("second")),
			},
			{
				"filename":       "dot.png",
				"media_type":     "image/png",
				"content_base64": tinyPNGBase64(t),
			},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("post run=%d %s", rr.Code, rr.Body.String())
	}
	pollRunStatus(t, h, postRunID(t, rr), store.StatusSucceeded)

	list := func() []struct {
		Object   string `json:"object"`
		Filename string `json:"filename"`
		URL      string `json:"url"`
	} {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/attachments", nil)
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("list=%d %s", res.Code, res.Body.String())
		}
		var body struct {
			Items []struct {
				Object   string `json:"object"`
				Filename string `json:"filename"`
				URL      string `json:"url"`
			} `json:"items"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Items
	}

	items := list()
	if len(items) != 3 {
		t.Fatalf("items=%+v", items)
	}
	notes := 0
	var firstNote string
	for _, it := range items {
		if it.Filename == "note.txt" {
			notes++
			if firstNote == "" {
				firstNote = it.Object
			}
		}
		if !strings.HasPrefix(it.URL, "/v0/channels/media/c1/") {
			t.Fatalf("url=%q", it.URL)
		}
	}
	if notes != 2 {
		t.Fatalf("note attachments=%d items=%+v", notes, items)
	}
	uploads, err := ws.ListFiles(context.Background(), "c1", "uploads")
	if err != nil || len(uploads) != 3 {
		t.Fatalf("workspace uploads=%+v err=%v", uploads, err)
	}

	bulkDelete := func(objects ...string) (int, int) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"objects": objects})
		req := httptest.NewRequest(http.MethodPost, "/v0/conversations/c1/attachments/delete",
			bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("bulk delete=%d %s", res.Code, res.Body.String())
		}
		var out struct {
			Deleted int `json:"deleted"`
			Failed  int `json:"failed"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Deleted, out.Failed
	}

	deleted, failed := bulkDelete(firstNote)
	if deleted != 1 || failed != 0 {
		t.Fatalf("deleted=%d failed=%d", deleted, failed)
	}
	remaining := list()
	if len(remaining) != 2 {
		t.Fatalf("after delete=%+v", remaining)
	}
	uploads, _ = ws.ListFiles(context.Background(), "c1", "uploads")
	if len(uploads) != 2 {
		t.Fatalf("workspace uploads after delete=%+v", uploads)
	}
	var noteBodies []string
	for _, e := range uploads {
		body, rerr := ws.ReadFile(context.Background(), "c1", "uploads/"+e.Name)
		if rerr == nil && (body == "first" || body == "second") {
			noteBodies = append(noteBodies, body)
		}
	}
	if len(noteBodies) != 1 {
		t.Fatalf("workspace note copies=%v, want exactly one remaining", noteBodies)
	}

	var rest []string
	for _, it := range remaining {
		rest = append(rest, it.Object)
	}
	if d, f := bulkDelete(rest...); d != 2 || f != 0 {
		t.Fatalf("bulk remaining deleted=%d failed=%d", d, f)
	}
	if len(list()) != 0 {
		t.Fatal("attachments remain after bulk delete")
	}
	uploads, _ = ws.ListFiles(context.Background(), "c1", "uploads")
	if len(uploads) != 0 {
		t.Fatalf("workspace uploads remain: %+v", uploads)
	}
}

func TestConversationAttachmentsRequireObjects(t *testing.T) {
	_, _, _, h, _ := attachmentsServer(t, false)
	req := httptest.NewRequest(http.MethodPost, "/v0/conversations/c1/attachments/delete",
		bytes.NewReader([]byte(`{"objects":[]}`)))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", res.Code)
	}
}
