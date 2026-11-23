// Package mockproject is a small project API used by the EV-PRES cross-domain
// hold-out. It mirrors examples/mock-ticket but with projects instead of tickets.
package mockproject

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type server struct {
	mu       sync.Mutex
	projects []Project
}

func NewHandler() http.Handler {
	s := &server{projects: []Project{
		{ID: "proj-1", Title: "网站改版", Status: "active", CreatedAt: time.Now().UTC()},
		{ID: "proj-2", Title: "数据迁移", Status: "active", CreatedAt: time.Now().UTC()},
	}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /projects", s.list)
	mux.HandleFunc("POST /projects", s.create)
	mux.HandleFunc("GET /projects/{id}", s.get)
	mux.HandleFunc("PATCH /projects/{id}", s.patch)
	return mux
}

func (s *server) list(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Project, len(s.projects))
	copy(out, s.projects)
	writeJSON(w, http.StatusOK, out)
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title required"})
		return
	}
	s.mu.Lock()
	p := Project{ID: "project_" + uuid.NewString(), Title: in.Title, Status: "active", CreatedAt: time.Now().UTC()}
	s.projects = append(s.projects, p)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, p)
}

func (s *server) get(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.projects {
		if p.ID == r.PathValue("id") {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *server) patch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || (in.Status != "active" && in.Status != "archived") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.projects {
		if s.projects[i].ID == r.PathValue("id") {
			s.projects[i].Status = in.Status
			writeJSON(w, http.StatusOK, s.projects[i])
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
