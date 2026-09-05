package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/runtimecfg"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func runtimeSettingsServer(t *testing.T) (*api.Server, store.Store) {
	t.Helper()
	st := store.NewMemory()
	// The settings/credentials endpoints never touch the runner; pass nil.
	srv := api.NewServer(st, tool.NewRegistry(), nil)
	base := runtimecfg.Snapshot{
		Knobs: runtimecfg.Knobs{MaxMessages: 40, MaxSteps: 16, CompactionEnabled: true, CompactThreshold: 0.8},
		Creds: runtimecfg.Credentials{
			OperatorToken: "op", AdminToken: "adm",
			Operators: []controlplane.Operator{{ID: "alice", Token: "ta"}},
		},
	}
	h := runtimecfg.New(base)
	if err := h.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	srv.Settings = h
	return srv, st
}

func doJSON(t *testing.T, srv *api.Server, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestGetRuntimeSettings(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/v0/settings/runtime", "op", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"max_steps":16`) {
		t.Fatalf("expected effective max_steps, body=%s", rr.Body.String())
	}
}

func TestPatchRuntimeSettingsHot(t *testing.T) {
	srv, st := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/runtime", "adm",
		map[string]any{"max_steps": 32, "compaction_enabled": false})
	if rr.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rr.Code, rr.Body.String())
	}
	if srv.Settings.Knobs().MaxSteps != 32 || srv.Settings.Knobs().CompactionEnabled {
		t.Fatalf("knobs not applied: %+v", srv.Settings.Knobs())
	}
	// Persists across a fresh holder over the same store (simulates restart /
	// another replica loading the KV).
	h2 := runtimecfg.New(runtimecfg.Snapshot{Knobs: runtimecfg.Knobs{MaxSteps: 16, CompactionEnabled: true}})
	if err := h2.Load(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if h2.Knobs().MaxSteps != 32 || h2.Knobs().CompactionEnabled {
		t.Fatalf("override must survive reload: %+v", h2.Knobs())
	}
}

func TestPatchRuntimeSettingsInvalid(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/runtime", "adm",
		map[string]any{"max_steps": 999})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPatchRuntimeSettingsForbiddenForOperator(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/runtime", "op",
		map[string]any{"max_steps": 32})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestCredentialsGetMasks(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	rr := doJSON(t, srv, http.MethodGet, "/v0/settings/credentials", "adm", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, secret := range []string{`"adm"`, `"op"`, `"ta"`} {
		if strings.Contains(body, secret) {
			t.Fatalf("credentials leaked %s: %s", secret, body)
		}
	}
	if !strings.Contains(body, `"source":"config"`) {
		t.Fatalf("expected source config, body=%s", body)
	}
}

func TestCredentialsRotateAddConflictRemoveReset(t *testing.T) {
	srv, _ := runtimeSettingsServer(t)
	// rotate admin
	rr := doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "adm",
		map[string]any{"admin_token": "new-adm"})
	if rr.Code != http.StatusOK {
		t.Fatalf("rotate status=%d %s", rr.Code, rr.Body.String())
	}
	// new admin token works immediately
	rr2 := doJSON(t, srv, http.MethodGet, "/v0/settings/credentials", "new-adm", nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("rotated admin not effective, status=%d", rr2.Code)
	}
	// add operator bob
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"add_operators": []map[string]any{{"id": "bob", "token": "tb"}}})
	if rr.Code != http.StatusOK {
		t.Fatalf("add op status=%d %s", rr.Code, rr.Body.String())
	}
	// duplicate -> 409
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"add_operators": []map[string]any{{"id": "bob", "token": "x"}}})
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rr.Code)
	}
	// remove config operator alice -> 400
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"remove_operators": []string{"alice"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 removing config op, got %d", rr.Code)
	}
	// reset -> baseline admin works again
	rr = doJSON(t, srv, http.MethodPatch, "/v0/settings/credentials", "new-adm",
		map[string]any{"reset": true})
	if rr.Code != http.StatusOK {
		t.Fatalf("reset status=%d %s", rr.Code, rr.Body.String())
	}
	rr3 := doJSON(t, srv, http.MethodGet, "/v0/settings/credentials", "adm", nil)
	if rr3.Code != http.StatusOK {
		t.Fatalf("baseline admin not restored, status=%d", rr3.Code)
	}
}
