package channel

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestDeliverUserTextWeixinMetaSendsText(t *testing.T) {
	ch := &fakeChannel{name: "fake"}
	meta := conversation.Meta{
		ID:          "weixin:acc:peer-1",
		Source:      "weixin",
		ChannelPeer: "peer-1",
	}
	DeliverUserText(context.Background(), ch, meta, "来自 UI 的用户话", map[string]string{
		"context_token": "tok-u",
	})
	sent := ch.texts()
	if len(sent) != 1 {
		t.Fatalf("SendText calls = %d, want 1", len(sent))
	}
	if sent[0].peerID != "peer-1" || sent[0].text != OutboundPrefixOperator+"来自 UI 的用户话" {
		t.Fatalf("SendText = %+v", sent[0])
	}
	if sent[0].extras["context_token"] != "tok-u" {
		t.Fatalf("extras = %+v", sent[0].extras)
	}
}

func TestFormatOutboundPrefixes(t *testing.T) {
	if got := FormatOperatorOutbound("hi"); got != OutboundPrefixOperator+"hi" {
		t.Fatalf("operator=%q", got)
	}
	if got := FormatAssistantOutbound("yo"); got != OutboundPrefixAssistant+"yo" {
		t.Fatalf("assistant=%q", got)
	}
	// Idempotent when already prefixed.
	if got := FormatOperatorOutbound(OutboundPrefixOperator + "x"); got != OutboundPrefixOperator+"x" {
		t.Fatalf("operator idempotent=%q", got)
	}
}

func TestDeliverUserTextUIMetaSkips(t *testing.T) {
	ch := &fakeChannel{name: "fake"}
	DeliverUserText(context.Background(), ch, conversation.Meta{Source: "ui"}, "hello", nil)
	if n := len(ch.texts()); n != 0 {
		t.Fatalf("SendText calls = %d, want 0", n)
	}
}

func TestDeliverAssistantReplyWeixinMetaSendsText(t *testing.T) {
	ch := &fakeChannel{name: "fake"}
	meta := conversation.Meta{
		ID:          "weixin:acc:peer-1",
		Source:      "weixin",
		ChannelPeer: "peer-1",
	}
	DeliverAssistantReply(context.Background(), ch, meta, "助手回复", nil, map[string]string{
		"context_token": "tok-1",
	})
	sent := ch.texts()
	if len(sent) != 1 {
		t.Fatalf("SendText calls = %d, want 1", len(sent))
	}
	if sent[0].peerID != "peer-1" || sent[0].text != OutboundPrefixAssistant+"助手回复" {
		t.Fatalf("SendText = %+v", sent[0])
	}
	if sent[0].extras["context_token"] != "tok-1" {
		t.Fatalf("extras = %+v", sent[0].extras)
	}
}

func TestDeliverAssistantReplyUIMetaSkips(t *testing.T) {
	ch := &fakeChannel{name: "fake"}
	meta := conversation.Meta{
		ID:     "ui-conv",
		Source: "ui",
	}
	DeliverAssistantReply(context.Background(), ch, meta, "hello", nil, nil)
	if n := len(ch.texts()); n != 0 {
		t.Fatalf("SendText calls = %d, want 0", n)
	}
}

func TestDeliverAssistantReplyNilChannelNoPanic(t *testing.T) {
	meta := conversation.Meta{Source: "weixin", ChannelPeer: "peer-1"}
	DeliverAssistantReply(context.Background(), nil, meta, "hello", nil, nil)
}

func TestDeliverAssistantReplyEmptyPeerSkips(t *testing.T) {
	ch := &fakeChannel{name: "fake"}
	meta := conversation.Meta{Source: "weixin", ChannelPeer: "  "}
	DeliverAssistantReply(context.Background(), ch, meta, "hello", nil, nil)
	if n := len(ch.texts()); n != 0 {
		t.Fatalf("SendText calls = %d, want 0", n)
	}
}

func TestDeliverAssistantReplySendErrorDoesNotPanic(t *testing.T) {
	ch := &errChannel{err: errors.New("send failed")}
	meta := conversation.Meta{Source: "weixin", ChannelPeer: "peer-1"}
	DeliverAssistantReply(context.Background(), ch, meta, "hello", nil, nil)
}

func TestDeliverAssistantReplySendsMedia(t *testing.T) {
	ch := &recordingMediaChannel{fakeChannel: fakeChannel{name: "fake"}}
	meta := conversation.Meta{Source: "weixin", ChannelPeer: "peer-1"}
	DeliverAssistantReply(context.Background(), ch, meta, "caption", []OutboundMedia{{
		Filename: "a.png",
		MIME:     "image/png",
		Data:     []byte{1, 2, 3},
	}}, nil)
	if len(ch.texts()) != 1 {
		t.Fatalf("SendText calls = %d, want 1", len(ch.texts()))
	}
	media := ch.medias()
	if len(media) != 1 {
		t.Fatalf("SendMedia calls = %d, want 1", len(media))
	}
	if media[0].peerID != "peer-1" || media[0].filename != "a.png" || media[0].mime != "image/png" {
		t.Fatalf("SendMedia = %+v", media[0])
	}
}

func TestDeliverAssistantReplyNotStartedSkips(t *testing.T) {
	ch := &startedFakeChannel{fakeChannel: fakeChannel{name: "fake"}, started: false}
	meta := conversation.Meta{Source: "weixin", ChannelPeer: "peer-1"}
	DeliverAssistantReply(context.Background(), ch, meta, "hello", nil, nil)
	if n := len(ch.texts()); n != 0 {
		t.Fatalf("SendText calls = %d, want 0 when not started", n)
	}
	ch.started = true
	DeliverAssistantReply(context.Background(), ch, meta, "hello", nil, nil)
	if n := len(ch.texts()); n != 1 {
		t.Fatalf("SendText calls = %d, want 1 when started", n)
	}
}

type startedFakeChannel struct {
	fakeChannel
	started bool
}

func (s *startedFakeChannel) IsStarted() bool { return s.started }

type errChannel struct {
	err error
}

func (e *errChannel) Name() string                { return "err" }
func (e *errChannel) Start(context.Context) error { return nil }
func (e *errChannel) Stop(context.Context) error  { return nil }
func (e *errChannel) SendText(context.Context, string, string, map[string]string) error {
	return e.err
}
func (e *errChannel) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return e.err
}

type recordingMediaChannel struct {
	fakeChannel
	mu    sync.Mutex
	media []sentMedia
}

type sentMedia struct {
	peerID   string
	filename string
	mime     string
	data     []byte
}

func (r *recordingMediaChannel) SendMedia(_ context.Context, peerID, filename, mime string, data []byte, _ map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := append([]byte(nil), data...)
	r.media = append(r.media, sentMedia{peerID: peerID, filename: filename, mime: mime, data: cp})
	return nil
}

func (r *recordingMediaChannel) medias() []sentMedia {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]sentMedia, len(r.media))
	copy(out, r.media)
	return out
}
