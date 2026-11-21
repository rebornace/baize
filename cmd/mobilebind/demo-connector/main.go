// Command demo-connector serves a self-contained MOBILE-RT demo:
// a remote config document, an OpenAPI 3 spec, and a tiny ticket API that
// requires the demo auth header.
//
//	go run ./cmd/mobilebind/demo-connector -addr 127.0.0.1:18081
//
// Then set the shell's "Remote config URL" to:
//
//	Android emulator: http://10.0.2.2:18081/config.json
//	iOS simulator:    http://127.0.0.1:18081/config.json
//
// The config derives absolute URLs from the request Host, so the same process
// works from an emulator, a simulator, or a phone on the LAN (bind 0.0.0.0).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const demoSpec = `openapi: 3.0.3
info:
  title: Baize Mobile Demo Connector
  version: "1.0"
paths:
  /tickets:
    get:
      operationId: list_tickets
      summary: List demo tickets
      description: Returns seeded demo tickets; filter by status or cap the count.
      parameters:
        - name: status
          in: query
          required: false
          description: Filter by status (open or closed)
          schema:
            type: string
        - name: limit
          in: query
          required: false
          description: Maximum number of tickets to return
          schema:
            type: integer
      responses:
        "200":
          description: ok
    post:
      operationId: create_ticket
      summary: Create a demo ticket
      description: Creates one demo ticket. Writes require foreground approval in the shell.
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - title
              properties:
                title:
                  type: string
                  description: Short ticket title
                priority:
                  type: string
                  description: high, normal or low (default normal)
      responses:
        "201":
          description: created
  /tickets/{id}:
    get:
      operationId: get_ticket
      summary: Get one demo ticket
      parameters:
        - name: id
          in: path
          required: true
          description: Ticket id, for example T-1001
          schema:
            type: string
      responses:
        "200":
          description: ok
`

type ticket struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
}

type handlerOptions struct {
	apiKey  string
	llmBase string
	llmPort string
	model   string
}

type server struct {
	opts    handlerOptions
	mu      sync.Mutex
	tickets []ticket
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18081", "listen address (use 0.0.0.0:18081 for a physical device)")
	apiKey := flag.String("api-key", "demo-key", "required X-API-Key value (empty disables auth)")
	llmBase := flag.String("llm-base", "", "override api_base in config.json (default: same host, -llm-port)")
	llmPort := flag.String("llm-port", "18080", "host port for the mock LLM when -llm-base is empty")
	model := flag.String("model", "mock", "model name written to config.json")
	flag.Parse()

	h := newHandler(handlerOptions{apiKey: *apiKey, llmBase: *llmBase, llmPort: *llmPort, model: *model})
	log.Printf("demo connector on http://%s (config: /config.json, spec: /openapi.yaml)", *addr)
	if err := http.ListenAndServe(*addr, h); err != nil {
		log.Fatal(err)
	}
}

func newHandler(opts handlerOptions) http.Handler {
	s := &server{
		opts: opts,
		tickets: []ticket{
			{ID: "T-1001", Title: "VPN 无法连接", Status: "open", Priority: "high"},
			{ID: "T-1002", Title: "报销单待审批", Status: "open", Priority: "normal"},
			{ID: "T-1003", Title: "会议室投影已修复", Status: "closed", Priority: "low"},
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /config.json", s.handleConfig)
	mux.HandleFunc("GET /openapi.yaml", s.handleSpec)
	mux.HandleFunc("GET /api/tickets", s.requireKey(s.handleList))
	mux.HandleFunc("POST /api/tickets", s.requireKey(s.handleCreate))
	mux.HandleFunc("GET /api/tickets/{id}", s.requireKey(s.handleGet))
	return mux
}

func (s *server) requireKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.opts.apiKey != "" && r.Header.Get("X-API-Key") != s.opts.apiKey {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *server) handleConfig(w http.ResponseWriter, r *http.Request) {
	origin := requestOrigin(r)
	readonly := map[string]any{
		"base_url":     origin + "/api",
		"spec_url":     origin + "/openapi.yaml",
		"allow_writes": true,
	}
	if s.opts.apiKey != "" {
		readonly["auth_header"] = "X-API-Key"
		readonly["auth_value"] = s.opts.apiKey
	}
	llmBase := s.opts.llmBase
	if llmBase == "" {
		llmBase = "http://" + net.JoinHostPort(hostnameOf(r.Host), s.opts.llmPort) + "/v1"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"api_base":     llmBase,
		"model":        s.opts.model,
		"readonly_api": readonly,
	})
}

func (s *server) handleSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write([]byte(demoSpec))
}

func (s *server) handleList(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	out := make([]ticket, 0, len(s.tickets))
	for _, t := range s.tickets {
		if status != "" && strings.ToLower(t.Status) != status {
			continue
		}
		out = append(out, t)
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 && n < len(out) {
			out = out[:n]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tickets": out, "count": len(out)})
}

func (s *server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title    string `json:"title"`
		Priority string `json:"priority"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json body"})
		return
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "title is required"})
		return
	}
	priority := strings.ToLower(strings.TrimSpace(input.Priority))
	if priority == "" {
		priority = "normal"
	}
	s.mu.Lock()
	created := ticket{
		ID:       fmt.Sprintf("T-%d", 1001+len(s.tickets)),
		Title:    title,
		Status:   "open",
		Priority: priority,
	}
	s.tickets = append(s.tickets, created)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, created)
}

func (s *server) handleGet(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := r.PathValue("id")
	for _, t := range s.tickets {
		if t.ID == id {
			writeJSON(w, http.StatusOK, t)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"error": "ticket not found"})
}

func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return scheme + "://" + host
}

func hostnameOf(hostport string) string {
	if hostport == "" {
		return "127.0.0.1"
	}
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
