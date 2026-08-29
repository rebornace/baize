package weixin

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/channel"
)

const (
	defaultCredsDir      = "./data/channels/weixin"
	defaultEmptyPollWait = time.Second
)

func init() {
	channel.RegisterChannel("weixin", openFromConfig)
}

// Channel is the weixin messaging plugin (iLink long-poll + outbound).
type Channel struct {
	ilink     ILink
	runtime   *channel.Runtime
	accountID string
	token     string
	credsDir  string

	mu            sync.Mutex
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	started       bool
	cursor        string
	emptyPollWait time.Duration
}

// New constructs a Channel with injected ILink and Runtime (tests / Task 6 wiring).
func New(ilink ILink, rt *channel.Runtime, accountID, token string) *Channel {
	return &Channel{
		ilink:         ilink,
		runtime:       rt,
		accountID:     accountID,
		token:         token,
		emptyPollWait: defaultEmptyPollWait,
	}
}

// SetRuntime attaches the inbound Runtime (Task 6 bootstrap).
func (c *Channel) SetRuntime(rt *channel.Runtime) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runtime = rt
}

// SetCredsDir sets the directory used for credential persistence.
func (c *Channel) SetCredsDir(dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.credsDir = dir
}

// SetCredentials updates in-memory account/token (after login).
func (c *Channel) SetCredentials(accountID, token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accountID = accountID
	c.token = token
}

// ClearCredentials clears in-memory account/token (after logout).
func (c *Channel) ClearCredentials() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accountID = ""
	c.token = ""
}

// IsStarted reports whether the poll loop is running.
func (c *Channel) IsStarted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.started
}

func openFromConfig(cfg channel.Config) (channel.Channel, error) {
	dir := strings.TrimSpace(cfg["creds_dir"])
	if dir == "" {
		dir = defaultCredsDir
	}
	baseURL := strings.TrimSpace(cfg["base_url"])
	ilink := NewClient(baseURL, nil)

	accountID, token, err := LoadCreds(dir)
	if err != nil {
		// Not logged in yet — still register a Channel shell; Start will fail until creds exist.
		accountID, token = "", ""
	}
	ch := &Channel{
		ilink:         ilink,
		accountID:     accountID,
		token:         token,
		credsDir:      dir,
		emptyPollWait: defaultEmptyPollWait,
	}
	return ch, nil
}

func (c *Channel) Name() string { return "weixin" }

func (c *Channel) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return nil
	}
	if strings.TrimSpace(c.token) == "" || strings.TrimSpace(c.accountID) == "" {
		return fmt.Errorf("weixin: missing credentials")
	}
	if c.runtime == nil {
		return fmt.Errorf("weixin: runtime not set")
	}
	if c.ilink == nil {
		return fmt.Errorf("weixin: ilink client not set")
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.started = true
	wait := c.emptyPollWait
	if wait <= 0 {
		wait = defaultEmptyPollWait
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.pollLoop(runCtx, wait)
	}()
	return nil
}

func (c *Channel) Stop(ctx context.Context) error {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.started = false
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Channel) pollLoop(ctx context.Context, emptyWait time.Duration) {
	for {
		if ctx.Err() != nil {
			return
		}
		token := c.token
		cursor := c.cursor
		updates, next, err := c.ilink.GetUpdates(ctx, token, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(emptyWait):
			}
			continue
		}
		if next != "" {
			c.cursor = next
		}
		for _, u := range updates {
			if err := c.handleUpdate(ctx, u); err != nil && ctx.Err() == nil {
				// Keep polling; inbound errors should not kill the loop.
				continue
			}
			if ctx.Err() != nil {
				return
			}
		}
		if len(updates) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(emptyWait):
			}
		}
	}
}

func (c *Channel) handleUpdate(ctx context.Context, u Update) error {
	peer := strings.TrimSpace(u.PeerID)
	if peer == "" || isGroupPeer(peer) {
		return nil
	}
	files := make([]channel.InboundFile, 0, len(u.Media))
	for _, m := range u.Media {
		data, err := c.ilink.DownloadMedia(ctx, c.token, m)
		if err != nil {
			return fmt.Errorf("weixin download media: %w", err)
		}
		name := m.FileName
		if name == "" {
			name = "media.bin"
		}
		mime := m.MIME
		if mime == "" {
			mime = "application/octet-stream"
		}
		files = append(files, channel.InboundFile{
			Name: name,
			MIME: mime,
			Data: data,
		})
	}
	extras := map[string]string{
		"account":       c.accountID,
		"context_token": u.ContextToken,
	}
	return c.runtime.HandleInbound(ctx, c, channel.Inbound{
		PeerID: peer,
		Text:   u.Text,
		Files:  files,
		Extras: extras,
	})
}

func isGroupPeer(peerID string) bool {
	return strings.Contains(peerID, "@chatroom")
}

func (c *Channel) SendText(ctx context.Context, peerID, text string, extras map[string]string) error {
	return c.sendViaILink(ctx, peerID, text, extras)
}

func (c *Channel) SendMedia(ctx context.Context, peerID, filename, mime string, data []byte, extras map[string]string) error {
	// iLink outbound media upload is not in v0 client surface; send a text placeholder via SendMessage.
	_ = mime
	_ = data
	label := strings.TrimSpace(filename)
	if label == "" {
		label = "media"
	}
	return c.sendViaILink(ctx, peerID, "（附件："+label+"）", extras)
}

func (c *Channel) sendViaILink(ctx context.Context, peerID, text string, extras map[string]string) error {
	if c.ilink == nil {
		return fmt.Errorf("weixin: ilink client not set")
	}
	ctxToken := ""
	if extras != nil {
		ctxToken = extras["context_token"]
	}
	return c.ilink.SendMessage(ctx, c.token, OutboundMessage{
		ToUserID:     peerID,
		Text:         text,
		ContextToken: ctxToken,
	})
}

var _ channel.Channel = (*Channel)(nil)
