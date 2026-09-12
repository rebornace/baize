package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/channel"
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

func TestSendTextPostsSignedOutbound(t *testing.T) {
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
	if err := ch.SendText(context.Background(), "peer9", "你好", map[string]string{"context_token": "tok123"}); err != nil {
		t.Fatalf("SendText: %v", err)
	}
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
	// Default kind is assistant; no run id -> no run-id header.
	if got.Kind != "assistant" {
		t.Fatalf("default kind=%q want assistant", got.Kind)
	}
	if got.RunID != "" || hdr.Get(HeaderRunID) != "" {
		t.Fatalf("run id should be empty when not provided: body=%q hdr=%q", got.RunID, hdr.Get(HeaderRunID))
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
	extras := map[string]string{
		"kind":          "notify",
		"run_id":        "run-77",
		"context_token": "tok",
	}
	if err := ch.SendText(context.Background(), "p", "审批？", extras); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if got.Kind != "notify" {
		t.Fatalf("kind=%q want notify", got.Kind)
	}
	if got.RunID != "run-77" {
		t.Fatalf("body run_id=%q want run-77", got.RunID)
	}
	if hdr.Get(HeaderRunID) != "run-77" {
		t.Fatalf("X-Baize-Run-Id header=%q want run-77", hdr.Get(HeaderRunID))
	}
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
	// White-box: attach a runtime the way Bootstrap would, then learn the
	// post-login account from a (simulated) inbound.
	c.rt = &channel.Runtime{Assignee: "channel:weixin", DefaultAgentID: "ag", Source: "weixin"}
	c.setActiveAccount("bot@im.bot")

	if err := c.SendText(context.Background(), "peer@im.wechat", "hi", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	m := <-got
	if m.Account != "bot@im.bot" {
		t.Fatalf("outbound account = %q want bot@im.bot", m.Account)
	}
	if m.ConversationID != "weixin:bot@im.bot:peer@im.wechat" {
		t.Fatalf("conv id = %q", m.ConversationID)
	}
}
