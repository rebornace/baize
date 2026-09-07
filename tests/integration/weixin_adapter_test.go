package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/webhooksig"
)

// fakeWeixinAdapter emulates the weixin-adapter process: it captures baize
// outbound posts and serves the /admin management plane. Every endpoint
// verifies the baize->adapter HMAC signature (outbound and admin hops share
// the instance secret).
type fakeWeixinAdapter struct {
	mu       sync.Mutex
	outbound []map[string]any
	hasCreds bool
	polling  bool
}

func (f *fakeWeixinAdapter) handler(secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Verify baize->adapter signature on every endpoint.
		if err := webhooksig.Verify(secret, r.Header.Get("X-Baize-Channel-Timestamp"), body,
			r.Header.Get("X-Baize-Channel-Signature"), time.Now(), 5*time.Minute); err != nil {
			http.Error(w, "bad sig", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/outbound":
			var m map[string]any
			_ = json.Unmarshal(body, &m)
			f.mu.Lock()
			f.outbound = append(f.outbound, m)
			f.mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/admin/status":
			f.mu.Lock()
			_ = json.NewEncoder(w).Encode(map[string]any{"has_credentials": f.hasCreds, "polling": f.polling})
			f.mu.Unlock()
		case r.URL.Path == "/admin/login/start":
			_ = json.NewEncoder(w).Encode(map[string]string{"ticket": "tk", "qr_url": "qr"})
		case r.URL.Path == "/admin/login/status":
			f.mu.Lock()
			f.hasCreds, f.polling = true, true
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		case r.URL.Path == "/admin/logout":
			f.mu.Lock()
			f.hasCreds, f.polling = false, false
			f.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
		case r.URL.Path == "/admin/start" || r.URL.Path == "/admin/stop":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	})
}

func (f *fakeWeixinAdapter) outboundCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.outbound)
}

func TestWeixinAdapterE2E(t *testing.T) {
	const secret = "wx-e2e-secret"
	adapter := &fakeWeixinAdapter{}
	srv := httptest.NewServer(adapter.handler(secret))
	defer srv.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "wx-agent"
	cfg.ControlPlane.AdminToken = "adm"
	cfg.Channels = []config.ChannelConfig{{
		Name: "weixin", Type: "webhook", Enabled: true,
		Config: map[string]string{
			"source":       "weixin",
			"secret":       secret,
			"outbound_url": srv.URL + "/outbound",
			"admin_url":    srv.URL,
			"assignee":     "channel:weixin",
			"agent_id":     "wx-agent",
		},
	}}
	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	// Management plane: login start via generic route (admin token).
	admReq, _ := http.NewRequest(http.MethodPost, runtimeURL+"/v0/settings/channels/weixin/login/start", nil)
	admReq.Header.Set("Authorization", "Bearer adm")
	resp, err := http.DefaultClient.Do(admReq)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("login/start=%d body=%s", resp.StatusCode, b)
	}
	_ = resp.Body.Close()

	// Status before adapter "login": has_credentials=false -> login_required.
	getStatus := func() (bool, string) {
		r, _ := http.NewRequest(http.MethodGet, runtimeURL+"/v0/settings/channels/weixin", nil)
		r.Header.Set("Authorization", "Bearer adm")
		rr, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		var st map[string]any
		_ = json.NewDecoder(rr.Body).Decode(&st)
		_ = rr.Body.Close()
		running, _ := st["running"].(bool)
		reason, _ := st["reason"].(string)
		return running, reason
	}
	if running, reason := getStatus(); running || reason != "login_required" {
		t.Fatalf("pre-login status running=%v reason=%q", running, reason)
	}

	// Adapter->baize inbound with dynamic account (post-login ilink_bot_id).
	inbound := map[string]any{
		"event":   "message",
		"account": "bot@im.bot",
		"peer":    map[string]any{"id": "peer@im.wechat"},
		"text":    "你好微信",
	}
	body, _ := json.Marshal(inbound)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, _ := http.NewRequest(http.MethodPost, runtimeURL+"/v0/channels/weixin/inbound", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", webhooksig.Sign(secret, ts, body))
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp2.Body)
		t.Fatalf("inbound=%d body=%s", resp2.StatusCode, b)
	}
	_ = resp2.Body.Close()

	// Assistant reply flows back to the adapter with dynamic account + prefix.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && adapter.outboundCount() == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if adapter.outboundCount() == 0 {
		t.Fatal("adapter never received outbound")
	}
	adapter.mu.Lock()
	out := adapter.outbound[0]
	adapter.mu.Unlock()
	if out["account"] != "bot@im.bot" {
		t.Fatalf("outbound account=%v want bot@im.bot", out["account"])
	}
	if conv, _ := out["conversation_id"].(string); conv != "weixin:bot@im.bot:peer@im.wechat" {
		t.Fatalf("conv=%v", out["conversation_id"])
	}
	if text, _ := out["text"].(string); !bytes.Contains([]byte(text), []byte("【助手】")) {
		t.Fatalf("assistant prefix missing: %q", text)
	}
	if kind, _ := out["kind"].(string); kind != "assistant" {
		t.Fatalf("kind=%v", kind)
	}
}
