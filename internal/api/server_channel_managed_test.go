package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/store"
)

// fakeManaged is a test channel implementing both channel.Channel and
// channel.ManagedChannel so the generic /v0/settings/channels/{name} plane can
// drive it without any channel-specific handlers.
type fakeManaged struct {
	settings  channel.ChannelSettings
	status    channel.AdapterStatus
	loggedOut bool

	procStarted int
	procStopped int
	procRestart int
	procErr     error
}

func (f *fakeManaged) Name() string { return "weixin" }
func (f *fakeManaged) Source() string {
	return "weixin"
}
func (f *fakeManaged) Start(context.Context) error { return nil }
func (f *fakeManaged) Stop(context.Context) error  { return nil }
func (f *fakeManaged) SendText(context.Context, string, string, map[string]string) error {
	return nil
}
func (f *fakeManaged) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}
func (f *fakeManaged) GetSettings() channel.ChannelSettings { return f.settings }
func (f *fakeManaged) UpdateSettings(s channel.ChannelSettings) channel.AdapterStatus {
	f.settings = s
	return f.status
}
func (f *fakeManaged) Status() channel.AdapterStatus { return f.status }
func (f *fakeManaged) LoginStart(context.Context) (channel.LoginTicket, error) {
	return channel.LoginTicket{Ticket: "tk", QRURL: "qr"}, nil
}
func (f *fakeManaged) LoginPoll(context.Context, string) (string, error) { return "success", nil }
func (f *fakeManaged) Logout(context.Context) error {
	f.loggedOut = true
	return nil
}
func (f *fakeManaged) StartProcess(context.Context) error {
	f.procStarted++
	return f.procErr
}
func (f *fakeManaged) StopProcess(context.Context) error {
	f.procStopped++
	return f.procErr
}
func (f *fakeManaged) RestartProcess(context.Context) error {
	f.procRestart++
	return f.procErr
}

var (
	_ channel.Channel           = (*fakeManaged)(nil)
	_ channel.ManagedChannel    = (*fakeManaged)(nil)
	_ channel.ProcessController = (*fakeManaged)(nil)
)

// fakeManagedNoProcess is managed but does NOT implement ProcessController
// (independently deployed adapter): process endpoints must return 501.
type fakeManagedNoProcess struct{}

func (fakeManagedNoProcess) Name() string                { return "weixin" }
func (fakeManagedNoProcess) Source() string              { return "weixin" }
func (fakeManagedNoProcess) Start(context.Context) error { return nil }
func (fakeManagedNoProcess) Stop(context.Context) error  { return nil }
func (fakeManagedNoProcess) SendText(context.Context, string, string, map[string]string) error {
	return nil
}
func (fakeManagedNoProcess) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}
func (fakeManagedNoProcess) GetSettings() channel.ChannelSettings {
	return channel.ChannelSettings{Enabled: true}
}
func (fakeManagedNoProcess) UpdateSettings(channel.ChannelSettings) channel.AdapterStatus {
	return channel.AdapterStatus{Running: true}
}
func (fakeManagedNoProcess) Status() channel.AdapterStatus {
	return channel.AdapterStatus{Running: true}
}
func (fakeManagedNoProcess) LoginStart(context.Context) (channel.LoginTicket, error) {
	return channel.LoginTicket{}, nil
}
func (fakeManagedNoProcess) LoginPoll(context.Context, string) (string, error) { return "pending", nil }
func (fakeManagedNoProcess) Logout(context.Context) error                      { return nil }

var _ channel.ManagedChannel = fakeManagedNoProcess{}

func managedTestServer(t *testing.T) *api.Server {
	t.Helper()
	ch := &fakeManaged{
		settings: channel.ChannelSettings{Assignee: "alice", AgentID: "ag", Enabled: true, Allowlist: []string{}},
		status:   channel.AdapterStatus{Running: true},
	}
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.RegisterChannel(&api.ChannelHandle{Name: "weixin", Channel: ch})
	return srv
}

func admReq(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer adm")
	return req
}

func TestManagedGetSettings(t *testing.T) {
	srv := managedTestServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, admReq(http.MethodGet, "/v0/settings/channels/weixin"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET=%d body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got["assignee"] != "alice" || got["enabled"] != true || got["running"] != true {
		t.Fatalf("body=%v", got)
	}
}

// P2 设置信息架构：渠道状态 GET 已降为 RoleOperator，运营可查看；
// 写操作（PUT）仍需管理员。
func TestManagedOperatorCanRead(t *testing.T) {
	srv := managedTestServer(t)
	srv.OperatorToken = "op"
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/weixin", nil)
	req.Header.Set("Authorization", "Bearer op")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("operator GET=%d want 200", rec.Code)
	}

	putReq := httptest.NewRequest(http.MethodPut, "/v0/settings/channels/weixin", nil)
	putReq.Header.Set("Authorization", "Bearer op")
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusForbidden {
		t.Fatalf("operator PUT=%d want 403", putRec.Code)
	}
}

