# HTTP 侧车插件协议 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** Runtime 可作为客户端发现并调用 `type: http` 侧车（healthz / tools / invoke）；参考侧车证明无 OpenAPI 也能挂进 Registry；HITL 与 `auth.mode` 与 OpenAPI Connector 同一套规则。

**架构：** 新包 `internal/connector/httpplugin` 与 OpenAPI 并列。PUT/YAML 按 `type` 分流。侧车 ToolDesc 登记进现有 Registry。invoke 带 `X-Baize-Protocol: v0` 与 Run 上下文。`examples/http-plugin` 为独立进程。开箱默认仍是 mock-ticket OpenAPI。

**技术栈：** Go 1.22+、net/http、httptest、现有 Registry / authcred / authresolve。

**规格：** `docs/superpowers/specs/2026-08-14-http-plugin-v0-design.md`

**全局约束：**
- 不做 MCP、企业回调、`callback_urls`、登录捕获
- `annotations.dangerous` 不自动 HITL；只认 `require_approval`
- 注册失败用 `httpplugin.ErrInvalidPlugin` → API `400 invalid_plugin`；不得部分注册
- 开箱不自动启动参考侧车；`configs/default.yaml` 保持 `type: openapi`
- commit 中文：`type(scope): 说明`

---

## 文件结构（将创建/修改）

| 路径 | 职责 |
|------|------|
| `internal/connector/httpplugin/protocol.go` | 常量头、ToolDesc、Invoke 请求/响应、错误体 |
| `internal/connector/httpplugin/client.go` | healthz、ListTools、Invoke（30s 超时） |
| `internal/connector/httpplugin/register.go` | `RegisterWithOpts`、`ErrInvalidPlugin` |
| `internal/connector/httpplugin/*_test.go` | 发现/invoke/失败不污染 |
| `internal/identity/context.go` | `WithRunID`/`RunIDFrom`、`WithAgentID`/`AgentIDFrom` |
| `internal/run/engine.go` | `injectAuthCtxFromRun` 写入 run_id、agent_id |
| `internal/api/server.go` | PUT 按 type 分流；http 不强制 spec |
| `internal/bootstrap/bootstrap.go` | YAML `type: http` 走 httpplugin |
| `examples/http-plugin/main.go` | 参考侧车 |
| `examples/http-plugin/handler.go` | healthz/tools/echo/create_ticket |
| `examples/http-plugin/handler_test.go` | 协议头与工具行为 |
| `tests/integration/http_plugin_test.go` | PUT→GET tools→Run |
| `README.md` / `README.zh-CN.md` | 「无 OpenAPI：HTTP 插件」 |
| `docs/architecture-and-plugin-protocol.md` | §4.2 标明客户端已实现 |

---

### 任务 1：协议客户端（healthz / list / invoke）

**文件：**
- 创建：`internal/connector/httpplugin/protocol.go`
- 创建：`internal/connector/httpplugin/client.go`
- 创建：`internal/connector/httpplugin/client_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
package httpplugin_test

func TestClientHealthzAndListTools(t *testing.T) {
	var sawProto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawProto = r.Header.Get("X-Baize-Protocol")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v0/tools":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"d"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := httpplugin.NewClient(srv.URL)
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sawProto != "v0" {
		t.Fatalf("protocol=%q", sawProto)
	}
	tools, err := c.ListTools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools=%+v err=%v", tools, err)
	}
}

func TestClientHealthzNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"down"}`))
	}))
	defer srv.Close()
	err := httpplugin.NewClient(srv.URL).Healthz(context.Background())
	if !errors.Is(err, httpplugin.ErrInvalidPlugin) {
		t.Fatalf("err=%v", err)
	}
}

func TestClientInvokeSendsContextAndHeaders(t *testing.T) {
	var gotBody map[string]any
	var gotRunHdr, gotAuth, gotProto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProto = r.Header.Get("X-Baize-Protocol")
		gotRunHdr = r.Header.Get("X-Baize-Run-Id")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
	}))
	defer srv.Close()
	c := httpplugin.NewClient(srv.URL)
	out, err := c.Invoke(context.Background(), "echo", map[string]any{"x": 1}, httpplugin.InvokeMeta{
		RunID:   "run_1",
		AgentID: "ag_1",
		Headers: map[string]string{"Authorization": "Bearer TOK"},
	})
	if err != nil || out.IsError || out.Content["ok"] != true {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if gotProto != "v0" || gotRunHdr != "run_1" || gotAuth != "Bearer TOK" {
		t.Fatalf("hdr proto=%s run=%s auth=%s", gotProto, gotRunHdr, gotAuth)
	}
	ctx, _ := gotBody["context"].(map[string]any)
	if ctx["run_id"] != "run_1" || ctx["agent_id"] != "ag_1" {
		t.Fatalf("body=%+v", gotBody)
	}
}

