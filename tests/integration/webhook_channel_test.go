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

// outboundCapture is a fake adapter: it records baize->adapter posts,
// including the raw body and signature headers so the test can prove the
// outbound hop is itself signed over the exact bytes delivered.
type outboundCapture struct {
	mu        sync.Mutex
	got       []map[string]any
	rawBodies [][]byte
	headers   []http.Header
}

func (o *outboundCapture) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		o.mu.Lock()
		o.got = append(o.got, m)
		o.rawBodies = append(o.rawBodies, body)
		o.headers = append(o.headers, r.Header.Clone())
		o.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
}

func (o *outboundCapture) snapshot() ([]map[string]any, [][]byte, []http.Header) {
	o.mu.Lock()
	defer o.mu.Unlock()
	got := append([]map[string]any(nil), o.got...)
	bodies := append([][]byte(nil), o.rawBodies...)
	headers := append([]http.Header(nil), o.headers...)
	return got, bodies, headers
}

func (o *outboundCapture) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.got)
}

func TestWebhookChannelBidirectionalE2E(t *testing.T) {
	adapter := &outboundCapture{}
	adapterSrv := httptest.NewServer(adapter.handler())
	defer adapterSrv.Close()

	const secret = "e2e-secret"
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "wh-agent"
	cfg.Agent.System = "你是测试助手。"
	cfg.Channels = []config.ChannelConfig{{
		Name:    "feishu",
		Type:    "webhook",
		Enabled: true,
		Config: map[string]string{
			"source":       "feishu",
			"account":      "feishu-bot",
			"secret":       secret,
			"outbound_url": adapterSrv.URL,
			"assignee":     "u-admin",
			"agent_id":     "wh-agent",
		},
	}}

	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	// Adapter -> baize: signed inbound message.
	inbound := map[string]any{
		"event":   "message",
		"peer":    map[string]any{"id": "peer-42", "name": "Alice"},
		"text":    "你好",
		"account": "feishu-bot",
	}
	body, _ := json.Marshal(inbound)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(secret, ts, body)

	req, _ := http.NewRequest("POST", runtimeURL+"/v0/channels/feishu/inbound", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("inbound post: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inbound status=%d body=%s", resp.StatusCode, raw)
	}

	// Wait for the engine to produce an assistant reply pushed to the adapter.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.count() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	got, bodies, headers := adapter.snapshot()
	if len(got) == 0 {
		t.Fatal("adapter never received an outbound message")
	}
	first := got[0]
	if peer, _ := first["peer"].(map[string]any); peer["id"] != "peer-42" {
		t.Fatalf("outbound peer mismatch: %+v", first["peer"])
	}
	if first["account"] != "feishu-bot" {
		t.Fatalf("outbound account mismatch: %+v", first["account"])
	}
	text, ok := first["text"].(string)
	if !ok || text == "" {
		t.Fatalf("outbound missing text: %+v", first)
	}
	if conv, _ := first["conversation_id"].(string); conv == "" {
		t.Fatalf("outbound missing conversation_id: %+v", first)
	}
	// The engine assistant reply must be tagged kind=assistant and carry run_id
	// in both body and header (design §5.2 message classification / run link).
	if kind, _ := first["kind"].(string); kind != "assistant" {
		t.Fatalf("outbound kind=%q want assistant", kind)
	}
	runID, _ := first["run_id"].(string)
	if runID == "" {
		t.Fatalf("outbound missing run_id: %+v", first)
	}
	outTS := headers[0].Get("X-Baize-Channel-Timestamp")
	outSig := headers[0].Get("X-Baize-Channel-Signature")
	if outTS == "" || outSig == "" {
		t.Fatalf("outbound missing signature headers: %+v", headers[0])
	}
	if hdrRun := headers[0].Get("X-Baize-Run-Id"); hdrRun != runID {
		t.Fatalf("X-Baize-Run-Id header=%q want %q", hdrRun, runID)
	}
	if err := webhooksig.Verify(secret, outTS, bodies[0], outSig, time.Now(), 5*time.Minute); err != nil {
		t.Fatalf("outbound signature verification failed: %v", err)
	}
}

func TestWebhookChannelRejectsBadSignatureE2E(t *testing.T) {
	adapter := &outboundCapture{}
	adapterSrv := httptest.NewServer(adapter.handler())
	defer adapterSrv.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "wh-agent"
	cfg.Channels = []config.ChannelConfig{{
		Name: "feishu", Type: "webhook", Enabled: true,
		Config: map[string]string{"source": "feishu", "account": "b", "secret": "right",
			"outbound_url": adapterSrv.URL, "assignee": "u", "agent_id": "wh-agent"},
	}}
	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	body, _ := json.Marshal(map[string]any{"event": "message", "peer": map[string]any{"id": "p"}, "text": "x"})
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign("wrong", ts, body) // signed with wrong secret
	req, _ := http.NewRequest("POST", runtimeURL+"/v0/channels/feishu/inbound", bytes.NewReader(body))
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	// Give any (buggy) async outbound a chance to arrive before asserting none.
	time.Sleep(500 * time.Millisecond)
	if adapter.count() != 0 {
		t.Fatal("no outbound should happen on rejected inbound")
	}
}
