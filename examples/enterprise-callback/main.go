// Package enterprisecallback is a minimal reference HTTP server for Baize §4.3
// enterprise execution callbacks.
//
// When Runtime is configured with runtime.public_base_url and the invoke carries
// run_id, the POST body includes callback_urls.event; this example POSTs a
// progress note back to that URL asynchronously.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

const headerProtocol = "X-Baize-Protocol"

func main() {
	addr := ":19100"
	if v := os.Getenv("BAIZE_ENTERPRISE_CALLBACK_ADDR"); v != "" {
		addr = v
	}
	log.Printf("enterprise-callback listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, newHandler()))
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/execute", handleExecute)
	return mux
}

type executeBody struct {
	Tool            string         `json:"tool"`
	Arguments       map[string]any `json:"arguments"`
	RunID           string         `json:"run_id"`
	AgentID         string         `json:"agent_id"`
	IdempotencyKey  string         `json:"idempotency_key"`
	CallbackURLs    struct {
		Event string `json:"event"`
	} `json:"callback_urls"`
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(r.Header.Get(headerProtocol)) != "v0" {
		writeError(w, http.StatusBadRequest, "protocol_unsupported", "expected X-Baize-Protocol: v0")
		return
	}
	var body executeBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json")
		return
	}
	content := map[string]any{
		"tool":            body.Tool,
		"arguments":       body.Arguments,
		"run_id":          body.RunID,
		"idempotency_key": body.IdempotencyKey,
	}
	if body.Tool == "create_ticket" {
		title, _ := body.Arguments["title"].(string)
		content["id"] = "T-1"
		content["title"] = title
	}
	if eventURL := strings.TrimSpace(body.CallbackURLs.Event); eventURL != "" {
		go postCallbackEvent(eventURL)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"content":  content,
		"is_error": false,
	})
}

func postCallbackEvent(eventURL string) {
	req, err := http.NewRequest(
		http.MethodPost,
		eventURL,
		strings.NewReader(`{"type":"event","name":"enterprise.note","payload":{"ok":true}}`),
	)
	if err != nil {
		log.Printf("callback_urls.event: build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerProtocol, "v0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("callback_urls.event: post: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("callback_urls.event: status=%d", resp.StatusCode)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
