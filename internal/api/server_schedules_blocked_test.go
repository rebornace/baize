package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/schedule"
	"github.com/rebornace/baize/internal/store"
)

func insertBlockedTestSchedule(t *testing.T, srv *Server, status string) schedule.Item {
	t.Helper()
	it, err := srv.Schedules.Insert(schedule.Item{
		OwnerID:        "alice",
		ConversationID: "c1",
		Instruction:    "x",
		Kind:           "daily",
		Timezone:       "Asia/Shanghai",
		TimeOfDay:      "09:00",
		Status:         status,
		NextRunAt:      time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("insert schedule: %v", err)
	}
	return it
}

func createBlockedTestRun(t *testing.T, srv *Server, status store.Status, hitl *store.HITLPayload) string {
	t.Helper()
	srv.Store.UpsertAgent(store.Agent{ID: "a"})
	r, err := srv.Store.CreateRun(store.CreateRunInput{AgentID: "a", Input: "x", ConversationID: "c1"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := srv.Store.UpdateRun(r.ID, status, "", ""); err != nil {
		t.Fatalf("update run: %v", err)
	}
	if err := srv.Store.SetHITL(r.ID, hitl); err != nil {
		t.Fatalf("set hitl: %v", err)
	}
	return r.ID
}

// listScheduleBlocked returns the blocked_reason for the row and the raw body.
func listScheduleBlocked(t *testing.T, srv *Server, scheduleID string) (string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(
		httptest.NewRequest(http.MethodGet, "/v0/settings/schedules", nil), "alice-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET list: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var list scheduleListResp
	if err := json.NewDecoder(strings.NewReader(rec.Body.String())).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	for _, it := range list.Items {
		if it.ID == scheduleID {
			return it.BlockedReason, rec.Body.String()
		}
	}
	t.Fatalf("schedule %s not in list: %s", scheduleID, rec.Body.String())
	return "", ""
}

func TestScheduleListBlockedReason(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		running   string       // write running_run_id directly; do not create a run
		runStatus store.Status // when non-empty, create a real run in this status
		hitl      *store.HITLPayload
		want      string
	}{
		{
			name:      "approval",
			status:    schedule.StatusActive,
			runStatus: store.StatusWaitingHuman,
			hitl:      &store.HITLPayload{Kind: store.HITLKindApproval},
			want:      "awaiting_approval",
		},
		{
			name:      "approval_empty_kind",
			status:    schedule.StatusActive,
			runStatus: store.StatusWaitingHuman,
			hitl:      &store.HITLPayload{},
			want:      "awaiting_approval",
		},
		{
			name:      "login",
			status:    schedule.StatusActive,
			runStatus: store.StatusWaitingHuman,
			hitl:      &store.HITLPayload{Kind: store.HITLKindLogin},
			want:      "awaiting_login",
		},
		{
			name:      "waiting_child",
			status:    schedule.StatusActive,
			runStatus: store.StatusWaitingHuman,
			hitl:      &store.HITLPayload{Kind: store.HITLKindWaitingChild},
			want:      "waiting_child",
		},
		{
			name:      "payload_nil",
			status:    schedule.StatusActive,
			runStatus: store.StatusWaitingHuman,
			want:      "awaiting_input",
		},
		{
			name:    "run_missing",
			status:  schedule.StatusActive,
			running: "run_does_not_exist",
			want:    "run_unavailable",
		},
		{
			name:    "pending_claim",
			status:  schedule.StatusActive,
			running: schedule.ClaimPending,
			want:    "",
		},
		{
			name:      "running",
			status:    schedule.StatusActive,
			runStatus: store.StatusRunning,
			want:      "",
		},
		{
			name:      "terminal_not_cleared",
			status:    schedule.StatusActive,
			runStatus: store.StatusSucceeded,
			want:      "",
		},
		{
			name:      "paused",
			status:    schedule.StatusPaused,
			runStatus: store.StatusWaitingHuman,
			hitl:      &store.HITLPayload{Kind: store.HITLKindApproval},
			want:      "",
		},
		{
			name:      "completed",
			status:    schedule.StatusCompleted,
			runStatus: store.StatusWaitingHuman,
			hitl:      &store.HITLPayload{Kind: store.HITLKindApproval},
			want:      "",
		},
		{
			name:   "no_running_run",
			status: schedule.StatusActive,
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := schedulesSettingsServer(t)
			it := insertBlockedTestSchedule(t, srv, tc.status)

			runID := tc.running
			if tc.runStatus != "" {
				runID = createBlockedTestRun(t, srv, tc.runStatus, tc.hitl)
			}
			if runID != "" {
				if err := srv.Schedules.SetRunningRunID(it.ID, runID); err != nil {
					t.Fatalf("set running run: %v", err)
				}
			}

			got, body := listScheduleBlocked(t, srv, it.ID)
			if got != tc.want {
				t.Fatalf("blocked_reason=%q want %q (body=%s)", got, tc.want, body)
			}
		})
	}
}

func TestScheduleListBlockedReasonOmittedWhenEmpty(t *testing.T) {
	srv := schedulesSettingsServer(t)
	it := insertBlockedTestSchedule(t, srv, schedule.StatusActive)

	got, body := listScheduleBlocked(t, srv, it.ID)
	if got != "" {
		t.Fatalf("blocked_reason=%q want empty", got)
	}
	if strings.Contains(body, "blocked_reason") {
		t.Fatalf("empty blocked_reason must be omitted, got %s", body)
	}
}