func TestClientInvokeHTTPErrorIsToolError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`oops`))
	}))
	defer srv.Close()
	out, err := httpplugin.NewClient(srv.URL).Invoke(context.Background(), "echo", nil, httpplugin.InvokeMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Fatalf("want is_error out=%+v", out)
	}
}

func TestClientListToolsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tools":[]}`))
	}))
	defer srv.Close()
	_, err := httpplugin.NewClient(srv.URL).ListTools(context.Background())
	if !errors.Is(err, httpplugin.ErrInvalidPlugin) {
		t.Fatalf("err=%v", err)
	}
}
```

- [ ] **步骤 2：** `go test ./internal/connector/httpplugin/ -count=1` 预期 FAIL（包不存在）

- [ ] **步骤 3：最少实现**

```go
package httpplugin

const (
	HeaderProtocol = "X-Baize-Protocol"
	HeaderRunID    = "X-Baize-Run-Id"
	ProtocolV0     = "v0"
)

var ErrInvalidPlugin = errors.New("invalid_plugin")

type ToolDesc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

type InvokeMeta struct {
	RunID   string
	AgentID string
	Headers map[string]string
}

type InvokeResult struct {
	Content map[string]any
	IsError bool
}

type Client struct {
	BaseURL    string
	HTTPClient *http.Client // nil → 30s timeout client
}

func NewClient(baseURL string) *Client
func (c *Client) Healthz(ctx context.Context) error
func (c *Client) ListTools(ctx context.Context) ([]ToolDesc, error)
func (c *Client) Invoke(ctx context.Context, name string, args map[string]any, meta InvokeMeta) (InvokeResult, error)
```

规则：所有请求设 `X-Baize-Protocol: v0`；Invoke 另设 `X-Baize-Run-Id`（非空时）并 merge `meta.Headers`。Healthz 要求 HTTP 200 且 `status=="ok"`。ListTools 要求至少 1 个非空 name。Invoke HTTP 非 2xx 或 `is_error` → `InvokeResult{IsError:true}` 且 `error==nil`。JSON 含 `error.code` 时把 message 放进 content。缺省 `input_schema` 不在 Client 填，留给 Register。超时 30s。

- [ ] **步骤 4：** `go test ./internal/connector/httpplugin/ -count=1`

- [ ] **步骤 5：Commit** `feat(httpplugin): 侧车协议客户端 healthz/tools/invoke`

---

### 任务 2：RegisterWithOpts 接入 Registry

**文件：**
- 创建：`internal/connector/httpplugin/register.go`
- 创建：`internal/connector/httpplugin/register_opts.go`
- 创建：`internal/connector/httpplugin/register_test.go`
- 修改：`internal/identity/context.go`
- 修改：`internal/run/engine.go`

- [ ] **步骤 1：失败测试**

```go
func TestRegisterListsToolsAndInvoke(t *testing.T) {
	var lastAuth, lastRun, lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		lastRun = r.Header.Get("X-Baize-Run-Id")
		lastPath = r.URL.Path
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	_, infos, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{
		ID: "side", BaseURL: srv.URL, Headers: map[string]string{"Authorization": "Bearer T"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Name != "echo" || infos[0].ConnectorID != "side" {
		t.Fatalf("%+v", infos)
	}
	if infos[0].Method != "" {
		t.Fatalf("method should be empty: %+v", infos[0])
	}
	ctx := identity.WithRunID(context.Background(), "run_9")
	content, isErr, invErr := reg.Invoke(ctx, "echo", map[string]any{"a": 1})
	if invErr != nil || isErr || content["ok"] != true {
		t.Fatalf("invoke %v %v %v", content, isErr, invErr)
	}
	if lastAuth != "Bearer T" || lastRun != "run_9" || !strings.Contains(lastPath, "echo") {
		t.Fatalf("auth=%s run=%s path=%s", lastAuth, lastRun, lastPath)
	}
	c, err := st.GetConnector("side")
	if err != nil || c.Type != "http" || c.Spec != "" {
		t.Fatalf("store %+v err=%v", c, err)
	}
}

func TestRegisterHealthzFailDoesNotRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", 500)
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.RegisterMeta(tool.Meta{Spec: llm.ToolSpec{Name: "keep"}, ConnectorID: "other"},
		func(context.Context, map[string]any) (map[string]any, bool, error) { return nil, false, nil }, false)
	_, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{ID: "side", BaseURL: srv.URL})
	if !errors.Is(err, httpplugin.ErrInvalidPlugin) {
		t.Fatalf("err=%v", err)
	}
	if len(reg.List()) != 1 || reg.List()[0].Name != "keep" {
		t.Fatalf("polluted %+v", reg.List())
	}
}

