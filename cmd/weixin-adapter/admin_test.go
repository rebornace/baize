package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const testSecret = "adm-secret"

// signedReq builds a baize->adapter admin request with a valid HMAC signature.
func signedReq(t *testing.T, method, target, secret string, body []byte) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(secret, ts, body)
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	return req
}

func newTestAdapter(t *testing.T, fake *weixinlink.Fake, baizeURL string) *Adapter {
	t.Helper()
	dir := t.TempDir()
	a := &Adapter{
		ilink:           fake,
		baizeInboundURL: baizeURL,
		secret:          testSecret,
		credsDir:        dir,
		emptyWait:       5 * time.Millisecond,
		httpClient:      &http.Client{Timeout: 2 * time.Second},
	}
	return a
}

func TestAdminRequiresSignature(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", nil)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned admin = %d, want 401", rec.Code)
	}
}

func TestAdminLoginStartStatusAndAutoPoll(t *testing.T) {
	// baize inbound receiver: capture the first forwarded message.
	var (
		mu      sync.Mutex
		gotBody map[string]any
		gotCh   = make(chan struct{}, 1)
	)
	baize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		// Verify adapter->baize signature too.
		if err := webhooksig.Verify(testSecret, r.Header.Get("X-Baize-Channel-Timestamp"), b,
			r.Header.Get("X-Baize-Channel-Signature"), time.Now(), 300*time.Second); err != nil {
			t.Errorf("baize inbound bad signature: %v", err)
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		gotBody = m
		mu.Unlock()
		select {
		case gotCh <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(baize.Close)

	fake := weixinlink.NewFake()
	fake.Updates = []weixinlink.Update{{
		PeerID:       "peer@im.wechat",
		Text:         "你好适配器",
		ContextToken: "ctx-1",
	}}
	a := newTestAdapter(t, fake, baize.URL)

	// login/start
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/admin/login/start", testSecret, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("login/start = %d body=%s", rec.Code, rec.Body.String())
	}
	var ls struct {
		Ticket string `json:"ticket"`
		QRURL  string `json:"qr_url"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ls)
	if ls.Ticket == "" || ls.QRURL == "" {
		t.Fatalf("login start resp=%+v", ls)
	}

	// login/status: pending then success.
	callStatus := func() string {
		r := httptest.NewRecorder()
		a.routes().ServeHTTP(r, signedReq(t, http.MethodGet, "/admin/login/status?ticket="+ls.Ticket, testSecret, nil))
		var st struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &st)
		return st.Status
	}
	if s := callStatus(); s != weixinlink.LoginStatusPending {
		t.Fatalf("poll1=%s", s)
	}
	if s := callStatus(); s != weixinlink.LoginStatusSuccess {
		t.Fatalf("poll2=%s", s)
	}

	// success -> credentials persisted + polling started.
	if !a.isPolling() {
		t.Fatal("expected polling after login success")
	}
	if _, err := os.Stat(filepath.Join(a.credsDir, "creds.json")); err != nil {
		t.Fatalf("creds.json not persisted: %v", err)
	}

	// status endpoint reflects credentials + polling + account.
	rec2 := httptest.NewRecorder()
	a.routes().ServeHTTP(rec2, signedReq(t, http.MethodGet, "/admin/status", testSecret, nil))
	var st struct {
		HasCredentials bool   `json:"has_credentials"`
		Polling        bool   `json:"polling"`
		AccountID      string `json:"account_id"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &st)
	if !st.HasCredentials || !st.Polling || st.AccountID == "" {
		t.Fatalf("status=%+v", st)
	}

	// Inbound text message was forwarded to baize with account/peer/text.
	select {
	case <-gotCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound forward")
	}
	mu.Lock()
	m := gotBody
	mu.Unlock()
	if m["event"] != "message" || m["text"] != "你好适配器" {
		t.Fatalf("forwarded body=%v", m)
	}
	if m["account"] != fake.AccountID {
		t.Fatalf("account=%v want %s", m["account"], fake.AccountID)
	}
	peer, _ := m["peer"].(map[string]any)
	if peer["id"] != "peer@im.wechat" || m["context_token"] != "ctx-1" {
		t.Fatalf("peer/ctx=%v", m)
	}

	// stop -> polling false; start -> true.
	a.routes().ServeHTTP(httptest.NewRecorder(), signedReq(t, http.MethodPost, "/admin/stop", testSecret, nil))
	if a.isPolling() {
		t.Fatal("still polling after stop")
	}
	a.routes().ServeHTTP(httptest.NewRecorder(), signedReq(t, http.MethodPost, "/admin/start", testSecret, nil))
	if !a.isPolling() {
		t.Fatal("not polling after start")
	}

	// logout -> credentials cleared + polling stopped + creds file removed.
	a.routes().ServeHTTP(httptest.NewRecorder(), signedReq(t, http.MethodPost, "/admin/logout", testSecret, nil))
	if a.isPolling() {
		t.Fatal("polling after logout")
	}
	if a.hasCredentials() {
		t.Fatal("credentials present after logout")
	}
	if _, err := os.Stat(filepath.Join(a.credsDir, "creds.json")); !os.IsNotExist(err) {
		t.Fatalf("creds.json should be removed, err=%v", err)
	}
}

func TestAdminShutdownRequestsExit(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	shutdown := make(chan struct{}, 1)
	a.requestShutdown = func() { shutdown <- struct{}{} }

	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/admin/shutdown", testSecret, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("shutdown status=%d body=%s", rec.Code, rec.Body.String())
	}
	if a.isPolling() {
		t.Fatal("polling should be stopped on shutdown")
	}
	select {
	case <-shutdown:
	case <-time.After(2 * time.Second):
		t.Fatal("requestShutdown not invoked after /admin/shutdown")
	}
}

