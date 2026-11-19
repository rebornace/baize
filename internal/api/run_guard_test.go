package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func runGuardServer(t *testing.T) *Server {
	t.Helper()
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	return NewServer(st, nil, &gateFakeRunner{store: st})
}

func postRun(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestPostRunRejectsBusyConversation(t *testing.T) {
	srv := runGuardServer(t)
	active, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "first", ConversationID: "c1",
	})
	if err != nil {
		t.Fatal(err)
	}

	rr := postRun(t, srv, `{"agent_id":"a","input":"second","conversation_id":"c1"}`)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "conversation_busy" {
		t.Fatalf("code=%q", body.Error.Code)
	}

	// A terminal run frees the slot.
	if err := srv.Store.UpdateRun(active.ID, store.StatusSucceeded, "done", ""); err != nil {
		t.Fatal(err)
	}
	rr = postRun(t, srv, `{"agent_id":"a","input":"third","conversation_id":"c1"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("after terminal status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestStartRunBusySentinel(t *testing.T) {
	srv := runGuardServer(t)
	if _, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "first", ConversationID: "c1",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := srv.startRun(context.Background(), startRunInput{
		AgentID: "a", Input: "second", ConversationID: "c1",
	})
	if !errors.Is(err, store.ErrConversationBusy) {
		t.Fatalf("err=%v want ErrConversationBusy", err)
	}
}

func TestCreateRunIfIdleSerializesConcurrentStarts(t *testing.T) {
	srv := runGuardServer(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := srv.createRunIfIdle("c1", store.CreateRunInput{
				AgentID: "a", Input: "x", ConversationID: "c1",
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	var ok, busy int
	for err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, store.ErrConversationBusy):
			busy++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if ok != 1 || busy != 1 {
		t.Fatalf("ok=%d busy=%d", ok, busy)
	}
}

func TestCreateRunIfIdleDifferentConversations(t *testing.T) {
	srv := runGuardServer(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, conv := range []string{"c1", "c2"} {
		wg.Add(1)
		go func(conv string) {
			defer wg.Done()
			_, err := srv.createRunIfIdle(conv, store.CreateRunInput{
				AgentID: "a", Input: "x", ConversationID: conv,
			})
			errs <- err
		}(conv)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("different conversations must not block: %v", err)
		}
	}
}

func TestPostRunMachinePathSkipsGuard(t *testing.T) {
	srv := runGuardServer(t)
	for i := 0; i < 2; i++ {
		rr := postRun(t, srv, `{"agent_id":"a","input":"machine"}`)
		if rr.Code != http.StatusOK {
			t.Fatalf("machine path call %d: status=%d body=%s", i, rr.Code, rr.Body.String())
		}
	}
}
