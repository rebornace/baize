package channel

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

// sourcedFake wraps fakeChannel with a distinct meta.Source so the Router can
// key it (like a future feishu/dingtalk channel alongside weixin).
type sourcedFake struct {
	fakeChannel
	source string
}

func (s *sourcedFake) Source() string { return s.source }

func TestRouterRoutesByMetaSource(t *testing.T) {
	wx := &sourcedFake{fakeChannel: fakeChannel{name: "wx"}, source: "weixin"}
	fs := &sourcedFake{fakeChannel: fakeChannel{name: "fs"}, source: "feishu"}
	r := NewRouter()
	r.Add(wx)
	r.Add(fs)

	// weixin 会话的回复只发到 wx
	DeliverAssistantReply(context.Background(), r, conversation.Meta{
		Source: "weixin", ChannelPeer: "peer-w",
	}, "hi", nil, nil)
	if n := len(wx.texts()); n != 1 {
		t.Fatalf("wx SendText=%d want 1", n)
	}
	if n := len(fs.texts()); n != 0 {
		t.Fatalf("feishu should not receive, got %d", n)
	}

	// feishu 会话的回复只发到 fs
	DeliverAssistantReply(context.Background(), r, conversation.Meta{
		Source: "feishu", ChannelPeer: "peer-f",
	}, "yo", nil, nil)
	if n := len(fs.texts()); n != 1 {
		t.Fatalf("fs SendText=%d want 1", n)
	}
}

func TestRouterUnknownSourceSkips(t *testing.T) {
	r := NewRouter()
	r.Add(&sourcedFake{fakeChannel: fakeChannel{name: "wx"}, source: "weixin"})
	// 未知 source（例如 ui 会话）：不投递、不 panic
	DeliverUserText(context.Background(), r, conversation.Meta{Source: "ui"}, "x", nil)
	DeliverAssistantReply(context.Background(), r, conversation.Meta{Source: "dingtalk", ChannelPeer: "p"}, "y", nil, nil)
}

func TestRouterSingleWeixinBehavesLikeDirect(t *testing.T) {
	// 单渠道（现状）：Router 只含 weixin，weixin 会话正常投递
	wx := &sourcedFake{fakeChannel: fakeChannel{name: "wx"}, source: "weixin"}
	r := NewRouter()
	r.Add(wx)
	DeliverAssistantReply(context.Background(), r, conversation.Meta{
		ID: "weixin:acc:p", Source: "weixin", ChannelPeer: "p",
	}, "回复", nil, nil)
	sent := wx.texts()
	if len(sent) != 1 || sent[0].text != OutboundPrefixAssistant+"回复" {
		t.Fatalf("single-router deliver = %+v", sent)
	}
}

func TestRouterExtrasByConvSource(t *testing.T) {
	r := NewRouter()
	rt := &Runtime{Source: "weixin"}
	rt.rememberContextToken("weixin:acc:p1", map[string]string{"context_token": "tok-1"})
	r.BindRuntime(rt)
	if got := r.Extras("weixin:acc:p1"); got["context_token"] != "tok-1" {
		t.Fatalf("extras=%v", got)
	}
	if got := r.Extras("feishu:x:y"); got != nil {
		t.Fatalf("unknown source extras=%v want nil", got)
	}
}
