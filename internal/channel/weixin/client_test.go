package weixin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFake_LoginPendingThenSuccess(t *testing.T) {
	f := NewFake()
	ctx := context.Background()

	ticket, qrURL, err := f.GetQR(ctx)
	if err != nil {
		t.Fatalf("GetQR: %v", err)
	}
	if ticket == "" || qrURL == "" {
		t.Fatalf("GetQR returned empty ticket=%q qrURL=%q", ticket, qrURL)
	}

	status, accountID, token, err := f.PollLogin(ctx, ticket)
	if err != nil {
		t.Fatalf("PollLogin #1: %v", err)
	}
	if status != LoginStatusPending {
		t.Fatalf("PollLogin #1 status=%q want %q", status, LoginStatusPending)
	}
	if accountID != "" || token != "" {
		t.Fatalf("pending should not return creds: account=%q token=%q", accountID, token)
	}

	status, accountID, token, err = f.PollLogin(ctx, ticket)
	if err != nil {
		t.Fatalf("PollLogin #2: %v", err)
	}
	if status != LoginStatusSuccess {
		t.Fatalf("PollLogin #2 status=%q want %q", status, LoginStatusSuccess)
	}
	if accountID == "" || token == "" {
		t.Fatalf("success missing creds: account=%q token=%q", accountID, token)
	}
}

func TestFake_PollLoginExpired(t *testing.T) {
	f := NewFake()
	f.LoginSequence = []string{LoginStatusExpired}
	ctx := context.Background()

	ticket, _, err := f.GetQR(ctx)
	if err != nil {
		t.Fatalf("GetQR: %v", err)
	}
	status, _, _, err := f.PollLogin(ctx, ticket)
	if err != nil {
		t.Fatalf("PollLogin: %v", err)
	}
	if status != LoginStatusExpired {
		t.Fatalf("status=%q want %q", status, LoginStatusExpired)
	}
}

func TestFake_GetUpdatesText(t *testing.T) {
	f := NewFake()
	f.Updates = []Update{{
		PeerID:       "user@im.wechat",
		Text:         "你好",
		ContextToken: "ctx-1",
	}}
	f.NextCursor = "cursor-2"

	updates, next, err := f.GetUpdates(context.Background(), "tok", "")
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if next != "cursor-2" {
		t.Fatalf("nextCursor=%q want cursor-2", next)
	}
	if len(updates) != 1 || updates[0].Text != "你好" || updates[0].PeerID != "user@im.wechat" {
		t.Fatalf("updates=%+v", updates)
	}
}

func TestFake_SendMessageAndDownloadMedia(t *testing.T) {
	f := NewFake()
	f.MediaBytes = []byte("fake-media")

	msg := OutboundMessage{
		ToUserID:     "user@im.wechat",
		Text:         "reply",
		ContextToken: "ctx-1",
	}
	if err := f.SendMessage(context.Background(), "tok", msg); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if len(f.Sent) != 1 || f.Sent[0].Text != "reply" {
		t.Fatalf("Sent=%+v", f.Sent)
	}

	data, err := f.DownloadMedia(context.Background(), "tok", MediaRef{URL: "http://example/m"})
	if err != nil {
		t.Fatalf("DownloadMedia: %v", err)
	}
	if string(data) != "fake-media" {
		t.Fatalf("media=%q", data)
	}
}

func TestClient_GetQRAndPollLogin(t *testing.T) {
	var pollN atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/ilink/bot/get_bot_qrcode"):
			if r.URL.Query().Get("bot_type") != "3" {
				t.Errorf("bot_type=%q", r.URL.Query().Get("bot_type"))
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"qrcode":             "qrc_test",
				"qrcode_img_content": "https://weixin.qq.com/x/test",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/ilink/bot/get_qrcode_status":
			if r.URL.Query().Get("qrcode") != "qrc_test" {
				t.Errorf("qrcode=%q", r.URL.Query().Get("qrcode"))
			}
			if r.Header.Get("iLink-App-ClientVersion") != "1" {
				t.Errorf("missing client version header")
			}
			n := pollN.Add(1)
			if n == 1 {
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "wait"})
				return
			}
			if n == 2 {
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "scaned"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "confirmed",
				"bot_token":     "ilinkbot_tok",
				"ilink_bot_id":  "bot@im.bot",
				"ilink_user_id": "user@im.wechat",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	ctx := context.Background()

	ticket, qrURL, err := c.GetQR(ctx)
	if err != nil {
		t.Fatalf("GetQR: %v", err)
	}
	if ticket != "qrc_test" || qrURL != "https://weixin.qq.com/x/test" {
		t.Fatalf("ticket=%q qrURL=%q", ticket, qrURL)
	}

	status, _, _, err := c.PollLogin(ctx, ticket)
	if err != nil || status != LoginStatusPending {
		t.Fatalf("poll1 status=%q err=%v", status, err)
	}
	status, _, _, err = c.PollLogin(ctx, ticket)
	if err != nil || status != LoginStatusPending {
		t.Fatalf("poll2 (scaned) status=%q err=%v", status, err)
	}
	status, accountID, token, err := c.PollLogin(ctx, ticket)
	if err != nil {
		t.Fatalf("poll3: %v", err)
	}
	if status != LoginStatusSuccess || token != "ilinkbot_tok" {
		t.Fatalf("status=%q token=%q", status, token)
	}
	if accountID != "bot@im.bot" && accountID != "user@im.wechat" {
		t.Fatalf("accountID=%q", accountID)
	}
}

