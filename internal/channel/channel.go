package channel

import (
	"context"
	"strings"
)

// Channel is a messaging channel plugin (weixin, etc.).
type Channel interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	SendText(ctx context.Context, peerID, text string, extras map[string]string) error
	SendMedia(ctx context.Context, peerID string, filename string, mime string, data []byte, extras map[string]string) error
}

// Inbound is a normalized inbound message from a Channel.
type Inbound struct {
	PeerID string
	Text   string
	Files  []InboundFile
	// Extras carries channel-specific fields such as context_token and account.
	Extras map[string]string
}

// InboundFile is a media/file payload on an inbound message.
type InboundFile struct {
	Name string
	MIME string
	Data []byte
}

// SourceSourced is implemented by channels whose conversations carry a distinct
// meta.Source / conv-id prefix (weixin today; feishu/dingtalk tomorrow).
type SourceSourced interface {
	Source() string // meta.Source value, e.g. "weixin"
}

// ConvID builds a channel conversation id "<source>:<account>:<peer>".
func ConvID(source, account, peer string) string {
	return source + ":" + account + ":" + peer
}

// SourceFromConvID returns the prefix before the first ":" (the channel source).
func SourceFromConvID(convID string) string {
	if i := strings.IndexByte(convID, ':'); i >= 0 {
		return convID[:i]
	}
	return convID
}
