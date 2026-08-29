package weixin

import "context"

// Login status values returned by PollLogin (mapped from iLink wait/scaned/confirmed/expired).
const (
	LoginStatusPending = "pending"
	LoginStatusSuccess = "success"
	LoginStatusExpired = "expired"
)

// DefaultBaseURL is the public iLink bot API base.
const DefaultBaseURL = "https://ilinkai.weixin.qq.com"

// DefaultCDNBaseURL is the public media CDN base for downloads.
const DefaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"

// ILink is the weixin iLink bot HTTP client surface.
type ILink interface {
	GetQR(ctx context.Context) (ticket, qrURL string, err error)
	PollLogin(ctx context.Context, ticket string) (status, accountID, token string, err error)
	GetUpdates(ctx context.Context, token, cursor string) (updates []Update, nextCursor string, err error)
	SendMessage(ctx context.Context, token string, msg OutboundMessage) error
	DownloadMedia(ctx context.Context, token string, media MediaRef) ([]byte, error)
}

// Update is a normalized inbound message from getupdates.
type Update struct {
	PeerID       string
	Text         string
	ContextToken string
	Media        []MediaRef
}

// OutboundMessage is a text outbound sendmessage payload.
type OutboundMessage struct {
	ToUserID     string
	Text         string
	ContextToken string
	ClientID     string
}

// MediaRef holds CDN / encrypted media fields for download.
type MediaRef struct {
	URL               string
	EncryptQueryParam string
	AESKey            string
	FileName          string
	MIME              string
}