func TestClient_PollLoginExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "expired"})
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	status, _, _, err := c.PollLogin(context.Background(), "qrc")
	if err != nil {
		t.Fatalf("PollLogin: %v", err)
	}
	if status != LoginStatusExpired {
		t.Fatalf("status=%q", status)
	}
}

func TestClient_GetUpdatesAndSendMessage(t *testing.T) {
	var sawAuth, sawAuthType, sawUIN bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer tok-1" {
			sawAuth = true
		}
		if r.Header.Get("AuthorizationType") == "ilink_bot_token" {
			sawAuthType = true
		}
		if r.Header.Get("X-WECHAT-UIN") != "" {
			sawUIN = true
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type=%q", r.Header.Get("Content-Type"))
		}

		switch r.URL.Path {
		case "/ilink/bot/getupdates":
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("body: %v", err)
			}
			if req["get_updates_buf"] != "cur-1" {
				t.Errorf("cursor=%v", req["get_updates_buf"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ret": 0,
				"msgs": []map[string]any{{
					"from_user_id":  "peer@im.wechat",
					"to_user_id":    "bot@im.bot",
					"context_token": "ctx-abc",
					"message_type":  1,
					"item_list": []map[string]any{{
						"type": 1,
						"text_item": map[string]string{
							"text": "hello-ilink",
						},
					}},
				}},
				"get_updates_buf": "cur-2",
			})
		case "/ilink/bot/sendmessage":
			body, _ := io.ReadAll(r.Body)
			var req map[string]any
			if err := json.Unmarshal(body, &req); err != nil {
				t.Errorf("send body: %v", err)
			}
			msg, _ := req["msg"].(map[string]any)
			if msg["to_user_id"] != "peer@im.wechat" || msg["context_token"] != "ctx-abc" {
				t.Errorf("msg=%v", msg)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	c.LongPollTimeout = 2 * time.Second

	updates, next, err := c.GetUpdates(context.Background(), "tok-1", "cur-1")
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if next != "cur-2" || len(updates) != 1 || updates[0].Text != "hello-ilink" {
		t.Fatalf("updates=%+v next=%q", updates, next)
	}
	if updates[0].PeerID != "peer@im.wechat" || updates[0].ContextToken != "ctx-abc" {
		t.Fatalf("update=%+v", updates[0])
	}

	err = c.SendMessage(context.Background(), "tok-1", OutboundMessage{
		ToUserID:     "peer@im.wechat",
		Text:         "pong",
		ContextToken: "ctx-abc",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !sawAuth || !sawAuthType || !sawUIN {
		t.Fatalf("headers auth=%v authType=%v uin=%v", sawAuth, sawAuthType, sawUIN)
	}
}

func TestClient_DownloadMediaRawBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("raw-cdn-bytes"))
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	data, err := c.DownloadMedia(context.Background(), "tok", MediaRef{URL: srv.URL + "/file"})
	if err != nil {
		t.Fatalf("DownloadMedia: %v", err)
	}
	if string(data) != "raw-cdn-bytes" {
		t.Fatalf("data=%q", data)
	}
}

func TestClient_GetUpdatesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ret":     -14,
			"errcode": -14,
			"errmsg":  "session timeout",
		})
	}))
	t.Cleanup(srv.Close)

	c := NewClient(srv.URL, srv.Client())
	_, _, err := c.GetUpdates(context.Background(), "tok", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "-14") && !strings.Contains(err.Error(), "session") {
		t.Fatalf("err=%v", err)
	}
}

func TestNewClient_DefaultBase(t *testing.T) {
	c := NewClient("", nil)
	if c.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL=%q", c.BaseURL)
	}
}
