package mockticket

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Ticket struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Priority  string    `json:"priority,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type createTicketRequest struct {
	Title    string `json:"title"`
	Priority string `json:"priority,omitempty"`
}

type patchTicketRequest struct {
	Status string `json:"status"`
}

var allowedStatuses = map[string]bool{
	"open":   true,
	"closed": true,
}

type server struct {
	mu      sync.Mutex
	tickets []Ticket
}

func NewHandler() http.Handler {
	s := &server{tickets: []Ticket{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /tickets", s.handleListTickets)
	mux.HandleFunc("POST /tickets", s.handleCreateTicket)
	mux.HandleFunc("GET /tickets/{id}", s.handleGetTicket)
	mux.HandleFunc("PATCH /tickets/{id}", s.handlePatchTicket)
	return mux
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *server) handleListTickets(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Ticket, len(s.tickets))
	copy(list, s.tickets)
	writeJSON(w, http.StatusOK, list)
}

func (s *server) handleCreateTicket(w http.ResponseWriter, r *http.Request) {
	var req createTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Title == "" {
		http.Error(w, "title required", http.StatusBadRequest)
		return
	}

	ticket := Ticket{
		ID:        "ticket_" + uuid.NewString(),
		Title:     req.Title,
		Priority:  req.Priority,
		Status:    "open",
		CreatedAt: time.Now().UTC(),
	}

	s.mu.Lock()
	s.tickets = append(s.tickets, ticket)
	s.mu.Unlock()

	writeJSON(w, http.StatusCreated, ticket)
}

func (s *server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.tickets {
		if t.ID == id {
			writeJSON(w, http.StatusOK, t)
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func (s *server) handlePatchTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req patchTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if !allowedStatuses[req.Status] {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.tickets {
		if s.tickets[i].ID == id {
			s.tickets[i].Status = req.Status
			writeJSON(w, http.StatusOK, s.tickets[i])
			return
		}
	}
	http.Error(w, "not found", http.StatusNotFound)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
