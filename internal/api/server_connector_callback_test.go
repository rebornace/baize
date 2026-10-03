package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// 两次 PUT 同一个 openapi 连接器：第一次显式设置 execution_callback_url；
// 第二次请求体省略该字段（只改 base_url），原回调 URL 必须被保留。
func TestPutConnectorOmitsExecutionCallbackPreservesExisting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "cb1", map[string]any{
		"type":                   "openapi",
		"spec":                   writeLoginGetMeAPISpec(t),
		"base_url":               upstream.URL,
		"execution_callback_url": "https://gw.example/execute",
	})

	putConnectorJSON(t, h, "cb1", map[string]any{
		"type":     "openapi",
		"base_url": upstream.URL + "/v2",
		// 无 execution_callback_url
	})

	got, err := st.GetConnector("cb1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionCallbackURL != "https://gw.example/execute" {
		t.Fatalf("callback not preserved: %q", got.ExecutionCallbackURL)
	}
}

// 显式提交空字符串必须清空已有 execution_callback_url，不被省略保留逻辑吞掉。
func TestPutConnectorEmptyExecutionCallbackClears(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(upstream.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	h := srv.Handler()

	putConnectorJSON(t, h, "cb2", map[string]any{
		"type":                   "openapi",
		"spec":                   writeLoginGetMeAPISpec(t),
		"base_url":               upstream.URL,
		"execution_callback_url": "https://gw.example/execute",
	})

	putConnectorJSON(t, h, "cb2", map[string]any{
		"type":                   "openapi",
		"base_url":               upstream.URL,
		"execution_callback_url": "",
	})

	got, err := st.GetConnector("cb2")
	if err != nil {
		t.Fatal(err)
	}
	if got.ExecutionCallbackURL != "" {
		t.Fatalf("callback not cleared: %q", got.ExecutionCallbackURL)
	}
}
