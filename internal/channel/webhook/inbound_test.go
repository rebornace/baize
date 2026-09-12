package webhook

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhooksig"
)

type fakeRuns struct {
	mu      sync.Mutex
	created []string
	active  bool
}

func (f *fakeRuns) CreateRun(in store.CreateRunInput) (*store.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, in.ConversationID)
	return &store.Run{ID: "run-1", AgentID: in.AgentID, ConversationID: in.ConversationID}, nil
}
func (f *fakeRuns) HasActiveRun(string) (bool, error)          { return f.active, nil }
func (f *fakeRuns) WaitingHumanRun(string) (*store.Run, error) { return nil, nil }

type routeRec struct{ pattern string }

func (r *routeRec) RegisterRoute(p string, h http.Handler) { r.pattern = p }

func bootChannel(t *testing.T, m map[string]string, runs channel.RunStore, hook func(*store.Run, []llm.ContentPart)) (*Channel, http.Handler) {
	t.Helper()
	ch, err := openFromConfig("feishu", m)
	if err != nil {
		t.Fatal(err)
	}
	rec := &routeRec{}
	rt, _, _, err := ch.Bootstrap(channel.BuildDeps{
		Store:          runs,
		Meta:           conversation.NewMemoryStore(),
		DefaultAgentID: "ag-default",
		Routes:         rec,
		ResolveModel: func(sig llm.TaskSignals) (string, bool, bool) {
			if sig.HasImages {
				return "mp_vision", true, true // image turns reach a vision model
			}
			return "", true, true
		},
		AfterCreateRun: func(ctx context.Context, run *store.Run, parts []llm.ContentPart) error {
			if hook != nil {
				hook(run, parts)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.rt = rt
	if rec.pattern != "POST /v0/channels/feishu/inbound" {
		t.Fatalf("unexpected route pattern: %q", rec.pattern)
	}
	return ch, ch.inboundHandler()
}

func signedBody(t *testing.T, secret string, msg InboundMessage) ([]byte, string, string) {
	t.Helper()
	body, _ := json.Marshal(msg)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return body, ts, webhooksig.Sign(secret, ts, body)
}

func post(handler http.Handler, body []byte, ts, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v0/channels/feishu/inbound", bytes.NewReader(body))
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, sig)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func baseCfg(url string) map[string]string {
	return map[string]string{
		"source": "feishu", "account": "acc-1", "secret": "s3cr3t",
		"outbound_url": url, "assignee": "u-admin",
	}
}

func TestInboundCreatesRun(t *testing.T) {
	runs := &fakeRuns{}
	var gotParts []llm.ContentPart
	ch, handler := bootChannel(t, baseCfg("http://x/o"), runs, func(r *store.Run, p []llm.ContentPart) { gotParts = p })

	msg := InboundMessage{Event: "message", Peer: Peer{ID: "peer9"}, Text: "你好"}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	rec := post(handler, body, ts, sig)
	if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(runs.created) != 1 {
		t.Fatalf("expected 1 run, got %v", runs.created)
	}
	if want := "feishu:acc-1:peer9"; runs.created[0] != want {
		t.Fatalf("convID=%q want %q", runs.created[0], want)
	}
	if len(gotParts) != 0 { // text-only => no multimodal parts
		t.Fatalf("expected no parts for text, got %+v", gotParts)
	}
}

func TestInboundRejectsBadSignature(t *testing.T) {
	runs := &fakeRuns{}
	_, handler := bootChannel(t, baseCfg("http://x/o"), runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x"}
	body, ts, _ := signedBody(t, "wrong-secret", msg)
	rec := post(handler, body, ts, webhooksig.Sign("wrong-secret", ts, body))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if len(runs.created) != 0 {
		t.Fatal("must not create run on bad signature")
	}
}

func TestInboundIdempotencyDedupes(t *testing.T) {
	runs := &fakeRuns{}
	ch, handler := bootChannel(t, baseCfg("http://x/o"), runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x", IdempotencyKey: "k-1"}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	if r := post(handler, body, ts, sig); r.Code >= 300 {
		t.Fatalf("first: %d", r.Code)
	}
	if r := post(handler, body, ts, sig); r.Code >= 300 {
		t.Fatalf("second: %d", r.Code)
	}
	if len(runs.created) != 1 {
		t.Fatalf("idempotent key should yield 1 run, got %d", len(runs.created))
	}
}

func TestInboundIdempotencyKeyExpires(t *testing.T) {
	// Shrink the TTL so the expiry path is observable without a real wait;
	// restore the production value when the test finishes.
	oldTTL := idempotencyTTL
	idempotencyTTL = 50 * time.Millisecond
	defer func() { idempotencyTTL = oldTTL }()

	runs := &fakeRuns{}
	ch, handler := bootChannel(t, baseCfg("http://x/o"), runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x", IdempotencyKey: "k-ttl"}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)

	if r := post(handler, body, ts, sig); r.Code >= 300 {
		t.Fatalf("first: %d", r.Code)
	}
	// Immediate replay is still a duplicate within the TTL window.
	if r := post(handler, body, ts, sig); r.Code >= 300 {
		t.Fatalf("replay within TTL: %d", r.Code)
	}
	if len(runs.created) != 1 {
		t.Fatalf("want 1 run before TTL expiry, got %d", len(runs.created))
	}

	// After the TTL elapses the same key is treated as unseen again.
	time.Sleep(120 * time.Millisecond)
	if r := post(handler, body, ts, sig); r.Code >= 300 {
		t.Fatalf("replay after TTL: %d", r.Code)
	}
	if len(runs.created) != 2 {
		t.Fatalf("want 2 runs after TTL expiry, got %d", len(runs.created))
	}
}

func TestInboundAllowlistBlocksOutsider(t *testing.T) {
	runs := &fakeRuns{}
	cfg := baseCfg("http://x/o")
	cfg["allowlist"] = "allowed-peer"
	ch, handler := bootChannel(t, cfg, runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "stranger"}, Text: "x"}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	rec := post(handler, body, ts, sig)
	if rec.Code >= 300 {
		t.Fatalf("ignored messages still ack 2xx, got %d", rec.Code)
	}
	if len(runs.created) != 0 {
		t.Fatal("peer not in allowlist must not create run")
	}
}

func TestInboundInlineImageBecomesPart(t *testing.T) {
	runs := &fakeRuns{}
	var gotParts []llm.ContentPart
	cfg := baseCfg("http://x/o")
	ch, handler := bootChannel(t, cfg, runs, func(r *store.Run, p []llm.ContentPart) { gotParts = p })

	// 1x1 transparent PNG
	// 1x1 transparent PNG (valid IDAT; the brief's literal string fails Go's
	// strict PNG decoder with "too much pixel data", so this is a real
	// image/png-encodeable 1x1).
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAEUlEQVR4nGJiYGBgAAQAAP//AA8AA/6P688AAAAASUVORK5CYII=")
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "看图",
		Attachments: []Attachment{{Name: "a.png", MIME: "image/png", ContentBase64: base64.StdEncoding.EncodeToString(png)}}}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	if rec := post(handler, body, ts, sig); rec.Code >= 300 {
		t.Fatalf("status=%d", rec.Code)
	}
	sawImage := false
	for _, p := range gotParts {
		if p.Type == "image" {
			sawImage = true
		}
	}
	if !sawImage {
		t.Fatalf("expected an image part, got %+v", gotParts)
	}
}

func TestInboundIdempotencyKeyNotConsumedOnFailure(t *testing.T) {
	runs := &fakeRuns{}
	ch, handler := bootChannel(t, baseCfg("http://x/o"), runs, nil)

	// First attempt with the same idempotency_key fails: its attachment has
	// undecodable base64. The failure must not consume the key.
	bad := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x", IdempotencyKey: "retry-k",
		Attachments: []Attachment{{Name: "a.bin", MIME: "application/octet-stream", ContentBase64: "%%%not-base64%%%"}}}
	body, ts, sig := signedBody(t, ch.cfg.Secret, bad)
	if rec := post(handler, body, ts, sig); rec.Code < 400 {
		t.Fatalf("failing attempt should be 4xx/5xx, got %d", rec.Code)
	}
	if len(runs.created) != 0 {
		t.Fatalf("failed attempt must not create run, got %v", runs.created)
	}

	// Retry with the SAME key but a valid message must now succeed.
	good := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x", IdempotencyKey: "retry-k"}
	body, ts, sig = signedBody(t, ch.cfg.Secret, good)
	if rec := post(handler, body, ts, sig); rec.Code >= 300 {
		t.Fatalf("retry should succeed, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(runs.created) != 1 {
		t.Fatalf("retry after failure should create 1 run, got %v", runs.created)
	}
}

func TestInboundAttachmentURLBlocksLoopback(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
	}))
	defer srv.Close()

	runs := &fakeRuns{}
	ch, handler := bootChannel(t, baseCfg("http://x/o"), runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x",
		Attachments: []Attachment{{Name: "a.bin", MIME: "application/octet-stream", URL: srv.URL + "/x"}}}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	rec := post(handler, body, ts, sig)
	if rec.Code < 400 {
		t.Fatalf("loopback attachment URL must be rejected, got %d", rec.Code)
	}
	if len(runs.created) != 0 {
		t.Fatalf("blocked SSRF fetch must not create run, got %v", runs.created)
	}
	if hit {
		t.Fatal("loopback target must never be reached")
	}
}

func TestResolveAttachmentsRejectsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	ch, err := openFromConfig("feishu", baseCfg("http://x/o"))
	if err != nil {
		t.Fatal(err)
	}
	// Use a plain client (no SSRF dial guard) to isolate the status-code check:
	// the guarded handler client would block 127.0.0.1 before any response.
	_, err = ch.resolveAttachments(context.Background(), http.DefaultClient,
		[]Attachment{{Name: "a.bin", MIME: "application/octet-stream", URL: srv.URL + "/missing"}})
	if err == nil {
		t.Fatal("404 fetch must return an error, not be accepted as attachment content")
	}
}

