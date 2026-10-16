package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/schedule"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func schedulesSettingsServer(t *testing.T) *Server {
	t.Helper()
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.AdminToken = "adm-token"
	srv.Operators = []controlplane.Operator{
		{ID: "alice", Token: "alice-token"},
		{ID: "bob", Token: "bob-token"},
	}
	srv.Schedules = schedule.NewMemoryStore()
	return srv
}

type scheduleListResp struct {
	Items []scheduleItemJSON `json:"items"`
}

func TestSchedulesSettingsFlow(t *testing.T) {
	srv := schedulesSettingsServer(t)
	h := srv.Handler()

	dailyBody := `{"conversation_id":"c1","kind":"daily","instruction":"每日提醒","time_of_day":"09:00"}`
	postRec := httptest.NewRecorder()
	h.ServeHTTP(postRec, withNamedOp(httptest.NewRequest(http.MethodPost, "/v0/settings/schedules",
		strings.NewReader(dailyBody)), "alice-token"))
	if postRec.Code != http.StatusCreated {
		t.Fatalf("POST daily: status=%d body=%s", postRec.Code, postRec.Body.String())
	}
	var created scheduleItemJSON
	if err := json.NewDecoder(postRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Instruction != "每日提醒" || created.Status != schedule.StatusActive {
		t.Fatalf("unexpected create: %+v", created)
	}

	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/schedules", nil), "alice-token"))
	if listRec.Code != http.StatusOK {
		t.Fatalf("GET list: status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var list scheduleListResp
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Instruction != "每日提醒" {
		t.Fatalf("alice list want 1 daily with instruction, got %+v", list.Items)
	}

	for i := 0; i < 19; i++ {
		body := fmt.Sprintf(`{"conversation_id":"c1","kind":"daily","instruction":"extra-%d","time_of_day":"10:00"}`, i)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, withNamedOp(httptest.NewRequest(http.MethodPost, "/v0/settings/schedules",
			strings.NewReader(body)), "alice-token"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("POST extra %d: status=%d body=%s", i, rec.Code, rec.Body.String())
		}
	}

	capRec := httptest.NewRecorder()
	h.ServeHTTP(capRec, withNamedOp(httptest.NewRequest(http.MethodPost, "/v0/settings/schedules",
		strings.NewReader(`{"conversation_id":"c1","kind":"daily","instruction":"第二十一条","time_of_day":"11:00"}`)), "alice-token"))
	if capRec.Code != http.StatusConflict {
		t.Fatalf("21st POST: want 409, got %d body=%s", capRec.Code, capRec.Body.String())
	}

	countRec := httptest.NewRecorder()
	h.ServeHTTP(countRec, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/schedules", nil), "alice-token"))
	var afterCap scheduleListResp
	if err := json.NewDecoder(countRec.Body).Decode(&afterCap); err != nil {
		t.Fatalf("decode count list: %v", err)
	}
	if len(afterCap.Items) != 20 {
		t.Fatalf("after cap failure want 20 rows, got %d", len(afterCap.Items))
	}

	pauseRec := httptest.NewRecorder()
	h.ServeHTTP(pauseRec, withNamedOp(httptest.NewRequest(http.MethodPost,
		"/v0/settings/schedules/"+created.ID+"/pause", nil), "alice-token"))
	if pauseRec.Code != http.StatusOK {
		t.Fatalf("pause: status=%d body=%s", pauseRec.Code, pauseRec.Body.String())
	}

	pausedListRec := httptest.NewRecorder()
	h.ServeHTTP(pausedListRec, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/schedules", nil), "alice-token"))
	var pausedList scheduleListResp
	if err := json.NewDecoder(pausedListRec.Body).Decode(&pausedList); err != nil {
		t.Fatalf("decode paused list: %v", err)
	}
	var found *scheduleItemJSON
	for i := range pausedList.Items {
		if pausedList.Items[i].ID == created.ID {
			found = &pausedList.Items[i]
			break
		}
	}
	if found == nil || found.Status != schedule.StatusPaused {
		t.Fatalf("paused schedule want status paused, got %+v", found)
	}

	bobListRec := httptest.NewRecorder()
	h.ServeHTTP(bobListRec, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/schedules", nil), "bob-token"))
	var bobList scheduleListResp
	if err := json.NewDecoder(bobListRec.Body).Decode(&bobList); err != nil {
		t.Fatalf("decode bob list: %v", err)
	}
	for _, it := range bobList.Items {
		if it.ID == created.ID {
			t.Fatalf("bob must not see alice schedule %+v", it)
		}
	}
}

func TestSchedulesSettingsStoreUnavailable(t *testing.T) {
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.AdminToken = "adm-token"
	srv.Operators = []controlplane.Operator{{ID: "alice", Token: "alice-token"}}
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withNamedOp(httptest.NewRequest(http.MethodGet, "/v0/settings/schedules", nil), "alice-token"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no schedule store: want 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}
