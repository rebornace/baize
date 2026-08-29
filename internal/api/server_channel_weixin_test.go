package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channel/weixin"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func weixinTestServer(t *testing.T) (*api.Server, *weixin.Fake, string) {
	t.Helper()
	dir := t.TempDir()
	fake := weixin.NewFake()
	fake.LoginSequence = []string{weixin.LoginStatusPending, weixin.LoginStatusSuccess}

	meta := conversation.NewMemoryStore()
	rt := &channel.Runtime{
		Runs:           store.NewMemory(),
		Meta:           meta,
		Messages:       meta,
		Assignee:       "alice",
		DefaultAgentID: "agent-1",
	}
	ch := weixin.New(fake, rt, "", "")
	ch.SetCredsDir(dir)

	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	srv.WeixinILink = fake
	srv.WeixinChannel = ch
	srv.WeixinRuntime = rt
	srv.WeixinCredsDir = dir
	return srv, fake, dir
}

func TestWeixinLoginStartForbiddenForOperator(t *testing.T) {
	srv, _, _ := weixinTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/channels/weixin/login/start", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWeixinLoginStartReturnsQR(t *testing.T) {
	srv, fake, _ := weixinTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/channels/weixin/login/start", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Ticket string `json:"ticket"`
		QRURL  string `json:"qr_url"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Ticket != fake.Ticket || resp.QRURL != fake.QRURL {
		t.Fatalf("got %+v want ticket=%q qr=%q", resp, fake.Ticket, fake.QRURL)
	}
}

func TestWeixinLoginStatusSuccessSavesCredsAndStarts(t *testing.T) {
	srv, fake, dir := weixinTestServer(t)
	fake.LoginSequence = []string{weixin.LoginStatusSuccess}

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/weixin/login/status?ticket=fake-ticket", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != weixin.LoginStatusSuccess {
		t.Fatalf("status=%q", resp.Status)
	}

	accountID, token, err := weixin.LoadCreds(dir)
	if err != nil {
		t.Fatalf("LoadCreds: %v", err)
	}
	if accountID != fake.AccountID || token != fake.Token {
		t.Fatalf("creds account=%q token=%q", accountID, token)
	}
	if !srv.WeixinChannel.IsStarted() {
		t.Fatal("channel should be started after successful login")
	}

	t.Cleanup(func() {
		_ = srv.WeixinChannel.Stop(t.Context())
	})
}

func TestWeixinLogoutClearsCredsAndStops(t *testing.T) {
	srv, fake, dir := weixinTestServer(t)
	fake.LoginSequence = []string{weixin.LoginStatusSuccess}

	startReq := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/weixin/login/status?ticket=t", nil)
	startReq.Header.Set("Authorization", "Bearer adm")
	startRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRR, startReq)
	if startRR.Code != http.StatusOK {
		t.Fatalf("login status=%d", startRR.Code)
	}
	if !srv.WeixinChannel.IsStarted() {
		t.Fatal("expected started")
	}

	req := httptest.NewRequest(http.MethodPost, "/v0/settings/channels/weixin/logout", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", rr.Code, rr.Body.String())
	}
	if srv.WeixinChannel.IsStarted() {
		t.Fatal("channel should be stopped after logout")
	}
	if _, err := os.Stat(filepath.Join(dir, "creds.json")); !os.IsNotExist(err) {
		t.Fatalf("creds.json should be removed, err=%v", err)
	}
}

func TestWeixinSettingsGetPut(t *testing.T) {
	srv, _, dir := weixinTestServer(t)

	putBody := jsonBody(t, map[string]any{
		"agent_id":  "agent-wx",
		"allowlist": []string{"peer-a", "peer-b"},
		"assignee":  "bob",
		"enabled":   true,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/v0/settings/channels/weixin", putBody)
	putReq.Header.Set("Authorization", "Bearer adm")
	putReq.Header.Set("Content-Type", "application/json")
	putRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", putRR.Code, putRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/weixin", nil)
	getReq.Header.Set("Authorization", "Bearer adm")
	getRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var resp struct {
		AgentID   string   `json:"agent_id"`
		Allowlist []string `json:"allowlist"`
		Assignee  string   `json:"assignee"`
		Enabled   bool     `json:"enabled"`
	}
	if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.AgentID != "agent-wx" || resp.Assignee != "bob" || !resp.Enabled {
		t.Fatalf("resp=%+v", resp)
	}
	if len(resp.Allowlist) != 2 || resp.Allowlist[0] != "peer-a" {
		t.Fatalf("allowlist=%v", resp.Allowlist)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatalf("settings.json: %v", err)
	}
	if srv.WeixinRuntime.Assignee != "bob" || srv.WeixinRuntime.DefaultAgentID != "agent-wx" {
		t.Fatalf("runtime not updated: assignee=%q agent=%q", srv.WeixinRuntime.Assignee, srv.WeixinRuntime.DefaultAgentID)
	}
}
