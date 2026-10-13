package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func goalAPIServer(t *testing.T) (*Server, conversation.Store, http.Handler) {
	t.Helper()
	st := store.NewMemory()
	msgs := conversation.NewMemoryStore()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Messages = msgs
	srv.DefaultAgentID = "default-agent"
	st.UpsertAgent(store.Agent{ID: "default-agent", System: "sys"})
	base := runtimecfg.Snapshot{
		Knobs: runtimecfg.Knobs{
			ActiveGoalEnabled:      true,
			ActiveGoalMaxDepth:     8,
			ActiveGoalMaxTextRunes: 500,
		},
	}
	srv.Settings = runtimecfg.New(base)
	return srv, msgs, srv.Handler()
}

func serveGoal(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, jsonBodyAPI(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func decodeGoal(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&out); err != nil {
		t.Fatalf("decode goal: %v body=%s", err, rr.Body.String())
	}
	return out
}

func TestGoalAPIFlow(t *testing.T) {
	_, msgs, h := goalAPIServer(t)
	_, _ = msgs.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "hello"})
	before := len(msgs.List("conv1"))

	rr := serveGoal(t, h, http.MethodPut, "/v0/conversations/conv1/goal", map[string]any{
		"text": "实现登录",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rr.Code, rr.Body.String())
	}
	put := decodeGoal(t, rr)
	if put["revision"] != float64(1) {
		t.Fatalf("put revision=%v", put["revision"])
	}

	rr = serveGoal(t, h, http.MethodPost, "/v0/conversations/conv1/goal/push", map[string]any{
		"text": "查 auth 方案",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("push status=%d body=%s", rr.Code, rr.Body.String())
	}
	push := decodeGoal(t, rr)
	tip, _ := push["tip"].(map[string]any)
	if tip["text"] != "查 auth 方案" {
		t.Fatalf("tip after push=%v", tip)
	}

	rr = serveGoal(t, h, http.MethodGet, "/v0/conversations/conv1/goal", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rr.Code, rr.Body.String())
	}
	got := decodeGoal(t, rr)
	tip, _ = got["tip"].(map[string]any)
	if tip["text"] != "查 auth 方案" {
		t.Fatalf("get tip=%v", tip)
	}

	rr = serveGoal(t, h, http.MethodPost, "/v0/conversations/conv1/goal/pop", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("pop status=%d body=%s", rr.Code, rr.Body.String())
	}
	pop := decodeGoal(t, rr)
	tip, _ = pop["tip"].(map[string]any)
	if tip["text"] != "实现登录" {
		t.Fatalf("tip after pop=%v", tip)
	}

	rr = serveGoal(t, h, http.MethodPost, "/v0/conversations/conv1/goal/stop", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("stop status=%d body=%s", rr.Code, rr.Body.String())
	}
	stop := decodeGoal(t, rr)
	if stop["tip"] != nil {
		t.Fatalf("expected no active tip after stop, got %v", stop["tip"])
	}

	rr = serveGoal(t, h, http.MethodPost, "/v0/conversations/conv1/goal/resume", map[string]any{})
	if rr.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", rr.Code, rr.Body.String())
	}
	resume := decodeGoal(t, rr)
	tip, _ = resume["tip"].(map[string]any)
	if tip["text"] != "实现登录" || tip["status"] != "active" {
		t.Fatalf("tip after resume=%v", tip)
	}

	if len(msgs.List("conv1")) != before {
		t.Fatalf("messages changed: before=%d after=%d", before, len(msgs.List("conv1")))
	}
}

func TestGoalAPIWriteForbiddenWhenDisabled(t *testing.T) {
	srv, _, h := goalAPIServer(t)
	off := false
	if err := srv.Settings.ApplyKnobs(context.Background(), srv.Store, runtimecfg.KnobsPatch{ActiveGoalEnabled: &off}); err != nil {
		t.Fatal(err)
	}

	rr := serveGoal(t, h, http.MethodPut, "/v0/conversations/conv1/goal", map[string]any{"text": "x"})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("put status=%d want 403 body=%s", rr.Code, rr.Body.String())
	}

	rr = serveGoal(t, h, http.MethodGet, "/v0/conversations/conv1/goal", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGoalAPIPutRejectsTextOverKnobMaxRunes(t *testing.T) {
	srv, _, h := goalAPIServer(t)
	maxRunes := 3
	if err := srv.Settings.ApplyKnobs(context.Background(), srv.Store, runtimecfg.KnobsPatch{ActiveGoalMaxTextRunes: &maxRunes}); err != nil {
		t.Fatal(err)
	}
	rr := serveGoal(t, h, http.MethodPut, "/v0/conversations/conv1/goal", map[string]any{
		"text": strings.Repeat("x", 4),
	})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("put status=%d want 400 body=%s", rr.Code, rr.Body.String())
	}
}

func TestGoalAPIForkCopiesStackWithNewFrameIDs(t *testing.T) {
	_, msgs, h := goalAPIServer(t)
	m1, _ := msgs.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "u1"})

	rr := serveGoal(t, h, http.MethodPut, "/v0/conversations/conv1/goal", map[string]any{"text": "主目标"})
	if rr.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rr.Code, rr.Body.String())
	}
	src := decodeGoal(t, rr)
	srcRoot, _ := src["root"].(map[string]any)
	srcRootID, _ := srcRoot["id"].(string)

	rr = serveGoal(t, h, http.MethodPost, "/v0/conversations/conv1/fork", map[string]any{
		"through_message_id": m1.ID,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("fork status=%d body=%s", rr.Code, rr.Body.String())
	}
	var forkBody map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&forkBody); err != nil {
		t.Fatal(err)
	}
	newID, _ := forkBody["conversation_id"].(string)

	rr = serveGoal(t, h, http.MethodGet, "/v0/conversations/"+newID+"/goal", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("get fork goal status=%d body=%s", rr.Code, rr.Body.String())
	}
	forkGoal := decodeGoal(t, rr)
	forkRoot, _ := forkGoal["root"].(map[string]any)
	if forkRoot["text"] != "主目标" {
		t.Fatalf("fork root text=%v", forkRoot["text"])
	}
	forkRootID, _ := forkRoot["id"].(string)
	if forkRootID == "" || forkRootID == srcRootID {
		t.Fatalf("fork root id=%q src=%q", forkRootID, srcRootID)
	}
}