func TestAdminStartWithoutCredentials(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/admin/start", testSecret, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("start without creds = %d, want 409", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "login") == false {
		t.Fatalf("body should mention login required: %s", rec.Body.String())
	}
}

// signedReqAt builds an admin request signed with an explicit unix timestamp
// (used to construct stale-timestamp negative cases).
func signedReqAt(t *testing.T, method, target, secret string, body []byte, unixTS int64) *http.Request {
	t.Helper()
	ts := strconv.FormatInt(unixTS, 10)
	sig := webhooksig.Sign(secret, ts, body)
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	return req
}

func TestAdminRejectsWrongSecret(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/admin/logout", "not-the-secret", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong secret = %d, want 401", rec.Code)
	}
}

func TestAdminRejectsTamperedBody(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	// Sign a legitimate body, then send a DIFFERENT body with the same
	// signature: the guard must reject the mismatch before any state change.
	signedBody := []byte(`{"ok":true}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(testSecret, ts, signedBody)
	tampered := []byte(`{"ok":false}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/logout", bytes.NewReader(tampered))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered body = %d, want 401", rec.Code)
	}
}

func TestAdminRejectsStaleTimestamp(t *testing.T) {
	a := newTestAdapter(t, weixinlink.NewFake(), "")
	rec := httptest.NewRecorder()
	stale := time.Now().Add(-1 * time.Hour).Unix()
	a.routes().ServeHTTP(rec, signedReqAt(t, http.MethodPost, "/admin/logout", testSecret, nil, stale))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("stale timestamp = %d, want 401", rec.Code)
	}
}

var idemKeyRe = regexp.MustCompile(`^wx-[0-9a-f]{8}-\d+$`)

func TestInboundIdempotencyKeyHasProcessPrefix(t *testing.T) {
	var (
		mu     sync.Mutex
		gotKey string
		gotCh  = make(chan struct{}, 1)
	)
	baize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		if k, ok := m["idempotency_key"].(string); ok {
			gotKey = k
		}
		mu.Unlock()
		select {
		case gotCh <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(baize.Close)

	fake := weixinlink.NewFake()
	fake.Updates = []weixinlink.Update{{PeerID: "p@im.wechat", Text: "hi", ContextToken: "c"}}
	a := newTestAdapter(t, fake, baize.URL)

	// Drive a successful login so polling starts and the update is forwarded.
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, signedReq(t, http.MethodPost, "/admin/login/start", testSecret, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("login/start = %d body=%s", rec.Code, rec.Body.String())
	}
	var ls struct {
		Ticket string `json:"ticket"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &ls)
	for range 2 {
		r := httptest.NewRecorder()
		a.routes().ServeHTTP(r, signedReq(t, http.MethodGet, "/admin/login/status?ticket="+ls.Ticket, testSecret, nil))
	}
	t.Cleanup(a.stopPolling)

	select {
	case <-gotCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for inbound forward")
	}
	mu.Lock()
	key := gotKey
	mu.Unlock()
	if !idemKeyRe.MatchString(key) {
		t.Fatalf("idempotency_key=%q does not match %s", key, idemKeyRe.String())
	}
}

func TestInboundMediaForwardedAsAttachment(t *testing.T) {
	var gotBody map[string]any
	gotCh := make(chan struct{}, 1)
	baize := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		select {
		case gotCh <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(baize.Close)

	fake := weixinlink.NewFake()
	fake.MediaBytes = []byte("IMAGE-BYTES")
	fake.Updates = []weixinlink.Update{{
		PeerID: "peer@im.wechat",
		Text:   "看图",
		Media:  []weixinlink.MediaRef{{FileName: "pic.png", MIME: "image/png"}},
	}}
	a := newTestAdapter(t, fake, baize.URL)
	a.setCredentials(fake.AccountID, fake.Token)
	if err := a.startPolling(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.stopPolling)

	select {
	case <-gotCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
	atts, _ := gotBody["attachments"].([]any)
	if len(atts) != 1 {
		t.Fatalf("attachments=%v", gotBody["attachments"])
	}
	att := atts[0].(map[string]any)
	if att["name"] != "pic.png" {
		t.Fatalf("att name=%v", att["name"])
	}
	dec, err := base64.StdEncoding.DecodeString(att["content_base64"].(string))
	if err != nil || string(dec) != "IMAGE-BYTES" {
		t.Fatalf("att bytes=%v err=%v", att["content_base64"], err)
	}
}