func TestRegisterConflict(t *testing.T) {
	// httptest 返回 name=echo；先在 other connector 注册 echo；期望 ErrToolConflict 且 other 仍在
}
```

`RegisterConflict`：先 `RegisterMeta` echo 到 `other`，侧车也返回 echo，期望 `errors.Is(err, openapi.ErrToolConflict)` **或** 本包导出同一哨兵。计划选定：**复用** `openapi.ErrToolConflict` 会让 httpplugin 依赖 openapi，不干净。改为 `httpplugin.ErrToolConflict` 与 openapi 同值字符串 `tool_conflict`，API 层 `errors.Is` 两个都认。

更干净：把 `ErrToolConflict` 留在 `internal/tool` 或两边各一份，API 用 `errors.Is(err, httpplugin.ErrToolConflict) || errors.Is(err, openapi.ErrToolConflict)`。

**裁定：** `httpplugin.ErrToolConflict = errors.New("tool_conflict")`，与 openapi 文本相同；API 两处 Is。

- [ ] **步骤 2：** 测试 FAIL

- [ ] **步骤 3：实现 RegisterOpts 与 RegisterWithOpts**

```go
type RegisterOpts struct {
	ID              string
	BaseURL         string
	RequireApproval []string
	Headers         map[string]string
	AuthMode        string
	Auth            store.ConnectorAuth
	Identities      identity.Store
	Resolver        authresolve.Resolver
}

func RegisterWithOpts(st store.Store, reg *tool.Registry, opts RegisterOpts) (store.Connector, []tool.Info, error)
```

顺序：Healthz → ListTools → 无名过滤后 names → WouldConflict → UnregisterConnector → 对每个 ToolDesc `RegisterMeta`（InputSchema 空则 `{type:object}`；dangerous **不** 置 requireApproval）→ UpsertConnector Type=`http` Spec=`""`。

Invoke 闭包：从 ctx 取 overlay（passthrough 与 openapi 相同逻辑）+ Resolver（SecuritySchemes 空列表，走默认身份/DefaultHeaders）+ `Client.Invoke(..., InvokeMeta{RunID: identity.RunIDFrom(ctx), AgentID: identity.AgentIDFrom(ctx), Headers: overlay})`。返回 `content, isError, err` 中 err 仅传输层 Client 返回的非 nil（Client.Invoke 对 HTTP 错误已转 IsError）。

`identity`：

```go
func WithRunID(ctx context.Context, id string) context.Context
func RunIDFrom(ctx context.Context) string
func WithAgentID(ctx context.Context, id string) context.Context
func AgentIDFrom(ctx context.Context) string
```

`injectAuthCtxFromRun`：

```go
ctx = identity.WithRunID(ctx, runRec.ID)
ctx = identity.WithAgentID(ctx, runRec.AgentID)
```

- [ ] **步骤 4：** `go test ./internal/connector/httpplugin/ ./internal/run/ ./internal/identity/ -count=1`

- [ ] **步骤 5：Commit** `feat(httpplugin): 注册侧车工具到 Registry`

---

### 任务 3：API / bootstrap 按 type 分流

**文件：**
- 修改：`internal/api/server.go`
- 修改：`internal/api/server_test.go` 或新建 `server_httpplugin_test.go`
- 修改：`internal/bootstrap/bootstrap.go`
- 测试：`internal/bootstrap` 仅在需要时加短测

- [ ] **步骤 1：失败测试**

```go
func TestPutHTTPConnectorRegistersTools(t *testing.T) {
	sidecar := httptest.NewServer(/* healthz ok + tools echo */)
	defer sidecar.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodPut, "/v0/connectors/side",
		jsonBody(t, map[string]any{"type": "http", "base_url": sidecar.URL}))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"spec":`) && strings.Contains(rr.Body.String(), `"examples`) {
		t.Fatal("http connector should not require spec")
	}
}