func TestResolveAttachmentsFetchesURLBytes(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAAEUlEQVR4nGJiYGBgAAQAAP//AA8AA/6P688AAAAASUVORK5CYII=")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(png)
	}))
	defer srv.Close()

	ch, err := openFromConfig("feishu", baseCfg("http://x/o"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := ch.resolveAttachments(context.Background(), http.DefaultClient,
		[]Attachment{{Name: "a.png", MIME: "image/png", URL: srv.URL + "/a.png"}})
	if err != nil {
		t.Fatalf("200 fetch should succeed: %v", err)
	}
	if len(files) != 1 || !bytes.Equal(files[0].Data, png) {
		t.Fatalf("expected fetched bytes to match attachment, got %+v", files)
	}
}

func TestResolveAttachmentsRejectsNonHTTPScheme(t *testing.T) {
	ch, err := openFromConfig("feishu", baseCfg("http://x/o"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"file:///etc/passwd", "gopher://x:70/1", "ftp://x/a", "://nohost", "http://"} {
		_, err := ch.resolveAttachments(context.Background(), http.DefaultClient,
			[]Attachment{{Name: "a.bin", MIME: "application/octet-stream", URL: bad}})
		if err == nil {
			t.Fatalf("attachment URL %q must be rejected", bad)
		}
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",        // loopback
		"::1",              // loopback v6
		"10.0.0.5",         // private
		"172.16.0.1",       // private
		"192.168.1.1",      // private
		"169.254.169.254",  // cloud metadata / link-local
		"fe80::1",          // link-local v6
		"0.0.0.0",          // unspecified
		"::",               // unspecified v6
		"100.64.0.1",       // CGNAT
		"100.127.255.254",  // CGNAT upper bound
		"198.18.0.1",       // benchmarking
		"198.19.255.255",   // benchmarking
		"224.0.0.1",        // multicast
		"::ffff:127.0.0.1", // IPv4-mapped loopback
	}
	for _, s := range blocked {
		if !isBlockedIP(net.ParseIP(s)) {
			t.Errorf("expected %s to be blocked", s)
		}
	}
	allowed := []string{
		"8.8.8.8",     // public
		"1.1.1.1",     // public
		"100.63.0.1",  // just below CGNAT
		"100.128.0.1", // just above CGNAT
		"203.0.113.9", // TEST-NET (doc range; public-unicast-ish, not internal)
	}
	for _, s := range allowed {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("expected %s to be allowed", s)
		}
	}
}
