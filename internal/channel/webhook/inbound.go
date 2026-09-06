package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
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

// inboundHandler returns the http.Handler for POST /v0/channels/{name}/inbound.
func (c *Channel) inboundHandler() http.Handler {
	var (
		mu      sync.Mutex
		seenKey = map[string]bool{}
	)
	fetch := &http.Client{Timeout: 15 * time.Second}
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
		// Mandatory inbound allowlist (when configured).
		if len(c.cfg.Allowlist) > 0 && !c.cfg.Allowlist[peer] {
			log.Printf("webhook %s: peer %q not in allowlist; ignored", c.cfg.Name, peer)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		// Best-effort idempotency (at-least-once delivery).
		if key := msg.IdempotencyKey; key != "" {
			mu.Lock()
			dup := seenKey[key]
			if !dup {
				seenKey[key] = true
			}
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
		extras := map[string]string{"account": c.cfg.Account}
		if msg.ContextToken != "" {
			extras["context_token"] = msg.ContextToken
		}
		in := channel.Inbound{PeerID: peer, Text: msg.Text, Files: files, Extras: extras}
		if err := c.rt.HandleInbound(r.Context(), c, in); err != nil {
			log.Printf("webhook %s: handle inbound: %v", c.cfg.Name, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "handle"})
			return
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
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := hc.Do(req)
			if err != nil {
				return nil, err
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
