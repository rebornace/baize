package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

// postOutbound signs and posts an outbound payload to the adapter.
func postOutbound(t *testing.T, a *Adapter, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/outbound", testSecret, body))
	return rec
}

func TestOutboundRejectsBadSignature(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/outbound", bytes.NewReader([]byte("{}"))))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned outbound = %d", rec.Code)
	}
}

func TestOutboundTextSent(t *testing.T) {
	fake := weixinlink.NewFake()
	a := newTestAdapter(t, fake, "")
	a.setCredentials("bot@im.bot", "tok")
	rec := postOutbound(t, a, map[string]any{
		"kind": "assistant", "peer": map[string]any{"id": "peer@im.wechat"},
		"text": "【助手】你好", "context_token": "ctx-9",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("outbound = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fake.Sent) != 1 || fake.Sent[0].Text != "【助手】你好" || fake.Sent[0].ContextToken != "ctx-9" {
		t.Fatalf("Sent=%+v", fake.Sent)
	}
}

func TestOutboundImageAndFile(t *testing.T) {
	fake := weixinlink.NewFake()
	a := newTestAdapter(t, fake, "")
	a.setCredentials("bot@im.bot", "tok")

	png := []byte("PNGBYTES")
	pdf := []byte("PDFBYTES")
	rec := postOutbound(t, a, map[string]any{
		"kind": "assistant", "peer": map[string]any{"id": "peer@im.wechat"},
		"text": "【助手】看图",
		"media": []map[string]any{
			{"name": "a.png", "mime": "image/png", "content_base64": base64.StdEncoding.EncodeToString(png)},
			{"name": "b.pdf", "mime": "application/pdf", "content_base64": base64.StdEncoding.EncodeToString(pdf)},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("outbound = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(fake.SentImages) != 1 || string(fake.SentImages[0].Data) != "PNGBYTES" || fake.SentImages[0].FileName != "a.png" {
		t.Fatalf("images=%+v", fake.SentImages)
	}
	if len(fake.SentFiles) != 1 || string(fake.SentFiles[0].Data) != "PDFBYTES" || fake.SentFiles[0].FileName != "b.pdf" {
		t.Fatalf("files=%+v", fake.SentFiles)
	}
}

func TestOutboundWithoutCredentials(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := postOutbound(t, a, map[string]any{"peer": map[string]any{"id": "p"}, "text": "x"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("outbound without creds = %d want 409", rec.Code)
	}
}
