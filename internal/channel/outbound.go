package channel

import (
	"context"
	"log"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

// OutboundMedia is an optional attachment to deliver alongside assistant text.
type OutboundMedia struct {
	Filename string
	MIME     string
	Data     []byte
}

// startedReporter is optional; when implemented, Deliver skips if not started
// (e.g. weixin logged out / poll loop stopped).
type startedReporter interface {
	IsStarted() bool
}

func weixinPeer(ch Channel, meta conversation.Meta) (peer string, ok bool) {
	if ch == nil {
		return "", false
	}
	if sr, ok := ch.(startedReporter); ok && !sr.IsStarted() {
		return "", false
	}
	if strings.TrimSpace(meta.Source) != "weixin" {
		return "", false
	}
	peer = strings.TrimSpace(meta.ChannelPeer)
	if peer == "" {
		return "", false
	}
	return peer, true
}

// DeliverUserText mirrors a /ui (or API) user turn to the weixin peer so the
// phone chat shows what the operator typed. Failures are logged only.
func DeliverUserText(ctx context.Context, ch Channel, meta conversation.Meta, text string, extras map[string]string) {
	peer, ok := weixinPeer(ch, meta)
	if !ok {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if err := ch.SendText(ctx, peer, text, copyExtras(extras)); err != nil {
		log.Printf("channel outbound user: SendText peer=%s: %v", peer, err)
	}
}

// DeliverAssistantReply sends a succeeded assistant reply to the channel peer
// when meta identifies a weixin conversation. Failures are logged only; callers
// must not treat errors as run failures. Nil channel is a no-op.
func DeliverAssistantReply(ctx context.Context, ch Channel, meta conversation.Meta, text string, media []OutboundMedia, extras map[string]string) {
	peer, ok := weixinPeer(ch, meta)
	if !ok {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" && len(media) == 0 {
		return
	}

	ex := copyExtras(extras)
	if text != "" {
		if err := ch.SendText(ctx, peer, text, ex); err != nil {
			log.Printf("channel outbound: SendText peer=%s: %v", peer, err)
		}
	}
	for _, m := range media {
		if len(m.Data) == 0 && strings.TrimSpace(m.Filename) == "" {
			continue
		}
		if err := ch.SendMedia(ctx, peer, m.Filename, m.MIME, m.Data, ex); err != nil {
			log.Printf("channel outbound: SendMedia peer=%s file=%s: %v", peer, m.Filename, err)
		}
	}
}