func TestPutHTTPMissingBaseURL(t *testing.T) {
	// type http, no base_url → 400 invalid_request
}

func TestPutHTTPInvalidPlugin(t *testing.T) {
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer sidecar.Close()
	// PUT → 400 invalid_plugin；reg.List 仍空
}

func TestPutOpenAPIStillRequiresSpec(t *testing.T) {
	// type openapi 或缺省、无 spec → 400（现有行为）
}
```

- [ ] **步骤 2：改 handlePutConnector**

```go
if body.Type == "" {
	body.Type = "openapi"
}
if body.Type != "openapi" && body.Type != "http" {
	writeError(w, 400, "invalid_request", "unsupported connector type")
	return
}
if body.Type == "openapi" && body.Spec == "" {
	writeError(..., "spec is required")
	return
}
if body.Type == "http" && strings.TrimSpace(body.BaseURL) == "" {
	writeError(..., "base_url is required")
	return
}
// ResolveDefaults 对两种 type 都做（现有）
switch body.Type {
case "http":
	c, infos, err = httpplugin.RegisterWithOpts(...)
case "openapi":
	c, infos, err = openapi.RegisterWithOpts(...)
}
if errors.Is(err, httpplugin.ErrInvalidPlugin) {
	writeError(w, 400, "invalid_plugin", err.Error())
	return
}
if errors.Is(err, httpplugin.ErrToolConflict) || errors.Is(err, openapi.ErrToolConflict) {
	writeError(w, 409, "tool_conflict", ...)
}
```

GET connector 已回显 store 字段，http 的空 spec 可原样输出。

bootstrap `registerConnector`：

```go
if typ == "http" {
	if strings.TrimSpace(cfg.Connector.BaseURL) == "" {
		return fmt.Errorf("connector.base_url is required")
	}
	_, _, err = httpplugin.RegisterWithOpts(...) // 无 Capture
	return err
}
if cfg.Connector.Spec == "" {
	return fmt.Errorf("connector.spec is required")
}
// 现有 openapi 路径
```

- [ ] **步骤 3：** `go test ./internal/api/ ./internal/bootstrap/ -count=1`

- [ ] **步骤 4：Commit** `feat(api): PUT type=http 注册侧车 Connector`

---

### 任务 4：参考侧车 examples/http-plugin

**文件：**
- 创建：`examples/http-plugin/main.go`
- 创建：`examples/http-plugin/handler.go`
- 创建：`examples/http-plugin/handler_test.go`

与 `examples/mock-ticket` 一样作为本模块子包（`package httppluginexample` 或 `package main` + 可测 Handler）。**选定：** `package httppluginex`，`NewHandler() http.Handler`，`main.go` ListenAndServe。默认地址 `:19090`。

- [ ] **步骤 1：测试**

```go
func TestProtocolHeaderRequired(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 400 {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "protocol_unsupported") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}

func TestEchoAndCreateTicket(t *testing.T) {
	h := NewHandler()
	hdr := func(r *http.Request) { r.Header.Set("X-Baize-Protocol", "v0") }
	// GET /healthz with proto → ok
	// GET /v0/tools → names echo, create_ticket
	// POST invoke echo arguments {"ping":1} → content 含 ping
	// POST invoke create_ticket {"title":"x"} → content.id 非空
}
```

协议校验：除可不校验的 `/healthz` 外，tools 与 invoke **必须** 有 `X-Baize-Protocol: v0`。规格写 healthz 也带该头；侧车 healthz **建议同样校验**，与 Client 发出的头一致。**裁定：所有三个端点都校验协议头。**

- [ ] **步骤 2：** 实现 handler：内存 `[]ticket`；create_ticket 无 title → is_error true；未知 tool 名 → 404 + error 体。

`main.go`：

```go
addr := ":19090"
if v := os.Getenv("BAIZE_HTTP_PLUGIN_LISTEN"); v != "" {
	addr = v
}
log.Fatal(http.ListenAndServe(addr, NewHandler()))
```

- [ ] **步骤 3：** `go test ./examples/http-plugin/ -count=1`

- [ ] **步骤 4：Commit** `feat(examples): HTTP 插件参考侧车`

---

### 任务 5：HITL + static 鉴权单测（注册层）

**文件：**
- 修改：`internal/connector/httpplugin/register_test.go`

把规格里 HITL / 鉴权从「仅集成」落到注册层，避免集成失败时无法定位。

- [ ] **步骤 1：**

```go
func TestRegisterRequireApproval(t *testing.T) {
	// 侧车返回 create_ticket；RegisterOpts.RequireApproval=["create_ticket"]
	// reg.RequiresApproval("create_ticket")==true
	// annotations.dangerous=true 的 echo 若不在名单则 RequiresApproval false
}

