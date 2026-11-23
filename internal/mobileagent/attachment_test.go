package mobileagent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testAttachments = `[{"id":"a1","name":"photo.jpg","mime":"image/jpeg","size":1234,"local_ref":"/data/app/files/attachments/a1"}]`

func TestAttachmentMetadataSnapshotRoundTrip(t *testing.T) {
	var sawAttachments int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"attachments"`) {
			atomic.StoreInt32(&sawAttachments, 1)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sseChunk("ok")+"data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := newTestAgent()
	if err := a.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.StartStreamWithAttachments("hi", testAttachments); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("start=%s", got)
	}
	if last := waitStream(t, a); last["status"] != "done" {
		t.Fatalf("final=%+v", last)
	}
	if atomic.LoadInt32(&sawAttachments) != 0 {
		t.Fatal("attachments metadata must not be sent to the model")
	}

	snap := a.ExportSession()
	if !strings.Contains(snap, `"attachments"`) || !strings.Contains(snap, "photo.jpg") {
		t.Fatalf("snapshot missing attachments: %s", snap)
	}
	restored := newTestAgent()
	if got := restored.ImportSession(snap); !strings.Contains(got, `"done"`) {
		t.Fatalf("import=%s", got)
	}
	atts := restored.SessionAttachments()
	if !strings.Contains(atts, `"name":"photo.jpg"`) || !strings.Contains(atts, `"local_ref":"/data/app/files/attachments/a1"`) {
		t.Fatalf("restored attachments=%s", atts)
	}
	if !strings.Contains(atts, `"message_index":0`) {
		t.Fatalf("message index lost: %s", atts)
	}
}

func TestAttachmentValidation(t *testing.T) {
	a := newTestAgent()
	tooMany := make([]string, maxAttachmentsPerMessage+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf(`{"name":"f%d","local_ref":"/tmp/f%d"}`, i, i)
	}
	cases := []string{
		`{}`,
		`[{"local_ref":"/tmp/a"}]`,
		`[{"name":"a"}]`,
		`[{"name":"a","local_ref":"/tmp/a","extra":1}]`,
		`[` + strings.Join(tooMany, ",") + `]`,
		`[{"name":"a","local_ref":"/tmp/a","size":-1}]`,
	}
	for _, raw := range cases {
		got := a.SendWithAttachments("hi", raw)
		if !strings.Contains(got, `"status":"error"`) {
			t.Fatalf("raw=%s got=%s", raw, got)
		}
	}
	if a.MessageCount() != 0 {
		t.Fatalf("invalid attachments appended messages: %d", a.MessageCount())
	}
}

func TestRetryPreservesAttachments(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if atomic.AddInt32(&calls, 1) == 1 {
			_, _ = io.WriteString(w, sseChunk("partial")+"data: {bad}\n\n")
			return
		}
		_, _ = io.WriteString(w, sseChunk("ok")+"data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := newTestAgent()
	if err := a.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.StartStreamWithAttachments("hi", testAttachments); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("start=%s", got)
	}
	if last := waitStream(t, a); last["status"] != "error" {
		t.Fatalf("first final=%+v", last)
	}
	if atts := a.SessionAttachments(); !strings.Contains(atts, "photo.jpg") {
		t.Fatalf("attachments lost after error: %s", atts)
	}
	if got := a.RetryStream(); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("retry=%s", got)
	}
	if last := waitStream(t, a); last["status"] != "done" {
		t.Fatalf("retry final=%+v", last)
	}
	if a.MessageCount() != 2 {
		t.Fatalf("message count=%d want user+assistant", a.MessageCount())
	}
	if atts := a.SessionAttachments(); !strings.Contains(atts, "photo.jpg") || !strings.Contains(atts, `"message_index":0`) {
		t.Fatalf("attachments after retry=%s", atts)
	}
}

func TestImportSessionRejectsInvalidAttachment(t *testing.T) {
	a := newTestAgent()
	raw := map[string]any{
		"version":       sessionSnapshotVersion,
		"message_count": 1,
		"messages": []any{
			map[string]any{
				"role": "user", "content": "hi",
				"attachments": []any{map[string]any{"name": "a", "local_ref": ""}},
			},
		},
	}
	b, _ := json.Marshal(raw)
	if got := a.ImportSession(string(b)); !strings.Contains(got, `"status":"error"`) {
		t.Fatalf("invalid snapshot accepted: %s", got)
	}
}
