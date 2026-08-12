package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/ui"
)

// Runner executes and resumes runs (implemented by run.Engine).
type Runner interface {
	Execute(ctx context.Context, runID string, ag agent.Def, input string) error
	ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error
}

type Server struct {
	Store          store.Store
	Registry       *tool.Registry
	Runner         Runner
	Identities     identity.Store
	Messages       conversation.Store // optional; nil = no message persistence
	DefaultAgentID string
	mux            *http.ServeMux
}

func NewServer(st store.Store, reg *tool.Registry, runner Runner) *Server {
	s := &Server{
		Store:      st,
		Registry:   reg,
		Runner:     runner,
		Identities: identity.NewMemoryStore(),
		mux:        http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.Handle("/ui/", ui.Handler())
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /v0/ui-config", s.handleUIConfig)
	s.mux.HandleFunc("PUT /v0/agents/{id}", s.handlePutAgent)
	s.mux.HandleFunc("PUT /v0/connectors/{id}", s.handlePutConnector)
	s.mux.HandleFunc("GET /v0/connectors/{id}", s.handleGetConnector)
	s.mux.HandleFunc("GET /v0/tools", s.handleGetTools)
	s.mux.HandleFunc("POST /v0/runs", s.handlePostRun)
	s.mux.HandleFunc("POST /v0/runs/{id}/resume", s.handlePostResume)
	s.mux.HandleFunc("GET /v0/runs/{id}/events", s.handleGetEvents)
	s.mux.HandleFunc("GET /v0/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("GET /v0/conversations/{id}/identities", s.handleListIdentities)
	s.mux.HandleFunc("POST /v0/conversations/{id}/identities/{iid}/default", s.handleSetDefaultIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities/{iid}", s.handleDeleteIdentity)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/identities", s.handleClearIdentities)
	s.mux.HandleFunc("GET /v0/conversations/{id}/messages", s.handleListMessages)
	s.mux.HandleFunc("DELETE /v0/conversations/{id}/messages", s.handleClearMessages)
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

func (s *Server) handleUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"agent_id": s.DefaultAgentID,
	})
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
		Type            string   `json:"type"`
		Spec            string   `json:"spec"`
		BaseURL         string   `json:"base_url"`
		RequireApproval []string `json:"require_approval"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if body.Type == "" {
		body.Type = "openapi"
	}
	if body.Spec == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "spec is required")
		return
	}

	c, infos, err := openapi.RegisterConnector(
		s.Store, s.Registry, id, body.Type, body.Spec, body.BaseURL, body.RequireApproval,
	)
	if err != nil {
		if errors.Is(err, openapi.ErrToolConflict) {
			writeError(w, http.StatusConflict, "tool_conflict", err.Error())
			return
		}
		if errors.Is(err, openapi.ErrInvalidSpec) {
			writeError(w, http.StatusBadRequest, "invalid_spec", err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":               c.ID,
		"type":             c.Type,
		"spec":             c.Spec,
		"base_url":         c.BaseURL,
		"require_approval": c.RequireApproval,
		"tools":            infos,
	})
}

func (s *Server) handleGetConnector(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	tools := make([]tool.Info, 0)
	for _, info := range s.Registry.List() {
		if info.ConnectorID == id {
			tools = append(tools, info)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":               c.ID,
		"type":             c.Type,
		"spec":             c.Spec,
		"base_url":         c.BaseURL,
		"require_approval": c.RequireApproval,
		"tools":            tools,
	})
}

func (s *Server) handleGetTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tools": s.Registry.List()})
}

func (s *Server) handlePostRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AgentID        string `json:"agent_id"`
		Input          string `json:"input"`
		ConversationID string `json:"conversation_id"`
		IdentityID     string `json:"identity_id"`
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

	conv := strings.TrimSpace(body.ConversationID)
	if conv == "" {
		conv = "conv_" + uuid.NewString()
	}

	runRec, err := s.Store.CreateRun(store.CreateRunInput{
		AgentID:        body.AgentID,
		Input:          body.Input,
		ConversationID: conv,
		IdentityID:     strings.TrimSpace(body.IdentityID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Persist the user turn before executing so the conversation window survives
	// crashes and is visible via GET /v0/conversations/{id}/messages. Execute's
	// buildMessages dedups the trailing user input against this appended entry.
	if s.Messages != nil && conv != "" {
		_, _ = s.Messages.Append(conv, conversation.Message{
			Role:    conversation.RoleUser,
			Content: body.Input,
			RunID:   runRec.ID,
		})
	}

	def := agent.Def{ID: ag.ID, System: ag.System}
	// Persist run.started before returning so the UI never polls an empty event stream
	// while the worker is still scheduling / contending on SQLite.
	_ = s.Store.AppendEvent(runRec.ID, store.Event{Type: run.EventRunStarted})
	go func(runID, input string, def agent.Def) {
		err := s.Runner.Execute(context.Background(), runID, def, input)
		if err == nil {
			return
		}
		cur, getErr := s.Store.GetRun(runID)
		if getErr != nil || cur == nil {
			return
		}
		switch cur.Status {
		case store.StatusRunning, store.StatusQueued:
			_ = s.Store.UpdateRun(runID, store.StatusFailed, "", err.Error())
			_ = s.Store.AppendEvent(runID, store.Event{
				Type: run.EventLLMError,
				Data: map[string]any{"error": err.Error()},
			})
			// Engine may have returned before recordTerminalMessage (e.g. early
			// failure while still running). Mirror its failed-note format here.
			if s.Messages != nil && cur.ConversationID != "" {
				note := strings.TrimSpace(err.Error())
				if note == "" {
					note = "运行失败"
				} else {
					note = "运行失败：" + note
				}
				_, _ = s.Messages.Append(cur.ConversationID, conversation.Message{
					Role:    conversation.RoleSystemNote,
					Content: note,
					RunID:   runID,
				})
			}
		}
	}(runRec.ID, body.Input, def)

	updated, err := s.Store.GetRun(runRec.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":          updated.ID,
		"status":          updated.Status,
		"conversation_id": conv,
	})
}

func (s *Server) handleListIdentities(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeJSON(w, http.StatusOK, []identity.PublicView{})
		return
	}
	views := s.Identities.ListPublic(r.PathValue("id"))
	if views == nil {
		views = []identity.PublicView{}
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleSetDefaultIdentity(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	if err := s.Identities.SetDefault(r.PathValue("id"), r.PathValue("iid")); err != nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteIdentity(w http.ResponseWriter, r *http.Request) {
	if s.Identities == nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	if err := s.Identities.Delete(r.PathValue("id"), r.PathValue("iid")); err != nil {
		writeError(w, http.StatusNotFound, "identity_not_found", "identity not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleClearIdentities(w http.ResponseWriter, r *http.Request) {
	if s.Identities != nil {
		s.Identities.ClearCaptured(r.PathValue("id"))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handlePostResume(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	if runRec.Status != store.StatusWaitingHuman {
		writeError(w, http.StatusConflict, "not_waiting", "run is not waiting_human")
		return
	}

	var body struct {
		Decision string `json:"decision"`
		Comment  string `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	approve := body.Decision == "approve"
	if body.Decision != "approve" && body.Decision != "reject" {
		writeError(w, http.StatusBadRequest, "invalid_request", "decision must be approve or reject")
		return
	}

	_ = s.Runner.ContinueFromHITL(r.Context(), id, run.Decision{
		Approve: approve,
		Comment: body.Comment,
	})

	updated, err := s.Store.GetRun(id)
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
	runRec, err := s.Store.GetRun(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "run_not_found", "run not found")
		return
	}
	writeJSON(w, http.StatusOK, runRec)
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

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	if s.Messages == nil {
		writeJSON(w, http.StatusOK, []conversation.Message{})
		return
	}
	msgs := s.Messages.List(r.PathValue("id"))
	if msgs == nil {
		msgs = []conversation.Message{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) handleClearMessages(w http.ResponseWriter, r *http.Request) {
	if s.Messages != nil {
		s.Messages.Clear(r.PathValue("id"))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
