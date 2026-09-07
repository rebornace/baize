package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const defaultEmptyPollWait = time.Second

// inboundPrefix is generated once per process and prefixes every inbound
// idempotency key. A plain per-process sequence counter (wx-<n>) resets to 1
// when the adapter restarts while baize keeps its dedupe cache resident, so
// post-restart messages would collide with stale keys and be silently dropped
// as duplicates. The random prefix changes on every process start, making
// keys unique across restarts while remaining stable within a process.
var inboundPrefix = func() string {
	b := make([]byte, 4)
	if _, err := crand.Read(b); err != nil {
		// crypto/rand failure is near-impossible; fall back to a timestamp
		// (hex) which still differs between process starts.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}()

var inboundSeq uint64

// startPolling launches the iLink long-poll loop. It returns errLoginRequired
// when no credentials are present. Idempotent: a no-op when already polling.
func (a *Adapter) startPolling() error {
	a.mu.Lock()
	if a.polling {
		a.mu.Unlock()
		return nil
	}
	token := a.token
	if strings.TrimSpace(token) == "" || strings.TrimSpace(a.account) == "" {
		a.mu.Unlock()
		return errLoginRequired
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.polling = true
	a.wg.Add(1)
	a.mu.Unlock()
	go a.pollLoop(ctx, token)
	return nil
}

func (a *Adapter) pollLoop(ctx context.Context, token string) {
	defer a.wg.Done()
	wait := a.emptyWait
	if wait <= 0 {
		wait = defaultEmptyPollWait
	}
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		updates, next, err := a.ilink.GetUpdates(ctx, token, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			continue
		}
		if next != "" {
			cursor = next
		}
		for _, u := range updates {
			if err := a.handleInboundUpdate(ctx, u); err != nil && ctx.Err() == nil {
				log.Printf("weixin-adapter: inbound update: %v", err)
			}
		}
		if len(updates) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
	}
}

// handleInboundUpdate drops group chats (@chatroom) and messages with no
// peer, and forwards a DM to baize. Each media ref is downloaded/decrypted
// via MediaDownloader and attached as an attachment; a media that errors or
// decrypts to zero bytes is skipped (logged) so ciphertext garbage is never
// forwarded, while the message text still goes through.
func (a *Adapter) handleInboundUpdate(ctx context.Context, u weixinlink.Update) error {
	peer := strings.TrimSpace(u.PeerID)
	if peer == "" || strings.Contains(peer, "@chatroom") {
		return nil
	}
	atts := []map[string]any{}
	if md, ok := a.ilink.(weixinlink.MediaDownloader); ok {
		for _, m := range u.Media {
			data, _, err := md.DownloadMediaDecrypted(ctx, a.currentToken(), m)
			if err != nil {
				log.Printf("weixin-adapter: download media %s: %v", m.FileName, err)
				continue
			}
			if len(data) == 0 {
				// Key present but undecryptable: skip bytes (do not forward
				// ciphertext garbage); the message text still goes through.
				log.Printf("weixin-adapter: media %s undecryptable; skipped", m.FileName)
				continue
			}
			name := strings.TrimSpace(m.FileName)
			if name == "" {
				name = "media.bin"
			}
			mime := strings.TrimSpace(m.MIME)
			if mime == "" {
				mime = "application/octet-stream"
			}
			atts = append(atts, map[string]any{
				"name":           name,
				"mime":           mime,
				"content_base64": base64.StdEncoding.EncodeToString(data),
			})
		}
	}
	return a.forwardInbound(ctx, peer, u.Text, u.ContextToken, atts)
}

func (a *Adapter) currentToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token
}

// forwardInbound signs and POSTs one inbound message to baize's webhook.
// The idempotency key is "wx-<process-prefix>-<seq>": stable within this
// process, unique across adapter restarts (see inboundPrefix).
func (a *Adapter) forwardInbound(ctx context.Context, peer, text, contextToken string, atts []map[string]any) error {
	seq := atomic.AddUint64(&inboundSeq, 1)
	payload := map[string]any{
		"event":           "message",
		"account":         a.currentAccount(),
		"peer":            map[string]any{"id": peer, "name": peer},
		"text":            text,
		"context_token":   contextToken,
		"idempotency_key": "wx-" + inboundPrefix + "-" + strconv.FormatUint(seq, 10),
	}
	if len(atts) > 0 {
		payload["attachments"] = atts
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baizeInboundURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Protocol", "v0")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", webhooksig.Sign(a.secret, ts, body))

	hc := a.httpClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpStatusError{code: resp.StatusCode}
	}
	return nil
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return "baize inbound http " + strconv.Itoa(e.code) }

func (a *Adapter) currentAccount() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.account
}

func (a *Adapter) isPolling() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.polling
}

func (a *Adapter) hasCredentials() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token != "" && a.account != ""
}
