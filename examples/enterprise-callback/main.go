// Package enterprisecallback is a minimal reference HTTP server for Baize §4.3
// enterprise execution callbacks.
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"content":  content,
		"is_error": false,
	})
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
