package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
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
	var got OutboundMessage
	srv := newCaptureServer(&got)
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
}

func newCaptureServer(got *OutboundMessage) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, got)
		w.WriteHeader(http.StatusOK)
	}))
}
