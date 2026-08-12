package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// Runner executes a run synchronously (implemented by run.Engine).
type Runner interface {
	Execute(ctx context.Context, runID string, ag agent.Def, input string) error
}

type Server struct {
	Store    *store.Store
	Registry *tool.Registry
	Runner   Runner
	mux      *http.ServeMux
}

func NewServer(st *store.Store, reg *tool.Registry, runner Runner) *Server {
	s := &Server{
		Store:    st,
		Registry: reg,
		Runner:   runner,
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("PUT /v0/agents/{id}", s.handlePutAgent)
	s.mux.HandleFunc("PUT /v0/connectors/{id}", s.handlePutConnector)
	s.mux.HandleFunc("POST /v0/runs", s.handlePostRun)
	s.mux.HandleFunc("GET /v0/runs/{id}/events", s.handleGetEvents)
	s.mux.HandleFunc("GET /v0/runs/{id}", s.handleGetRun)
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Error apiError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePutAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing agent id")
		return
	}
	var body struct {
		System string `json:"system"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	s.Store.UpsertAgent(store.Agent{ID: id, System: body.System})
	writeJSON(w, http.StatusOK, store.Agent{ID: id, System: body.System})
}

func (s *Server) handlePutConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing connector id")
		return
	}
	var body struct {
		Type    string `json:"type"`
		Spec    string `json:"spec"`
		BaseURL string `json:"base_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Type == "" {
		body.Type = "openapi"
	}
	if body.Type != "openapi" {
		writeError(w, http.StatusBadRequest, "invalid_request", "unsupported connector type")
		return
	}
	if body.Spec == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "spec is required")
		return
	}

	routes, err := openapi.LoadTools(body.Spec)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_spec", err.Error())
		return
	}

	c := store.Connector{
		ID:      id,
		Type:    body.Type,
		Spec:    body.Spec,
		BaseURL: body.BaseURL,
	}
	s.Store.UpsertConnector(c)

	inv := &openapi.Invoker{BaseURL: body.BaseURL, Tools: routes}
	for _, route := range routes {
		route := route
		name := route.Name
		s.Registry.RegisterSpec(llm.ToolSpec{
			Name:        route.Name,
			Description: route.Description,
			InputSchema: route.InputSchema,
		}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			res, err := inv.Invoke(ctx, name, args)
			if err != nil {
				return nil, true, err
			}
			return res.Content, res.IsError, nil
		})
	}

	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handlePostRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID string `json:"agent_id"`
		Input   string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if strings.TrimSpace(body.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}

	ag, err := s.Store.GetAgent(body.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}

	run, err := s.Store.CreateRun(body.AgentID, body.Input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	def := agent.Def{ID: ag.ID, System: ag.System}
	_ = s.Runner.Execute(r.Context(), run.ID, def, body.Input)

	updated, err := s.Store.GetRun(run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": updated.ID,
		"status": updated.Status,
	})
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	evs, err := s.Store.ListEvents(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if evs == nil {
		evs = []store.Event{}
	}
	writeJSON(w, http.StatusOK, evs)
}
