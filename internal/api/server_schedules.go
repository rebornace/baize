package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/schedule"
	"github.com/rebornace/baize/internal/store"
)

type scheduleItemJSON struct {
	ID             string `json:"id"`
	OwnerID        string `json:"owner_id"`
	ConversationID string `json:"conversation_id"`
	Instruction    string `json:"instruction"`
	Kind           string `json:"kind"`
	Timezone       string `json:"timezone"`
	FireAt         string `json:"fire_at,omitempty"`
	TimeOfDay      string `json:"time_of_day,omitempty"`
	Weekday        int    `json:"weekday,omitempty"`
	Status         string `json:"status"`
	NextRunAt      string `json:"next_run_at"`
	RunningRunID   string `json:"running_run_id,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	MaxRetries     int    `json:"max_retries,omitempty"`
	RetryCount     int    `json:"retry_count,omitempty"`
	BlockedReason  string `json:"blocked_reason,omitempty"`
	BlockedSince   string `json:"blocked_since,omitempty"`
	Revision       int    `json:"revision"`
}

func scheduleItemToJSON(it schedule.Item) scheduleItemJSON {
	out := scheduleItemJSON{
		ID:             it.ID,
		OwnerID:        it.OwnerID,
		ConversationID: it.ConversationID,
		Instruction:    it.Instruction,
		Kind:           it.Kind,
		Timezone:       it.Timezone,
		TimeOfDay:      it.TimeOfDay,
		Weekday:        it.Weekday,
		Status:         it.Status,
		RunningRunID:   it.RunningRunID,
		LastError:      it.LastError,
		MaxRetries:     it.MaxRetries,
		RetryCount:     it.RetryCount,
		Revision:       it.Revision,
	}
	if !it.FireAt.IsZero() {
		out.FireAt = it.FireAt.UTC().Format(time.RFC3339)
	}
	if !it.NextRunAt.IsZero() {
		out.NextRunAt = it.NextRunAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (s *Server) requireScheduleStore(w http.ResponseWriter) bool {
	if s.Schedules == nil {
		writeError(w, http.StatusServiceUnavailable, "schedules_unavailable", "schedule store not wired")
		return false
	}
	return true
}

func (s *Server) scheduleOwnerID(r *http.Request) string {
	return s.memoryOwnerID(r.Context())
}

func (s *Server) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleStore(w) {
		return
	}
	owner := s.scheduleOwnerID(r)
	items, err := s.Schedules.List(owner)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if items == nil {
		items = []schedule.Item{}
	}
	out := make([]scheduleItemJSON, 0, len(items))
	for _, it := range items {
		if it.Kind == schedule.KindFollowup {
			continue
		}
		item := scheduleItemToJSON(it)
		item.BlockedReason, item.BlockedSince = s.scheduleBlockedReason(it)
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// scheduleBlockedReason derives why an active schedule is not firing even
// though it is due. It is read-only: lookups fail soft so the list never 500s.
// The second result is when the blocking wait began (empty when unknown).
func (s *Server) scheduleBlockedReason(it schedule.Item) (string, string) {
	if it.Status != schedule.StatusActive {
		return "", ""
	}
	if s.Store == nil {
		return "", ""
	}
	runID := strings.TrimSpace(it.RunningRunID)
	if runID == "" {
		// RUN-GUARD also skips a due schedule when the conversation has an
		// active Run that this schedule did not open (a manual/scheduled turn
		// waiting for approval). Surface that instead of showing "active".
		busy, err := s.Store.HasActiveRun(it.ConversationID)
		if err == nil && busy {
			since := ""
			if waiting, werr := s.Store.WaitingHumanRun(it.ConversationID); werr == nil && waiting != nil {
				since = formatBlockedSince(run.HITLWaitingSince(s.Store, waiting))
			}
			return "conversation_busy", since
		}
		return "", ""
	}
	if runID == schedule.ClaimPending {
		return "", ""
	}
	runRec, err := s.Store.GetRun(runID)
	if err != nil || runRec == nil {
		return "run_unavailable", ""
	}
	if runRec.Status != store.StatusWaitingHuman {
		return "", ""
	}
	since := formatBlockedSince(run.HITLWaitingSince(s.Store, runRec))
	payload, err := s.Store.GetHITL(runID)
	if err != nil || payload == nil {
		return "awaiting_input", since
	}
	switch payload.Kind {
	case store.HITLKindWaitingChild:
		return "waiting_child", since
	case store.HITLKindLogin:
		return "awaiting_login", since
	default: // approval or empty kind
		return "awaiting_approval", since
	}
}

func formatBlockedSince(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func (s *Server) handlePostSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleStore(w) {
		return
	}
	var body struct {
		ConversationID string `json:"conversation_id"`
		Kind           string `json:"kind"`
		Instruction    string `json:"instruction"`
		Timezone       string `json:"timezone"`
		FireAt         string `json:"fire_at"`
		TimeOfDay      string `json:"time_of_day"`
		Weekday        *int   `json:"weekday"`
		MaxRetries     *int   `json:"max_retries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	convID := strings.TrimSpace(body.ConversationID)
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "conversation_id is required")
		return
	}
	d, err := draftFromScheduleBody(body.Kind, body.Instruction, body.Timezone, body.FireAt, body.TimeOfDay, body.Weekday)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if body.MaxRetries != nil {
		if *body.MaxRetries < 0 || *body.MaxRetries > schedule.MaxRetriesLimit {
			writeError(w, http.StatusBadRequest, "invalid_request", "max_retries must be between 0 and 5")
			return
		}
		d.MaxRetries = *body.MaxRetries
	}
	now := time.Now()
	next, err := schedule.Validate(d, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	owner := s.scheduleOwnerID(r)
	it, err := s.Schedules.Insert(schedule.Item{
		OwnerID:        owner,
		ConversationID: convID,
		Instruction:    strings.TrimSpace(d.Instruction),
		Kind:           d.Kind,
		Timezone:       d.Timezone,
		FireAt:         d.FireAt,
		TimeOfDay:      d.TimeOfDay,
		Weekday:        d.Weekday,
		Status:         schedule.StatusActive,
		NextRunAt:      next.UTC(),
		MaxRetries:     d.MaxRetries,
	})
	if err != nil {
		if errors.Is(err, schedule.ErrActiveCap) {
			writeError(w, http.StatusConflict, "schedule_active_cap", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, scheduleItemToJSON(it))
}

func draftFromScheduleBody(kind, instruction, timezone, fireAt, timeOfDay string, weekday *int) (schedule.Draft, error) {
	kind = strings.TrimSpace(kind)
	instruction = strings.TrimSpace(instruction)
	if kind == "" {
		return schedule.Draft{}, errors.New("kind is required")
	}
	if instruction == "" {
		return schedule.Draft{}, errors.New("instruction is required")
	}
	d := schedule.Draft{Kind: kind, Instruction: instruction, Timezone: strings.TrimSpace(timezone)}
	switch kind {
	case "once":
		fireAt = strings.TrimSpace(fireAt)
		if fireAt == "" {
			return schedule.Draft{}, errors.New("fire_at is required for once")
		}
		t, err := parseScheduleFireAt(fireAt)
		if err != nil {
			return schedule.Draft{}, err
		}
		d.FireAt = t
	case "daily":
		timeOfDay = strings.TrimSpace(timeOfDay)
		if timeOfDay == "" {
			return schedule.Draft{}, errors.New("time_of_day is required for daily")
		}
		d.TimeOfDay = timeOfDay
	case "weekly":
		timeOfDay = strings.TrimSpace(timeOfDay)
		if timeOfDay == "" {
			return schedule.Draft{}, errors.New("time_of_day is required for weekly")
		}
		if weekday == nil {
			return schedule.Draft{}, errors.New("weekday is required for weekly")
		}
		d.TimeOfDay = timeOfDay
		d.Weekday = *weekday
	default:
		return schedule.Draft{}, errors.New("invalid kind")
	}
	return d, nil
}

func parseScheduleFireAt(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errors.New("fire_at must be RFC3339")
	}
	return t, nil
}

func (s *Server) scheduleItemForOwner(owner, id string) (schedule.Item, bool, error) {
	items, err := s.Schedules.List(owner)
	if err != nil {
		return schedule.Item{}, false, err
	}
	for _, it := range items {
		if it.ID == id {
			return it, true, nil
		}
	}
	return schedule.Item{}, false, nil
}

func (s *Server) handlePauseSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleStore(w) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing schedule id")
		return
	}
	owner := s.scheduleOwnerID(r)
	it, ok, err := s.scheduleItemForOwner(owner, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "schedule_not_found", "schedule not found")
		return
	}
	if it.Status != schedule.StatusActive {
		writeError(w, http.StatusConflict, "schedule_not_active", "only active schedules can be paused")
		return
	}
	if err := s.Schedules.Pause(owner, id); err != nil {
		writeScheduleStoreError(w, err)
		return
	}
	it.Status = schedule.StatusPaused
	writeJSON(w, http.StatusOK, scheduleItemToJSON(it))
}

