package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

const (
	outboundTimeout = 10 * time.Second
	maxOutboundBody = 1 << 16 // 64KiB drain cap for error responses
)

// sender posts signed outbound messages to the adapter. It holds a pointer to
// the channel's live config (not a copy): the HMAC secret is finalized in
// Bootstrap via resolveSecret() (which may replace a freshly generated secret
// with the persisted one AFTER openFromConfig built this sender). A by-value
// copy would keep signing with the stale, pre-resolution secret and every
// outbound call would 401 while admin calls (built later) succeed.
type sender struct {
	cfg *instanceConfig
	hc  *http.Client
}

func newSender(cfg *instanceConfig) *sender {
	return &sender{cfg: cfg, hc: &http.Client{Timeout: outboundTimeout}}
}

// postOnce performs a single signed POST to targetURL. Retries are owned by the
// channel outbox worker. On HTTP responses, err is nil and the status code is
// returned (including 4xx/5xx) so callers can apply Retryable semantics.
func (s *sender) postOnce(ctx context.Context, targetURL string, msg OutboundMessage) (int, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return 0, err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(s.cfg.OutboundSecret, ts, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderProtocol, ProtocolVersion)
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, sig)
	if msg.RunID != "" {
		req.Header.Set(HeaderRunID, msg.RunID)
	}
	resp, err := s.hc.Do(req)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxOutboundBody))
	_ = resp.Body.Close()
	// HTTP responses return status with nil err so Retryable can distinguish
	// 4xx (dead) from 5xx/429 (retry). Network failures return err only.
	return resp.StatusCode, nil
}
