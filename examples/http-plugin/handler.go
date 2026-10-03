package httppluginex

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

type ticket struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type server struct {
	mu      sync.Mutex
	tickets []ticket
}

func NewHandler() http.Handler {
	s := &server{tickets: []ticket{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.requireProtocol(s.handleHealthz))
	mux.HandleFunc("GET /v0/tools", s.requireProtocol(s.handleListTools))
	mux.HandleFunc("POST /v0/tools/{name}/invoke", s.requireProtocol(s.handleInvoke))
	mux.HandleFunc("GET /tickets", s.handleListTickets)
	return mux
}

func (s *server) requireProtocol(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Baize-Protocol") != "v0" {
			writeError(w, http.StatusBadRequest, "protocol_unsupported", "X-Baize-Protocol must be v0")
			return
		}
		next(w, r)
	}
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleListTools(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"tools": []map[string]any{
			{
				"name":        "echo",
				"description": "Echo arguments back in content",
				"input_schema": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			{
				"name":        "create_ticket",
				"description": "Create a ticket (writes to in-memory store)",
				"input_schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{"type": "string"},
					},
					"required": []string{"title"},
				},
			},
			{
				"name":        "login",
				"description": "Demo login; returns accessToken for Baize capture",
				"input_schema": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
	})
}

func (s *server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "failed to read body")
		return
	}
	var req struct {
		Arguments map[string]any `json:"arguments"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid json")
			return
		}
	}
	if req.Arguments == nil {
		req.Arguments = map[string]any{}
	}

	switch name {
	case "login":
		writeJSON(w, http.StatusOK, map[string]any{
			"content": map[string]any{
				"accessToken": "demo-plugin-token",
				"email":       "demo@example.com",
			},
			"is_error": false,
		})
		return
	case "echo":
		writeJSON(w, http.StatusOK, map[string]any{
			"content":  req.Arguments,
			"is_error": false,
		})
	case "create_ticket":
		title, _ := req.Arguments["title"].(string)
		if title == "" {
			writeJSON(w, http.StatusOK, map[string]any{
				"content":  map[string]any{"message": "title required"},
				"is_error": true,
			})
			return
		}
		t := ticket{
			ID:    "ticket_" + uuid.NewString(),
			Title: title,
		}
		s.mu.Lock()
		s.tickets = append(s.tickets, t)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"content":  map[string]any{"id": t.ID, "title": t.Title},
			"is_error": false,
		})
	default:
		writeError(w, http.StatusNotFound, "not_found", "unknown tool: "+name)
	}
}

func (s *server) handleListTickets(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]ticket, len(s.tickets))
	copy(list, s.tickets)
	writeJSON(w, http.StatusOK, list)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