func (s *Server) handleResumeSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleStore(w) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing schedule id")
		return
	}
	owner := s.scheduleOwnerID(r)
	it, ok, err := s.scheduleItemForOwner(owner, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "schedule_not_found", "schedule not found")
		return
	}
	if it.Status == schedule.StatusCompleted {
		writeError(w, http.StatusConflict, "schedule_completed", "completed schedules cannot be resumed")
		return
	}
	if it.Status != schedule.StatusPaused {
		writeError(w, http.StatusConflict, "schedule_not_paused", "only paused schedules can be resumed")
		return
	}
	d := schedule.Draft{
		Kind:        it.Kind,
		Instruction: it.Instruction,
		Timezone:    it.Timezone,
		FireAt:      it.FireAt,
		TimeOfDay:   it.TimeOfDay,
		Weekday:     it.Weekday,
	}
	next, err := schedule.Next(d, time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := s.Schedules.Resume(owner, id, next); err != nil {
		if errors.Is(err, schedule.ErrActiveCap) {
			writeError(w, http.StatusConflict, "schedule_active_cap", err.Error())
			return
		}
		writeScheduleStoreError(w, err)
		return
	}
	it.Status = schedule.StatusActive
	it.NextRunAt = next.UTC()
	writeJSON(w, http.StatusOK, scheduleItemToJSON(it))
}

