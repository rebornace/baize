package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/rebornace/baize/internal/conversation"
)

const (
	// Defaults align with runtimecfg / bootstrap when Settings is nil.
	defaultActiveGoalMaxDepth     = 8
	defaultActiveGoalMaxTextRunes = 500
)

type goalHTTPResponse struct {
	ConversationID string                   `json:"conversation_id"`
	Revision       int                      `json:"revision"`
	Frames         []conversation.GoalFrame `json:"frames"`
	Root           *conversation.GoalFrame  `json:"root,omitempty"`
	Tip            *conversation.GoalFrame  `json:"tip,omitempty"`
}

func (s *Server) activeGoalEnabled() bool {
	if s.Settings == nil {
		return true
	}
	return s.Settings.Knobs().ActiveGoalEnabled
}

func (s *Server) activeGoalMaxDepth() int {
	maxDepth := defaultActiveGoalMaxDepth
	if s.Settings != nil {
		if k := s.Settings.Knobs().ActiveGoalMaxDepth; k > 0 {
			maxDepth = k
		}
	}
	return maxDepth
}

func (s *Server) activeGoalMaxTextRunes() int {
	maxRunes := defaultActiveGoalMaxTextRunes
	if s.Settings != nil {
		if k := s.Settings.Knobs().ActiveGoalMaxTextRunes; k > 0 {
			maxRunes = k
		}
	}
	return maxRunes
}

func validateActiveGoalText(text string, maxRunes int) error {
	if text == "" {
		return fmt.Errorf("goal text is required")
	}
	if utf8.RuneCountInString(text) > maxRunes {
		return fmt.Errorf("goal text is too long")
	}
	return nil
}

func (s *Server) requireMessageStore(w http.ResponseWriter) bool {
	if s.Messages == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "message store not configured")
		return false
	}
	return true
}

func (s *Server) requireActiveGoalWrites(w http.ResponseWriter) bool {
	if !s.activeGoalEnabled() {
		writeError(w, http.StatusForbidden, "forbidden", "active goal is disabled")
		return false
	}
	return true
}

func (s *Server) loadGoalStack(convID string) conversation.GoalStack {
	stack := conversation.GoalStack{ConversationID: convID}
	if s.Messages != nil {
		if got, ok := s.Messages.GetGoalStack(convID); ok {
			stack = got
		}
	}
	if stack.Frames == nil {
		stack.Frames = []conversation.GoalFrame{}
	}
	return stack
}

func goalHTTPView(stack conversation.GoalStack) goalHTTPResponse {
	out := goalHTTPResponse{
		ConversationID: stack.ConversationID,
		Revision:       stack.Revision,
		Frames:         stack.Frames,
	}
	if root, ok := conversation.Root(stack); ok {
		r := root
		out.Root = &r
	}
	if tip, ok := conversation.Tip(stack); ok {
		t := tip
		out.Tip = &t
	}
	return out
}

func (s *Server) writeGoalStack(w http.ResponseWriter, stack conversation.GoalStack) {
	writeJSON(w, http.StatusOK, goalHTTPView(stack))
}

func (s *Server) persistGoalStack(w http.ResponseWriter, stack conversation.GoalStack) bool {
	if err := s.Messages.UpsertGoalStack(stack); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return false
	}
	return true
}

func writeGoalOpError(w http.ResponseWriter, err error) {
	writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
}

func (s *Server) handleGetGoal(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	stack := s.loadGoalStack(convID)
	stack.ConversationID = convID
	s.writeGoalStack(w, stack)
}

