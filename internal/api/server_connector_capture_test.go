package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func apiNewServer(st store.Store, reg *tool.Registry) *api.Server {
	return api.NewServer(st, reg, &fakeRunner{store: st})
}

func putConnectorJSON(t *testing.T, h http.Handler, id string, body map[string]any) {
	t.Helper()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPut,
		"/v0/connectors/"+id, jsonBody(t, body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT %s status=%d body=%s", id, rr.Code, rr.Body.String())
	}
}

// 两次 PUT 同一个 openapi 连接器：第一次显式设置自定义 capture；第二次请求体
// 完全不带 auth.capture（连接器页改版后的提交形状），capture 必须被保留，
// 同时 static 等默认凭证按第二次提交（空）写入。
func TestPutConnectorOmitsCapturePreservesExisting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	spec := writeLoginGetMeAPISpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.Identities = identity.NewMemoryStore()
	h := srv.Handler()

	first := map[string]any{
		"type":     "openapi",
		"spec":     spec,
		"base_url": upstream.URL,
		"auth": map[string]any{
			"mode": "static",
			"static": map[string]any{
				"headers": map[string]string{"Authorization": "Bearer OLD"},
			},
			"capture": map[string]any{
				"tool_name_glob":   "custom_login_*",
				"token_json_paths": []string{"data.token"},
				"header_template":  "Token {{token}}",
			},
		},
	}
	putConnectorJSON(t, h, "cap1", first)

	// 第二次：连接器页提交，不含 auth（Go encoding/json 缺省即 nil 指针）。
	second := map[string]any{
		"type":     "openapi",
		"base_url": upstream.URL,
	}
	putConnectorJSON(t, h, "cap1", second)

	got, err := st.GetConnector("cap1")
	if err != nil {
		t.Fatalf("get connector: %v", err)
	}
	if got.Auth.Capture.ToolNameGlob != "custom_login_*" {
		t.Fatalf("capture glob not preserved: %+v", got.Auth.Capture)
	}
	if len(got.Auth.Capture.TokenJSONPaths) != 1 || got.Auth.Capture.TokenJSONPaths[0] != "data.token" {
		t.Fatalf("capture token paths not preserved: %+v", got.Auth.Capture)
	}
	if got.Auth.Capture.HeaderTemplate != "Token {{token}}" {
		t.Fatalf("capture template not preserved: %+v", got.Auth.Capture)
	}
	if len(got.Auth.Static.Headers) != 0 {
		t.Fatalf("static headers must be cleared on omit, got %+v", got.Auth.Static.Headers)
	}
}

// 新建连接器（无历史记录）且省略 capture：生效 CaptureDefaults（*login*）。
func TestPutConnectorNewOmitsCaptureGetsDefaults(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "cap2", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
	})
	got, err := st.GetConnector("cap2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Auth.Capture.ToolNameGlob != "*login*" {
		t.Fatalf("new connector capture default glob = %q want *login*", got.Auth.Capture.ToolNameGlob)
	}
}

// 显式提交 capture（含 __none__）必须覆盖旧值，不被保留逻辑吞掉。
func TestPutConnectorExplicitCaptureOverrides(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "cap3", map[string]any{
		"type":     "openapi",
		"spec":     writeLoginGetMeAPISpec(t),
		"base_url": upstream.URL,
		"auth":     map[string]any{"capture": map[string]any{"tool_name_glob": "keep_*"}},
	})
	putConnectorJSON(t, h, "cap3", map[string]any{
		"type":     "openapi",
		"base_url": upstream.URL,
		"auth":     map[string]any{"capture": map[string]any{"tool_name_glob": "__none__"}},
	})
	got, _ := st.GetConnector("cap3")
	if got.Auth.Capture.ToolNameGlob != "__none__" {
		t.Fatalf("explicit capture not honored: %+v", got.Auth.Capture)
	}
}

// 第二步保存工具权限时显式发送 require_approval: []（JSON 必须真正序列化为
// []，而非缺省）必须清空第一步/之前保存的审批勾选，与 require_login 的
// nil=保留、非 nil=整表重写语义对称（I-1）。
func TestPutConnectorEmptyApprovalArrayClearsExisting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	// 第一次 PUT：第一步连接信息 + 第二步审批名单（login 为 spec 中的 POST 工具）。
	putConnectorJSON(t, h, "app1", map[string]any{
		"type":             "openapi",
		"spec":             writeLoginGetMeAPISpec(t),
		"base_url":         upstream.URL,
		"require_approval": []string{"login"},
	})
	first, err := st.GetConnector("app1")
	if err != nil {
		t.Fatalf("get after first PUT: %v", err)
	}
	if len(first.RequireApproval) != 1 || first.RequireApproval[0] != "login" {
		t.Fatalf("first PUT require_approval=%v want [login]", first.RequireApproval)
	}

	// 第二次 PUT：第二步取消全部审批勾选。空切片必须编码为 JSON []，不能缺省。
	body := map[string]any{
		"type":             "openapi",
		"base_url":         upstream.URL,
		"require_approval": []string{},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"require_approval":[]`) {
		t.Fatalf("test body must serialize empty array as [], got %s", raw)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, "/v0/connectors/app1",
		strings.NewReader(string(raw))))
	if rr.Code != http.StatusOK {
		t.Fatalf("second PUT status=%d body=%s", rr.Code, rr.Body.String())
	}

	got, err := st.GetConnector("app1")
	if err != nil {
		t.Fatalf("get after second PUT: %v", err)
	}
	if len(got.RequireApproval) != 0 {
		t.Fatalf("connector require_approval must be cleared, got %v", got.RequireApproval)
	}
	var found bool
	for _, tl := range st.ListToolsByConnector("app1") {
		if tl.Name == "login" {
			found = true
			if tl.RequireApproval {
				t.Fatalf("login.RequireApproval must be false after explicit empty array, got %+v", tl)
			}
		}
	}
	if !found {
		t.Fatal("login tool missing from catalog")
	}

	// GET 必须把空名单稳定序列化为 []，而不是 null（O-1）。
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/v0/connectors/app1", nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	gotBody := getRR.Body.String()
	for _, key := range []string{`"require_approval":[]`, `"require_login":[]`} {
		if !strings.Contains(gotBody, key) {
			t.Fatalf("GET connector body must contain %s, got %s", key, gotBody)
		}
	}
}
