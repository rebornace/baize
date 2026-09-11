package api_test

import (
	"net/http"
	"net/http/httptest"
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
