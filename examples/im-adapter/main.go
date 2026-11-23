// Command im-adapter is a minimal, stdlib-only reference adapter for the baize
// out-of-process webhook channel protocol. It:
//   - serves POST /outbound to receive messages FROM baize (verifies HMAC),
//   - serves POST /send?peer=<id>&text=<msg> to simulate an inbound IM message,
//     signing it and POSTing to baize's inbound webhook.
//
// Configure via flags: -baize (inbound url), -secret (shared HMAC), -addr.
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

	// maxSkew is the allowed clock difference between baize and the adapter.
	// Requests whose timestamp is older or newer than this are rejected to
	// prevent replay of captured outbound requests.
	maxSkew = 300 * time.Second
)

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func verify(secret, timestamp string, body []byte, sig string) bool {
	want := sign(secret, timestamp, body)
	return hmac.Equal([]byte(want), []byte(sig))
}

func main() {
	baize := flag.String("baize", env("BAIZE_INBOUND_URL", "http://127.0.0.1:8080/v0/channels/demo/inbound"), "baize inbound webhook url")
	secret := flag.String("secret", env("CHANNEL_SECRET", "dev-secret"), "shared HMAC secret")
	addr := flag.String("addr", ":9100", "listen address")
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
		log.Printf("[baize->adapter] %v", msg)
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		peer := r.URL.Query().Get("peer")
		text := r.URL.Query().Get("text")
		if peer == "" || text == "" {
			http.Error(w, "peer and text required", http.StatusBadRequest)
			return
		}
		inbound := map[string]any{
			"event": "message",
			"peer":  map[string]any{"id": peer, "name": peer},
			"text":  text,
		}
		body, _ := json.Marshal(inbound)
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req, _ := http.NewRequest(http.MethodPost, *baize, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(hProtocol, "v0")
		req.Header.Set(hTimestamp, ts)
		req.Header.Set(hSignature, sign(*secret, ts, body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		fmt.Fprintf(w, "baize responded %d\n", resp.StatusCode)
	})

	log.Printf("im-adapter listening on %s (baize=%s)", *addr, *baize)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
