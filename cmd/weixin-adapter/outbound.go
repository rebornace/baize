package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
	"github.com/rebornace/baize/internal/webhooksig"
)

const outboundBodyLimit = 10 << 20

type outboundPayload struct {
	Kind string `json:"kind"`
	Peer struct {
		ID string `json:"id"`
	} `json:"peer"`
	Text         string `json:"text"`
	ContextToken string `json:"context_token"`
	Media        []struct {
		Name          string `json:"name"`
		MIME          string `json:"mime"`
		ContentBase64 string `json:"content_base64"`
	} `json:"media"`
}

// handleOutbound receives baize -> adapter messages. Text is sent via
// SendMessage; image/* media via SendImage, other files via SendFile (real
// CDN upload). Media failures are logged but do not fail the request (text is
// already delivered; outbound must not interrupt the run).
func (a *Adapter) handleOutbound(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, outboundBodyLimit))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
		return
	}
	if err := webhooksig.Verify(a.secret, r.Header.Get("X-Baize-Channel-Timestamp"), body,
		r.Header.Get("X-Baize-Channel-Signature"), time.Now(), adminSkew); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}
	var p outboundPayload
	if err := json.Unmarshal(body, &p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	peer := strings.TrimSpace(p.Peer.ID)
	if peer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer.id required"})
		return
	}
	token := a.currentToken()
	if token == "" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "login_required"})
		return
	}

	if strings.TrimSpace(p.Text) != "" {
		if err := a.ilink.SendMessage(r.Context(), token, weixinlink.OutboundMessage{
			ToUserID:     peer,
			Text:         p.Text,
			ContextToken: p.ContextToken,
		}); err != nil {
			log.Printf("weixin-adapter: outbound text: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "send failed"})
			return
		}
	}
	for _, m := range p.Media {
		data, derr := base64.StdEncoding.DecodeString(m.ContentBase64)
		if derr != nil {
			log.Printf("weixin-adapter: outbound media %s bad base64: %v", m.Name, derr)
			continue
		}
		var serr error
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.MIME)), "image/") {
			serr = a.ilink.SendImage(r.Context(), token, peer, m.Name, m.MIME, data, p.ContextToken)
		} else {
			serr = a.ilink.SendFile(r.Context(), token, peer, m.Name, m.MIME, data, p.ContextToken)
		}
		if serr != nil {
			log.Printf("weixin-adapter: outbound media %s: %v", m.Name, serr)
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
