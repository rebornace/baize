package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhooksig"
)

func TestOpenFromConfigRequiresInstanceName(t *testing.T) {
	// empty instance name must error (multi-instance disambiguation)
	if _, err := openFromConfig("", map[string]string{"secret": "s", "outbound_url": "http://x/o", "assignee": "a"}); err == nil {
		t.Fatal("expected error for empty instance name")
	}
}

func TestOpenFromConfigBuildsChannel(t *testing.T) {
	ch, err := openFromConfig("feishu", map[string]string{
		"source":       "feishu",
		"account":      "acc-1",
		"secret":       "s",
		"outbound_url": "http://x/o",
		"assignee":     "u-admin",
		"agent_id":     "ag1",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if ch.Name() != "feishu" {
		t.Fatalf("Name()=%q want feishu (instance name)", ch.Name())
	}
	if ch.Source() != "feishu" {
		t.Fatalf("Source()=%q want feishu", ch.Source())
	}
}

func attachOutboxForTest(t *testing.T, ch *Channel) *store.Memory {
	t.Helper()
	mem := store.NewMemory()
	ch.SetStore(mem)
	if ch.outboxWake == nil {
		ch.outboxWake = make(chan struct{}, 1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go ch.StartOutboxWorker(ctx)
	return mem
}

func TestSendTextPostsSignedOutbound(t *testing.T) {
	var (
		got OutboundMessage
		hdr http.Header
		raw []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ = io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		hdr = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	ch, _ := openFromConfig("feishu", map[string]string{
		"source": "feishu", "account": "acc-1", "secret": "s",
		"outbound_url": srv.URL, "assignee": "u-admin",
	})
	mem := attachOutboxForTest(t, ch)
	if err := ch.SendText(context.Background(), "peer9", "你好", map[string]string{"context_token": "tok123"}); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	_ = waitChannelOutbox(t, mem, "feishu", store.ChannelOutboxDelivered, 5*time.Second)
	if got.Peer.ID != "peer9" || got.Text != "你好" {
		t.Fatalf("bad peer/text: %+v", got)
	}
	if got.Account != "acc-1" {
		t.Fatalf("outbound account should be instance account, got %q", got.Account)
	}
	if got.ConversationID == "" {
		t.Fatal("ConversationID should be set")
	}
	if got.ContextToken != "tok123" {
		t.Fatalf("context token not propagated: %+v", got)
	}
	if got.Kind != "assistant" {
		t.Fatalf("default kind=%q want assistant", got.Kind)
	}
	if got.RunID != "" || hdr.Get(HeaderRunID) != "" {
		t.Fatalf("run id should be empty when not provided: body=%q hdr=%q", got.RunID, hdr.Get(HeaderRunID))
	}
	if err := webhooksig.Verify(ch.cfg.OutboundSecret, hdr.Get(HeaderTimestamp), raw, hdr.Get(HeaderSignature), time.Now(), 300*time.Second); err != nil {
		t.Fatalf("signature verify: %v", err)
	}
}

func TestSendTextPropagatesKindAndRunID(t *testing.T) {
	var (
		got OutboundMessage
		hdr http.Header
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		hdr = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	ch, _ := openFromConfig("feishu", map[string]string{
		"source": "feishu", "account": "acc-1", "secret": "s",
		"outbound_url": srv.URL, "assignee": "u-admin",
	})
	_ = attachOutboxForTest(t, ch)
	extras := map[string]string{
		"kind":          "notify",
		"run_id":        "run-77",
		"context_token": "tok",
	}
	if err := ch.SendText(context.Background(), "p", "审批？", extras); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if got.Kind == "notify" && got.RunID == "run-77" && hdr.Get(HeaderRunID) == "run-77" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("kind/run_id not delivered: kind=%q run=%q hdr=%q", got.Kind, got.RunID, hdr.Get(HeaderRunID))
}

func TestOpenFromConfigRejectsUnsafeInstanceName(t *testing.T) {
	base := map[string]string{"secret": "s", "outbound_url": "http://x/o", "assignee": "a"}
	for _, bad := range []string{"a/b", "a b", "a?b", "a#b", "中文"} {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		if _, err := openFromConfig(bad, m); err == nil {
			t.Fatalf("expected error for unsafe instance name %q", bad)
		}
	}
}

func TestDynamicAccountInboundThenOutbound(t *testing.T) {
	// Inbound carries the post-login account; outbound must use it (not the
	// static config account/name).
	got := make(chan OutboundMessage, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m OutboundMessage
		_ = json.NewDecoder(r.Body).Decode(&m)
		got <- m
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c, err := openFromConfig("weixin", map[string]string{
		"source":       "weixin",
		"secret":       "s",
		"outbound_url": srv.URL,
		"assignee":     "channel:weixin",
		// no static account: defaults to name "weixin" until first inbound.
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = attachOutboxForTest(t, c)
	// White-box: attach a runtime the way Bootstrap would, then learn the
	// post-login account from a (simulated) inbound.
	c.rt = &channel.Runtime{Assignee: "channel:weixin", DefaultAgentID: "ag", Source: "weixin"}
	c.setActiveAccount("bot@im.bot")

	if err := c.SendText(context.Background(), "peer@im.wechat", "hi", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m.Account != "bot@im.bot" {
			t.Fatalf("outbound account = %q want bot@im.bot", m.Account)
		}
		if m.ConversationID != "weixin:bot@im.bot:peer@im.wechat" {
			t.Fatalf("conv id = %q", m.ConversationID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for outbound")
	}
}
