// Command mockllm serves a tiny OpenAI-compatible /v1/chat/completions for MOBILE-RT demos.
//
//	go run ./cmd/mobilebind/mockllm -addr 127.0.0.1:18080
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18080", "listen address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		user := ""
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" {
				user = req.Messages[i].Content
				break
			}
		}
		reply := "MOBILE-RT mock: 收到「" + truncate(user, 80) + "」。本回复由设备侧编排直连本 mock，未经过 baize serve。"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": reply}},
			},
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("mockllm listening on http://%s/v1 (MOBILE-RT demo)", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
