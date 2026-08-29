package channel

import (
	"context"
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
