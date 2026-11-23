package weixinlink

import (
	"context"
	"fmt"
	"sync"
)

// Fake is an in-memory ILink for tests.
type Fake struct {
	mu sync.Mutex

	Ticket string
	QRURL  string

	// LoginSequence is mapped statuses returned by successive PollLogin calls.
	// Empty defaults to pending then success.
	LoginSequence []string
	loginIdx      int

	AccountID string
	Token     string

	Updates    []Update
	NextCursor string

	Sent       []OutboundMessage
	SentImages []FakeMedia
	SentFiles  []FakeMedia
	MediaBytes []byte
}

// FakeMedia records an outbound image/file send captured by the Fake.
type FakeMedia struct {
	Token        string
	ToUserID     string
	FileName     string
	MIME         string
	Data         []byte
	ContextToken string
}

// NewFake returns a Fake with sensible defaults.
func NewFake() *Fake {
	return &Fake{
		Ticket:     "fake-ticket",
		QRURL:      "https://weixin.qq.com/x/fake",
		AccountID:  "fake-bot@im.bot",
		Token:      "fake-bot-token",
		MediaBytes: []byte("fake-media-bytes"),
	}
}

func (f *Fake) GetQR(context.Context) (ticket, qrURL string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Ticket, f.QRURL, nil
}

func (f *Fake) PollLogin(context.Context, string) (status, accountID, token string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	seq := f.LoginSequence
	if len(seq) == 0 {
		seq = []string{LoginStatusPending, LoginStatusSuccess}
	}
	if f.loginIdx >= len(seq) {
		return LoginStatusExpired, "", "", nil
	}
	status = seq[f.loginIdx]
	f.loginIdx++

	switch status {
	case LoginStatusSuccess:
		return status, f.AccountID, f.Token, nil
	case LoginStatusPending, LoginStatusExpired:
		return status, "", "", nil
	default:
		return "", "", "", fmt.Errorf("weixin fake: unknown login status %q", status)
	}
}

func (f *Fake) GetUpdates(context.Context, string, string) ([]Update, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Update, len(f.Updates))
	copy(out, f.Updates)
	// Drain so poll loops do not re-deliver the same batch.
	f.Updates = nil
	return out, f.NextCursor, nil
}

func (f *Fake) SendMessage(_ context.Context, _ string, msg OutboundMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Sent = append(f.Sent, msg)
	return nil
}

func (f *Fake) SendImage(_ context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SentImages = append(f.SentImages, FakeMedia{Token: token, ToUserID: toUserID, FileName: filename, MIME: mime, Data: append([]byte(nil), data...), ContextToken: contextToken})
	return nil
}

func (f *Fake) SendFile(_ context.Context, token, toUserID, filename, mime string, data []byte, contextToken string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.SentFiles = append(f.SentFiles, FakeMedia{Token: token, ToUserID: toUserID, FileName: filename, MIME: mime, Data: append([]byte(nil), data...), ContextToken: contextToken})
	return nil
}

func (f *Fake) DownloadMedia(context.Context, string, MediaRef) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.MediaBytes))
	copy(out, f.MediaBytes)
	return out, nil
}

func (f *Fake) DownloadMediaDecrypted(context.Context, string, MediaRef) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.MediaBytes))
	copy(out, f.MediaBytes)
	return out, false, nil // fake media is plaintext
}

var _ ILink = (*Fake)(nil)
var _ MediaDownloader = (*Fake)(nil)
