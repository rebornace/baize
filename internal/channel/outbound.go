package channel

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

// Outbound role prefixes — both UI operator and assistant text are sent as Bot
// bubbles on WeChat, so labels are required for the phone user to tell them apart.
const (
	OutboundPrefixOperator  = "【客服】"
	OutboundPrefixAssistant = "【助手】"
)

// OperatorOutboundSettle is waited after mirroring a /ui operator turn so the
// WeChat client is more likely to show it before the following assistant reply.
const OperatorOutboundSettle = 400 * time.Millisecond

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

// resolveOutbound picks the channel + peer for a conversation. When ch is a
// *Router it selects the child matching meta.Source; otherwise ch is used
// directly (back-compat for tests/nil). Returns false for ui-only / unknown.
func resolveOutbound(ch Channel, meta conversation.Meta) (Channel, string, bool) {
	if ch == nil {
		return nil, "", false
	}
	if r, ok := ch.(*Router); ok {
		c, ok := r.For(strings.TrimSpace(meta.Source))
		if !ok {
			return nil, "", false
		}
		ch = c
	} else {
		// 非 Router：保持原语义——只有渠道自报 source 与 meta.Source 一致才投递；
		// 无 Source() 的旧渠道（如测试夹具）默认按 weixin 语义处理。
		if ss, ok := ch.(SourceSourced); ok {
			if strings.TrimSpace(meta.Source) != ss.Source() {
				return nil, "", false
			}
		} else if strings.TrimSpace(meta.Source) != "weixin" {
			return nil, "", false
		}
	}
	if sr, ok := ch.(startedReporter); ok && !sr.IsStarted() {
		return nil, "", false
	}
	peer := strings.TrimSpace(meta.ChannelPeer)
	if peer == "" {
		return nil, "", false
	}
	return ch, peer, true
}

// FormatOperatorOutbound prefixes a /ui operator turn for WeChat display.
func FormatOperatorOutbound(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, OutboundPrefixOperator) || strings.HasPrefix(text, OutboundPrefixAssistant) {
		return text
	}
	return OutboundPrefixOperator + text
}

// FormatAssistantOutbound prefixes an assistant reply for WeChat display.
func FormatAssistantOutbound(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, OutboundPrefixOperator) || strings.HasPrefix(text, OutboundPrefixAssistant) {
		return text
	}
	return OutboundPrefixAssistant + text
}

// DeliverUserText mirrors a /ui (or API) user turn to the weixin peer so the
// phone chat shows what the operator typed. Failures are logged only.
func DeliverUserText(ctx context.Context, ch Channel, meta conversation.Meta, text string, extras map[string]string) {
	target, peer, ok := resolveOutbound(ch, meta)
	if !ok {
		return
	}
	text = FormatOperatorOutbound(text)
	if text == "" {
		return
	}
	if err := target.SendText(ctx, peer, text, copyExtras(extras)); err != nil {
		log.Printf("channel outbound user: SendText peer=%s: %v", peer, err)
		return
	}
	// Sequential HTTP is not always enough for WeChat UI ordering; brief settle.
	t := time.NewTimer(OperatorOutboundSettle)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// DeliverAssistantReply sends a succeeded assistant reply to the channel peer
// when meta identifies a weixin conversation. Failures are logged only; callers
// must not treat errors as run failures. Nil channel is a no-op.
func DeliverAssistantReply(ctx context.Context, ch Channel, meta conversation.Meta, text string, media []OutboundMedia, extras map[string]string) {
	target, peer, ok := resolveOutbound(ch, meta)
	if !ok {
		return
	}
	text = FormatAssistantOutbound(text)
	if text == "" && len(media) == 0 {
		return
	}

	ex := copyExtras(extras)
	if text != "" {
		if err := target.SendText(ctx, peer, text, ex); err != nil {
			log.Printf("channel outbound: SendText peer=%s: %v", peer, err)
		}
	}
	for _, m := range media {
		if len(m.Data) == 0 && strings.TrimSpace(m.Filename) == "" {
			continue
		}
		if err := target.SendMedia(ctx, peer, m.Filename, m.MIME, m.Data, ex); err != nil {
			log.Printf("channel outbound: SendMedia peer=%s file=%s: %v", peer, m.Filename, err)
		}
	}
}