func TestRegisterPassthroughUsesContextHeaders(t *testing.T) {
	// AuthMode passthrough，Headers 空；ctx WithPassthroughHeaders Authorization Bearer FROM_RUN
	// invoke 时侧车看到该头
}
```

- [ ] **步骤 2–4：** 实现应已在任务 2 完成；本任务只补测试，若缺口则修闭包。

- [ ] **步骤 5：Commit** `test(httpplugin): HITL 名单与透传头`

---

### 任务 6：集成测试 + 文档

**文件：**
- 创建：`tests/integration/http_plugin_test.go`
- 修改：`README.md`、`README.zh-CN.md`
- 修改：`docs/architecture-and-plugin-protocol.md`

- [ ] **步骤 1：集成测试**

使用 `examples/http-plugin` 的 `NewHandler` + `httptest.NewServer`（不必 Listen :19090）：

1. `api.NewServer` + `httpplugin` PUT（经 HTTP PUT `/v0/connectors/side`）
2. GET `/v0/tools` 含 `echo`、`connector_id=side`
3. `run.Engine` + mock LLM 第一次 tool_call `echo`，第二次文本；断言下游 invoke 发生且 `X-Baize-Protocol: v0`
4. 另测：RequireApproval create_ticket → Execute 后 status waiting_human（可用 Engine 直连，不必走 LLM 脚本两次）

- [ ] **步骤 2：**

```bash
go test ./tests/integration/ -count=1
go test ./... -count=1
```

- [ ] **步骤 3：文档**

README 中英在「Platform integration (OpenAPI)」后增加：

```markdown
## No OpenAPI: HTTP plugin

When the legacy system has no usable OpenAPI spec, run a sidecar that implements
`GET /healthz`, `GET /v0/tools`, and `POST /v0/tools/{name}/invoke`
(`X-Baize-Protocol: v0`).

```bash
go run ./examples/http-plugin
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/legacy-sidecar \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"http\",\"base_url\":\"http://127.0.0.1:19090\",\"require_approval\":[\"create_ticket\"]}"
```

HITL still uses `require_approval`. Default `baize start` keeps the mock-ticket OpenAPI connector.
```

架构 §4.2 末加一句：Runtime 已实现该协议的客户端；`callback_urls` 与 §4.3 企业回调尚未实现。

确认 `configs/default.yaml` 仍是 `type: openapi`。

- [ ] **步骤 4：Commit** `test+docs(plugin): HTTP 侧车接入与文档`

---

## 自检（对照规格）

| 规格要点 | 任务 |
|----------|------|
| type=http PUT、无 spec、缺 base_url | 3 |
| healthz/tools/invoke 客户端 | 1 |
| Registry 登记、冲突、失败不污染 | 2 |
| 协议头、run_id、auth 默认头 | 1, 2, 5 |
| HITL 仅 require_approval | 2, 5, 6 |
| 参考侧车 echo + create_ticket | 4 |
| 集成 PUT→tools→Run | 6 |
| 文档；开箱不改默认 | 6 |
| 不做 MCP/回调/捕获 | 全计划未包含 |

无 TODO 占位。类型名：`httpplugin.Client`、`ErrInvalidPlugin`、`ErrToolConflict`、`RegisterOpts`、`identity.WithRunID`。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-14-http-plugin-v0.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 任务间审查
2. **内联执行** — 本会话用 executing-plans 按任务推进并设检查点

选哪种方式？