// P2 设置信息架构：运营可发起微信扫码登录（login/start、login/status 为 200），
// 但登出与进程控制仍需管理员（403）。
func TestManagedOperatorLoginAllowedProcessForbidden(t *testing.T) {
	srv := managedTestServer(t)
	srv.OperatorToken = "op"
	opReq := func(method, path string) *http.Request {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer op")
		return req
	}
	code := func(method, path string) int {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, opReq(method, path))
		return rec.Code
	}

	if c := code(http.MethodPost, "/v0/settings/channels/weixin/login/start"); c != http.StatusOK {
		t.Fatalf("operator login/start=%d want 200", c)
	}
	if c := code(http.MethodGet, "/v0/settings/channels/weixin/login/status?ticket=tk"); c != http.StatusOK {
		t.Fatalf("operator login/status=%d want 200", c)
	}
	if c := code(http.MethodPost, "/v0/settings/channels/weixin/logout"); c != http.StatusForbidden {
		t.Fatalf("operator logout=%d want 403", c)
	}
	if c := code(http.MethodPost, "/v0/settings/channels/weixin/process/start"); c != http.StatusForbidden {
		t.Fatalf("operator process/start=%d want 403", c)
	}
	if c := code(http.MethodPost, "/v0/settings/channels/weixin/process/stop"); c != http.StatusForbidden {
		t.Fatalf("operator process/stop=%d want 403", c)
	}
	if c := code(http.MethodGet, "/v0/settings/events-webhook"); c != http.StatusForbidden {
		t.Fatalf("operator GET events-webhook=%d want 403", c)
	}
	if c := code(http.MethodGet, "/v0/settings/store"); c != http.StatusForbidden {
		t.Fatalf("operator GET store=%d want 403", c)
	}
}

func TestManagedLoginLogoutRoutes(t *testing.T) {
	srv := managedTestServer(t)
	hit := func(method, path string) int {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, admReq(method, path))
		return rec.Code
	}
	if c := hit(http.MethodPost, "/v0/settings/channels/weixin/login/start"); c != http.StatusOK {
		t.Fatalf("login/start=%d", c)
	}
	if c := hit(http.MethodGet, "/v0/settings/channels/weixin/login/status?ticket=tk"); c != http.StatusOK {
		t.Fatalf("login/status=%d", c)
	}
	if c := hit(http.MethodPost, "/v0/settings/channels/weixin/logout"); c != http.StatusOK {
		t.Fatalf("logout=%d", c)
	}
	if c := hit(http.MethodGet, "/v0/settings/channels/unknown"); c != http.StatusNotFound {
		t.Fatalf("unknown channel=%d want 404", c)
	}
}

func TestManagedProcessControl(t *testing.T) {
	ch := &fakeManaged{
		settings: channel.ChannelSettings{Enabled: true},
		status:   channel.AdapterStatus{Running: true},
	}
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.RegisterChannel(&api.ChannelHandle{Name: "weixin", Channel: ch})

	post := func(path string) int {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, admReq(http.MethodPost, path))
		return rec.Code
	}
	if c := post("/v0/settings/channels/weixin/process/start"); c != http.StatusOK {
		t.Fatalf("process/start=%d", c)
	}
	if c := post("/v0/settings/channels/weixin/process/stop"); c != http.StatusOK {
		t.Fatalf("process/stop=%d", c)
	}
	if c := post("/v0/settings/channels/weixin/process/restart"); c != http.StatusOK {
		t.Fatalf("process/restart=%d", c)
	}
	if ch.procStarted != 1 || ch.procStopped != 1 || ch.procRestart != 1 {
		t.Fatalf("calls start=%d stop=%d restart=%d want 1/1/1", ch.procStarted, ch.procStopped, ch.procRestart)
	}

	// Operator token must be forbidden on process control.
	srv.OperatorToken = "op"
	opReq := httptest.NewRequest(http.MethodPost, "/v0/settings/channels/weixin/process/restart", nil)
	opReq.Header.Set("Authorization", "Bearer op")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, opReq)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator restart=%d want 403", rec.Code)
	}
}

func TestManagedProcessNotControllable(t *testing.T) {
	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.AdminToken = "adm"
	srv.RegisterChannel(&api.ChannelHandle{Name: "weixin", Channel: fakeManagedNoProcess{}})
	for _, path := range []string{"start", "stop", "restart"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, admReq(http.MethodPost, "/v0/settings/channels/weixin/process/"+path))
		if rec.Code != http.StatusNotImplemented {
			t.Fatalf("process/%s=%d want 501 body=%s", path, rec.Code, rec.Body.String())
		}
	}
}
