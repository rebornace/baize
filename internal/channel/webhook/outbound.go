package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

const (
	outboundTimeout  = 10 * time.Second
	outboundMaxTries = 3
	maxOutboundBody  = 1 << 16 // 64KiB drain cap for error responses
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

// post sends msg to the adapter outbound URL, signing with the outbound
// secret. Retries connection errors and 5xx with exponential backoff; 4xx
// (protocol/config error) is not retried.
func (s *sender) post(ctx context.Context, msg OutboundMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(s.cfg.OutboundSecret, ts, body)

	var lastErr error
	for attempt := 0; attempt < outboundMaxTries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.OutboundURL, bytes.NewReader(body))
		if err != nil {
			return err
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
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxOutboundBody))
		_ = resp.Body.Close()
		code := resp.StatusCode
		if code >= 200 && code < 300 {
			return nil
		}
		if code >= 400 && code < 500 {
			return fmt.Errorf("webhook: outbound rejected with status %d (not retried)", code)
		}
		lastErr = fmt.Errorf("webhook: outbound status %d", code)
	}
	return lastErr
}

func backoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 1 * time.Second
	case 2:
		return 2 * time.Second
	default:
		return 4 * time.Second
	}
}