// handleRunScheduleNow opens a run immediately from a schedule without
// changing its next_run_at or status. The conversation must be idle and
// owned by the caller; a completed one-off can still be re-run manually.
func (s *Server) handleRunScheduleNow(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleStore(w) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing schedule id")
		return
	}
	owner := s.scheduleOwnerID(r)
	it, ok, err := s.scheduleItemForOwner(owner, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "schedule_not_found", "schedule not found")
		return
	}
	if !s.requireConversationAccess(w, r, it.ConversationID) {
		return
	}
	runID, err := NewScheduleRunOpener(s).Open(it.ConversationID, it.Instruction, owner)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConversationBusy):
			writeError(w, http.StatusConflict, "conversation_busy", "conversation already has an active run")
		case errors.Is(err, ErrCostBudgetExceeded):
			writeError(w, http.StatusTooManyRequests, "cost_budget_exceeded", "daily cost budget reached")
		case errors.Is(err, store.ErrNoPriorConversationRun):
			writeError(w, http.StatusConflict, "no_prior_run", "conversation has no prior run to copy agent/model from")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID})
}

func (s *Server) handleDeleteSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.requireScheduleStore(w) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing schedule id")
		return
	}
	owner := s.scheduleOwnerID(r)
	_, ok, err := s.scheduleItemForOwner(owner, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "schedule_not_found", "schedule not found")
		return
	}
	if err := s.Schedules.Delete(owner, id); err != nil {
		writeScheduleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeScheduleStoreError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not found"), strings.Contains(msg, "owner mismatch"):
		writeError(w, http.StatusNotFound, "schedule_not_found", "schedule not found")
	case strings.Contains(msg, "not active"):
		writeError(w, http.StatusConflict, "schedule_not_active", msg)
	case strings.Contains(msg, "not paused"):
		writeError(w, http.StatusConflict, "schedule_not_paused", msg)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", msg)
	}
}
