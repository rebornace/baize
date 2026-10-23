// Command im-adapter-dingtalk is a stdlib-only DingTalk→baize reference adapter.
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	hTimestamp = "X-Baize-Channel-Timestamp"
	hSignature = "X-Baize-Channel-Signature"
	hProtocol  = "X-Baize-Protocol"
	maxSkew    = 300 * time.Second
)

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func verify(secret, timestamp string, body []byte, sig string) bool {
	return hmac.Equal([]byte(sign(secret, timestamp, body)), []byte(sig))
}

func main() {
	baize := flag.String("baize", env("BAIZE_INBOUND_URL", "http://127.0.0.1:8080/v0/channels/dingtalk/inbound"), "baize inbound webhook url")
	secret := flag.String("secret", env("CHANNEL_SECRET", "dev-secret"), "shared HMAC secret")
	addr := flag.String("addr", ":9102", "listen address")
	flag.Parse()

	http.HandleFunc("/outbound", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		tsStr := r.Header.Get(hTimestamp)
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil {
			http.Error(w, "bad timestamp", http.StatusUnauthorized)
			return
		}
		if skew := time.Since(time.Unix(ts, 0)); skew > maxSkew || skew < -maxSkew {
			http.Error(w, "stale timestamp", http.StatusUnauthorized)
			return
		}
		if !verify(*secret, tsStr, body, r.Header.Get(hSignature)) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		var msg map[string]any
		_ = json.Unmarshal(body, &msg)
		log.Printf("[baize->dingtalk-adapter] %v", msg)
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/dingtalk/callback", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		peer, text, idem, err := mapDingTalkCallback(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := forwardInbound(*baize, *secret, peer, text, idem); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		fmt.Fprintln(w, "ok")
	})

	log.Printf("im-adapter-dingtalk listening on %s (baize=%s)", *addr, *baize)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func mapDingTalkCallback(raw []byte) (peer, text, idem string, err error) {
	var env map[string]any
	if err = json.Unmarshal(raw, &env); err != nil {
		return "", "", "", fmt.Errorf("json: %w", err)
	}
	peer, _ = env["senderStaffId"].(string)
	if peer == "" {
		peer, _ = env["senderId"].(string)
	}
	idem, _ = env["msgId"].(string)
	if textObj, _ := env["text"].(map[string]any); textObj != nil {
		text, _ = textObj["content"].(string)
	}
	if text == "" {
		text, _ = env["content"].(string)
	}
	if peer == "" || text == "" {
		return "", "", "", fmt.Errorf("peer and text required after mapping")
	}
	return peer, text, idem, nil
}

func forwardInbound(baizeURL, secret, peer, text, idem string) error {
	inbound := map[string]any{
		"event": "message",
		"peer":  map[string]any{"id": peer, "name": peer},
		"text":  text,
	}
	if idem != "" {
		inbound["idempotency_key"] = idem
	}
	body, _ := json.Marshal(inbound)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequest(http.MethodPost, baizeURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(hProtocol, "v0")
	req.Header.Set(hTimestamp, ts)
	req.Header.Set(hSignature, sign(secret, ts, body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("baize %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