func (s *Server) handlePutGoal(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if !s.requireMessageStore(w) || !s.requireActiveGoalWrites(w) {
		return
	}
	var body struct {
		Text     string   `json:"text"`
		Details  string   `json:"details"`
		Criteria []string `json:"criteria"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	text := strings.TrimSpace(body.Text)
	details := strings.TrimSpace(body.Details)
	if details == "" {
		text, details = conversation.SplitGoalTitleAndDetails(text)
	}
	if err := validateActiveGoalText(text, s.activeGoalMaxTextRunes()); err != nil {
		writeGoalOpError(w, err)
		return
	}
	stack := s.loadGoalStack(convID)
	next, err := conversation.SetRootDetailed(stack, convID, text, details, body.Criteria, conversation.GoalSourceUser)
	if err != nil {
		writeGoalOpError(w, err)
		return
	}
	if !s.persistGoalStack(w, next) {
		return
	}
	s.writeGoalStack(w, next)
}

func (s *Server) handlePostGoalPush(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if !s.requireMessageStore(w) || !s.requireActiveGoalWrites(w) {
		return
	}
	var body struct {
		Text     string   `json:"text"`
		Details  string   `json:"details"`
		Criteria []string `json:"criteria"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	text := strings.TrimSpace(body.Text)
	details := strings.TrimSpace(body.Details)
	if details == "" {
		text, details = conversation.SplitGoalTitleAndDetails(text)
	}
	if err := validateActiveGoalText(text, s.activeGoalMaxTextRunes()); err != nil {
		writeGoalOpError(w, err)
		return
	}
	stack := s.loadGoalStack(convID)
	next, err := conversation.PushDetailed(stack, text, details, body.Criteria, conversation.GoalSourceUser, s.activeGoalMaxDepth())
	if err != nil {
		writeGoalOpError(w, err)
		return
	}
	if !s.persistGoalStack(w, next) {
		return
	}
	s.writeGoalStack(w, next)
}

func (s *Server) handlePostGoalPop(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if !s.requireMessageStore(w) || !s.requireActiveGoalWrites(w) {
		return
	}
	discardBody(r)
	stack := s.loadGoalStack(convID)
	next, err := conversation.Pop(stack)
	if err != nil {
		writeGoalOpError(w, err)
		return
	}
	if !s.persistGoalStack(w, next) {
		return
	}
	s.writeGoalStack(w, next)
}

func (s *Server) handlePostGoalStop(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if !s.requireMessageStore(w) || !s.requireActiveGoalWrites(w) {
		return
	}
	discardBody(r)
	stack := s.loadGoalStack(convID)
	next, err := conversation.Stop(stack)
	if err != nil {
		writeGoalOpError(w, err)
		return
	}
	if !s.persistGoalStack(w, next) {
		return
	}
	s.writeGoalStack(w, next)
}

func (s *Server) handlePostGoalResume(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if !s.requireMessageStore(w) || !s.requireActiveGoalWrites(w) {
		return
	}
	discardBody(r)
	stack := s.loadGoalStack(convID)
	next, err := conversation.Resume(stack)
	if err != nil {
		writeGoalOpError(w, err)
		return
	}
	if !s.persistGoalStack(w, next) {
		return
	}
	s.writeGoalStack(w, next)
}

func (s *Server) handlePatchGoalTip(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	if !s.requireMessageStore(w) || !s.requireActiveGoalWrites(w) {
		return
	}
	var body struct {
		Text     string    `json:"text"`
		Details  *string   `json:"details"`
		Criteria *[]string `json:"criteria"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	var criteria []string
	if body.Criteria != nil {
		criteria = *body.Criteria
	}
	text := strings.TrimSpace(body.Text)
	if text != "" {
		if err := validateActiveGoalText(text, s.activeGoalMaxTextRunes()); err != nil {
			writeGoalOpError(w, err)
			return
		}
	}
	var detailsPtr *string
	if body.Details != nil {
		d := strings.TrimSpace(*body.Details)
		detailsPtr = &d
	}
	stack := s.loadGoalStack(convID)
	next, err := conversation.PatchTipDetailed(stack, text, criteria, detailsPtr)
	if err != nil {
		writeGoalOpError(w, err)
		return
	}
	if !s.persistGoalStack(w, next) {
		return
	}
	s.writeGoalStack(w, next)
}

func discardBody(r *http.Request) {
	if r.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, r.Body)
}
