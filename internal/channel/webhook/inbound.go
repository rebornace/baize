package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	urlpkg "net/url"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/webhooksig"
)

const (
	maxInboundBody = 10 << 20 // 10MiB (base64 media included)
	maxFetchBody   = 10 << 20
	signatureSkew  = 300 * time.Second
)

// idempotencyTTL bounds how long a successfully-handled idempotency key is
// remembered for duplicate suppression. It is a package-level var (not const)
// so tests can temporarily shrink it; expired keys are swept lazily.
var idempotencyTTL = 10 * time.Minute

// inboundHandler returns the http.Handler for POST /v0/channels/{name}/inbound.
func (c *Channel) inboundHandler() http.Handler {
	var (
		mu     sync.Mutex
		seenAt = map[string]time.Time{}
	)
	fetch := newFetchClient()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxInboundBody))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
			return
		}
		// Verify the signature BEFORE parsing or trusting any of the body.
		if err := webhooksig.Verify(c.cfg.Secret, r.Header.Get(HeaderTimestamp), body,
			r.Header.Get(HeaderSignature), time.Now(), signatureSkew); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
			return
		}
		var msg InboundMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
			return
		}
		if msg.Event != "" && msg.Event != "message" {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored"})
			return
		}
		peer := msg.Peer.ID
		if peer == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer.id required"})
			return
		}
		// Mandatory inbound allowlist; reads the hot-updatable settings
		// allowlist (empty = open to all).
		if !c.peerAllowed(peer) {
			log.Printf("webhook %s: peer %q not in allowlist; ignored", c.cfg.Name, peer)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		// Best-effort idempotency (at-least-once delivery). The key is only
		// marked seen AFTER successful handling: a failed attempt (bad
		// attachment, HandleInbound error) must not consume the key, or a
		// legitimate retry would be acked as duplicate and silently dropped.
		key := msg.IdempotencyKey
		if key != "" {
			mu.Lock()
			now := time.Now()
			// Lazy expiry sweep: drop entries older than the TTL so the map
			// cannot grow without bound. The table is small and writes are
			// infrequent, so an O(n) sweep under the lock is acceptable.
			for k, t := range seenAt {
				if now.Sub(t) >= idempotencyTTL {
					delete(seenAt, k)
				}
			}
			t, seen := seenAt[key]
			dup := seen && now.Sub(t) < idempotencyTTL
			mu.Unlock()
			if dup {
				writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
				return
			}
		}
		files, err := c.resolveAttachments(r.Context(), fetch, msg.Attachments)
		if err != nil {
			log.Printf("webhook %s: attachment error: %v", c.cfg.Name, err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "attachment"})
			return
		}
		// The account stamped on inbound is the adapter's real post-login account
		// (autostart adapters do not know it at config time). Learn it here so
		// subsequent outbound messages use it; fall back to the static config.
		acct := strings.TrimSpace(msg.Account)
		if acct == "" {
			acct = c.cfg.Account
		}
		c.setActiveAccount(acct)
		extras := map[string]string{"account": acct}
		if msg.ContextToken != "" {
			extras["context_token"] = msg.ContextToken
		}
		in := channel.Inbound{PeerID: peer, Text: msg.Text, Files: files, Extras: extras}
		if err := c.rt.HandleInbound(r.Context(), c, in); err != nil {
			log.Printf("webhook %s: handle inbound: %v", c.cfg.Name, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "handle"})
			return
		}
		if key != "" {
			mu.Lock()
			seenAt[key] = time.Now()
			mu.Unlock()
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func (c *Channel) resolveAttachments(ctx context.Context, hc *http.Client, atts []Attachment) ([]channel.InboundFile, error) {
	files := make([]channel.InboundFile, 0, len(atts))
	for _, a := range atts {
		var data []byte
		switch {
		case a.ContentBase64 != "":
			b, err := base64.StdEncoding.DecodeString(a.ContentBase64)
			if err != nil {
				return nil, err
			}
			data = b
		case a.URL != "":
			// Only http/https are fetched; this fails early on file://,
			// gopher://, etc. (http.Client would reject them anyway, but an
			// explicit check gives a clear, dial-free error).
			if u, err := urlpkg.Parse(a.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return nil, fmt.Errorf("webhook: attachment URL %q must be an http(s) URL", a.URL)
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := hc.Do(req)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				_ = resp.Body.Close()
				return nil, fmt.Errorf("webhook: fetch %q returned status %d", a.URL, resp.StatusCode)
			}
			b, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
			_ = resp.Body.Close()
			if err != nil {
				return nil, err
			}
			data = b
		default:
			continue
		}
		files = append(files, channel.InboundFile{Name: a.Name, MIME: a.MIME, Data: data})
	}
	return files, nil
}

// newFetchClient builds the http.Client used to download attachment URLs. Its
// DialContext rejects connections to loopback/private/link-local/unspecified
// IPs (SSRF protection). The check runs on the post-DNS TCP peer address
// (conn.RemoteAddr), so a public hostname that resolves to an internal IP is
// blocked too; wrapping the Transport dialer also covers every redirect hop.
func newFetchClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			tcpAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
			if !ok || isBlockedIP(tcpAddr.IP) {
				_ = conn.Close()
				return nil, fmt.Errorf("webhook: attachment URL target %s is not allowed", address)
			}
			return conn, nil
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
	return &http.Client{Timeout: 15 * time.Second, Transport: transport}
}

// isBlockedIP reports whether ip is an address adapter-supplied attachment
// URLs must never be allowed to reach: loopback, private, link-local, the
// unspecified address, shared/CGNAT space, benchmarking range, or multicast.
func isBlockedIP(ip net.IP) bool {
	switch {
	case ip == nil:
		return true
	case ip.IsLoopback():
		// 127.0.0.0/8, ::1
		return true
	case ip.IsPrivate():
		// 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7
		return true
	case ip.IsLinkLocalUnicast():
		// 169.254.0.0/16 (incl. cloud metadata 169.254.169.254), fe80::/10
		return true
	case ip.IsLinkLocalMulticast(), ip.IsMulticast():
		// 224.0.0.0/24 link-local multicast, 224.0.0.0/4 multicast, ff00::/8
		return true
	case ip.IsUnspecified():
		// 0.0.0.0, ::
		return true
	case isCGNAT(ip), isBenchmarking(ip):
		// 100.64.0.0/10 (carrier-grade/shared), 198.18.0.0/15 (benchmarking)
		return true
	}
	return false
}

// isCGNAT reports whether ip is in 100.64.0.0/10 (RFC 6598 shared address
// space), often reachable inside carrier/container networks.
func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
}

// isBenchmarking reports whether ip is in 198.18.0.0/15 (RFC 2544).
func isBenchmarking(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
