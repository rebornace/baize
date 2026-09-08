package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

func testCfg(url string) *instanceConfig {
	return &instanceConfig{Name: "t", Source: "feishu", Account: "acc", Secret: "sec", OutboundSecret: "sec", OutboundURL: url, Assignee: "a"}
}

func TestOutboundPostSignsAndDelivers(t *testing.T) {
	var got OutboundMessage
	var gotSig, gotTS, gotProto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		gotSig = r.Header.Get(HeaderSignature)
		gotTS = r.Header.Get(HeaderTimestamp)
		gotProto = r.Header.Get(HeaderProtocol)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newSender(testCfg(srv.URL))
	err := s.post(context.Background(), OutboundMessage{Kind: "assistant", Account: "acc", Peer: Peer{ID: "u1"}, Text: "hi", RunID: "r1"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if got.Text != "hi" || got.Kind != "assistant" || got.Peer.ID != "u1" || got.Account != "acc" {
		t.Fatalf("bad body: %+v", got)
	}
	if gotProto != ProtocolVersion {
		t.Fatalf("bad protocol header: %q", gotProto)
	}
	if err := webhooksig.Verify("sec", gotTS, mustMarshal(t, got), gotSig, time.Now(), 300*time.Second); err != nil {
		t.Fatalf("signature verify: %v", err)
	}
}

func TestOutboundRetriesOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	s := newSender(testCfg(srv.URL))
	if err := s.post(context.Background(), OutboundMessage{Kind: "assistant", Account: "acc", Peer: Peer{ID: "u"}}); err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestOutboundNoRetryOn4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	s := newSender(testCfg(srv.URL))
	if err := s.post(context.Background(), OutboundMessage{Kind: "assistant", Account: "acc", Peer: Peer{ID: "u"}}); err == nil {
		t.Fatal("expected error on 4xx")
	}
	if calls != 1 {
		t.Fatalf("4xx must not retry, got %d calls", calls)
	}
}

// TestOutboundUsesSecretAfterResolution guards the regression where the sender
// held a by-value copy of the config from openFromConfig, so a secret finalized
// later in Bootstrap (resolveSecret reusing persisted secret.key) was not used
// for signing and every outbound call 401'd while admin calls succeeded.
func TestOutboundUsesSecretAfterResolution(t *testing.T) {
	var gotSig, gotTS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotSig = r.Header.Get(HeaderSignature)
		gotTS = r.Header.Get(HeaderTimestamp)
		_ = webhooksig.Verify("persisted-secret", gotTS, body, gotSig, time.Now(), 300*time.Second)
		var msg OutboundMessage
		_ = json.Unmarshal(body, &msg)
		// Echo whether the signature verifies against the PERSISTED secret.
		if err := webhooksig.Verify("persisted-secret", gotTS, body, gotSig, time.Now(), 300*time.Second); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := testCfg(srv.URL)
	s := newSender(cfg)
	// Simulate resolveSecret() swapping the generated secret for the persisted
	// one AFTER the sender was constructed.
	cfg.Secret = "persisted-secret"
	cfg.OutboundSecret = "persisted-secret"

	err := s.post(context.Background(), OutboundMessage{Kind: "assistant", Account: "acc", Peer: Peer{ID: "u1"}, Text: "hi"})
	if err != nil {
		t.Fatalf("outbound must sign with the resolved secret: %v", err)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
