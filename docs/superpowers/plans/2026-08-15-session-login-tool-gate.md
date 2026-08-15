# 按会话登录、工具「需要登录」与 Connector 热更新接线 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 对话只用会话身份；配置 Token 可选且不用于带 `conversation_id` 的 Run；每个工具有可改的 `require_login`；`PUT /v0/connectors` 与 `baize start` 共用 `connector.Apply`（Identities / Resolver / OpenAPI Capture）。

**架构：** `internal/connector.Apply` 是唯一注册入口。invoke 时若 ctx 有 `conversation_id`，Resolver 不接收 Connector 默认头；`require_login` 在 HITL 之前拦截。PATCH 只改 Registry + 进程内 Store 列表，重启以 YAML 为准。

**技术栈：** Go 1.22+、现有 httptest、React 设置页、vitest（构建仍 Vite）。

**规格：** `docs/superpowers/specs/2026-08-15-session-login-tool-gate-design.md`

**全局约束：**
- 不在 `main` 上改代码：先 `git checkout -b feat/session-login-tool-gate`
- 不推断 OpenAPI `security` 来设 `require_login`（默认公开）
- HTTP 插件本轮不做登录捕获
- PUT 不增加 `require_approval_mutating`
- 秘密不进 events / GET run / SSE
- commit 中文 `type(scope): 说明`；PowerShell 不要 bash HEREDOC
- Go：`C:\Users\Administrator\sdk\go\bin`；`GOPROXY=https://goproxy.cn,direct`
- 每步测试：`$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH; go test <pkg> -count=1`

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/tool/registry.go` | `RequireLogin`、`SetRequireLogin`、`RequiresLogin`、`Get`、`SecuritySchemes` |
| `internal/tool/login.go` | `LoginRequiredContent()` |
| `internal/tool/registry_test.go` | 开关与 List JSON |
| `internal/store/store.go` | `Connector.RequireLogin`；`ConnectorAuth.Capture` |
| `internal/config/config.go` | `connector.require_login` |
| `internal/connector/openapi/register_opts.go` | `RequireLogin []string`；Meta 带 `SecuritySchemes` |
| `internal/connector/openapi/register.go` | 会话跳过默认头；invoke 时读 `reg.RequiresLogin` |
| `internal/connector/httpplugin/register_opts.go` | `RequireLogin []string` |
| `internal/connector/httpplugin/register.go` | 同上（无 Capture） |
| `internal/connector/capture.go` | 从 bootstrap 挪来的 `CaptureDefaults` |
| `internal/connector/scheme.go` | `UniqueSecurityScheme` |
| `internal/connector/apply.go` | `Apply` / `ApplyInput` |
| `internal/bootstrap/bootstrap.go` | `registerConnector` 改为调 `Apply` |
| `internal/api/server.go` | PUT 走 Apply；PATCH tools；GET 回显 |
| `internal/run/engine.go` | HITL 前登录门闸；`Engine.Identities` |
| `configs/default.yaml` / `configs/docker.yaml` | 去掉硬依赖 Token 的 static header |
| `web/chat/src/api.ts` / `pages/ToolsSettings.tsx` | 「需要登录」开关 |
| `README.md` / `README.zh-CN.md` / `docs/architecture-and-plugin-protocol.md` | 文档对齐 |

---

### 任务 1：Registry `require_login`

**文件：**
- 修改：`internal/tool/registry.go`
- 创建：`internal/tool/login.go`
- 修改或创建：`internal/tool/registry_test.go`
- 修改：`internal/store/store.go`（只加字段，本任务不写 Apply）

- [ ] **步骤 1：写失败测试**（`internal/tool/registry_test.go`）

```go
package tool_test

func TestRequireLoginRoundTrip(t *testing.T) {
	reg := tool.NewRegistry()
	reg.RegisterMeta(tool.Meta{
		Spec:         llm.ToolSpec{Name: "create_ticket"},
		ConnectorID:  "ticket-api",
		RequireLogin: true,
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true}, false, nil
	}, false)
	if !reg.RequiresLogin("create_ticket") {
		t.Fatal("expected require_login")
	}
	if reg.RequiresApproval("create_ticket") {
		t.Fatal("approval must stay independent")
	}
	info, ok := reg.Get("create_ticket")
	if !ok || !info.RequireLogin || info.RequireApproval {
		t.Fatalf("%+v ok=%v", info, ok)
	}
	if err := reg.SetRequireLogin("create_ticket", false); err != nil {
		t.Fatal(err)
	}
	if reg.RequiresLogin("create_ticket") {
		t.Fatal("toggle off")
	}
	if err := reg.SetRequireLogin("missing", true); err == nil {
		t.Fatal("expected unknown tool error")
	}
	got := tool.LoginRequiredContent()
	if got["code"] != "login_required" || got["message"] != "此工具需要先登录" {
		t.Fatalf("%+v", got)
	}
}
```

- [ ] **步骤 2：运行确认失败**

```powershell
$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH
go test ./internal/tool -count=1 -run TestRequireLoginRoundTrip
```

预期：FAIL（`RequiresLogin` / `SetRequireLogin` / `Get` / `LoginRequiredContent` 未定义）。

- [ ] **步骤 3：最少实现**

`Meta` 增加 `RequireLogin bool`、`SecuritySchemes []string`。`entry` 增加对应字段。`RegisterMeta` 从 `meta` 写入，**不要**用 OpenAPI security 自动设 `RequireLogin`。

```go
func LoginRequiredContent() map[string]any {
	return map[string]any{"code": "login_required", "message": "此工具需要先登录"}
}
```

`List` / `Get` 输出 `require_login`。`SetRequireLogin` 未知名返回 `fmt.Errorf("unknown tool: %s", name)`。`RequiresLogin`、`SecuritySchemes(name)` 对称于 `RequiresApproval`。

`store.Connector` 增加 `RequireLogin []string `json:"require_login,omitempty"``。`ConnectorAuth` 增加：

```go
type CaptureAuth struct {
	ToolNameGlob   string   `json:"tool_name_glob,omitempty"`
	TokenJSONPaths []string `json:"token_json_paths,omitempty"`
	LabelJSONPaths []string `json:"label_json_paths,omitempty"`
	HeaderTemplate string   `json:"header_template,omitempty"`
	DefaultScheme  string   `json:"default_scheme,omitempty"`
}
```

`ConnectorAuth` 增加 `Capture CaptureAuth `json:"capture,omitempty"``。

- [ ] **步骤 4：测试通过**

`go test ./internal/tool ./internal/store -count=1`

- [ ] **步骤 5：Commit**

```powershell
git add internal/tool internal/store/store.go
git commit -m "feat(tool): 为每个工具增加 require_login 开关"
```

---

### 任务 2：OpenAPI invoke — 对话不用默认头 + 登录门闸

**文件：**
- 修改：`internal/connector/openapi/register_opts.go`（`RequireLogin []string`）
- 修改：`internal/connector/openapi/register.go`
- 修改：`internal/connector/openapi/register_test.go`

**闭包规则（写进实现，不要只改测试）：**

1. `conv := identity.ConversationIDFrom(ctx)`
2. 传给 Resolver 的 `DefaultHeaders`：`conv == ""` 时与今天相同（含 passthrough）；`conv != ""` 时为 `nil`（含 passthrough 也不回退）
3. `res.OK` 为 false 且 `conv != ""` 时，`overlay` 必须是 `nil`，**禁止**留下 `opts.Headers`
4. `reg.RequiresLogin(name)` 在 **invoke 时**读取（PATCH 后立即生效），不要闭包捕获注册瞬间的 bool
5. 若 `conv != ""` 且 `RequiresLogin` 且没有可用凭证头（`!res.OK` 或 headers 空）：`return tool.LoginRequiredContent(), true, nil`，不调用 `InvokeWithHeaders`
6. 无 `conversation_id`：不看 `require_login`，行为与 2026-08-13 一致
7. 注册：`login[name]=true` 当且仅当名称在 `opts.RequireLogin`；写入 `Meta.RequireLogin` 与 `Meta.SecuritySchemes: route.Security`；Store `Connector.RequireLogin` 存该列表（建议 `sort.Strings` 副本）

- [ ] **步骤 1：写失败测试**（追加到 `register_test.go`）

在现有 `TestRegisterWithOptsResolveAndCapture` **同文件**追加（可新函数，复用 `writeLoginGetMeSpec`）：

```go
func TestConversationDoesNotUseStaticDefaultHeaders(t *testing.T) {
	var lastAuth string
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		lastAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	spec := writeLoginGetMeSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	if _, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID: "c", SpecPath: spec, BaseURL: srv.URL,
		Headers:    map[string]string{"Authorization": "Bearer ENV_TOKEN"},
		Identities: identity.NewMemoryStore(),
		Resolver:   authresolve.OpenAPISecurityResolver{},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := identity.WithConversationID(context.Background(), "conv_no_id")
	_, isErr, err := reg.Invoke(ctx, "getMe", nil)
	if err != nil || isErr {
		t.Fatalf("public getMe: isErr=%v err=%v", isErr, err)
	}
	if lastAuth == "Bearer ENV_TOKEN" {
		t.Fatal("conversation must not send static token")
	}
}

func TestRequireLoginBlocksHTTP(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	spec := writeLoginGetMeSpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	if _, _, err := openapi.RegisterWithOpts(st, reg, openapi.RegisterOpts{
		ID: "c", SpecPath: spec, BaseURL: srv.URL,
		RequireLogin: []string{"getMe"},
		Identities:   identity.NewMemoryStore(),
		Resolver:     authresolve.OpenAPISecurityResolver{},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := identity.WithConversationID(context.Background(), "conv_gate")
	content, isErr, err := reg.Invoke(ctx, "getMe", nil)
	if err != nil || !isErr || content["code"] != "login_required" {
		t.Fatalf("content=%v isErr=%v err=%v", content, isErr, err)
	}
	if hits != 0 {
		t.Fatalf("downstream hits=%d", hits)
	}
	_, isErr, err = reg.Invoke(context.Background(), "getMe", nil)
	if err != nil || isErr {
		t.Fatal("machine path must ignore require_login")
	}
}
```

把现有 `TestRegisterWithOptsResolveAndCapture` 的「无 conv 用 ENV、login 后用捕获」留下；它仍然有效。

- [ ] **步骤 2：运行确认失败**

`go test ./internal/connector/openapi -count=1 -run "TestConversationDoesNotUseStaticDefaultHeaders|TestRequireLoginBlocksHTTP"`

预期：FAIL（对话仍带 ENV，或需登录仍打了 HTTP）。

- [ ] **步骤 3：改 `register.go` 闭包**（按本任务开头 7 条）

`needLogin := login[name]` 只用于 `RegisterMeta` 的 `Meta.RequireLogin`。invoke 用 `reg.RequiresLogin(name)`。

- [ ] **步骤 4：测试通过**

`go test ./internal/connector/openapi -count=1`

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector/openapi
git commit -m "feat(openapi): 对话路径不用默认 Token 并拦截需登录工具"
```

---

### 任务 3：HTTP 插件同样的会话规则（无 Capture）

**文件：**
- 修改：`internal/connector/httpplugin/register_opts.go`、`register.go`、`register_test.go`

闭包规则与任务 2 相同，但：不匹配 `Capture`、不 `Upsert` 身份。`SecuritySchemes` 传 `nil`（Resolver 走「任意活跃身份」）。Store 的 `Auth.Capture` 保持空。

- [ ] **步骤 1：写失败测试**

```go
func TestHTTPPluginConversationSkipsStaticAndRespectsLogin(t *testing.T) {
	var lastAuth string
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo"},{"name":"secret_op"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			hits++
			lastAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{
		ID: "side", BaseURL: srv.URL,
		Headers:      map[string]string{"Authorization": "Bearer PLUGIN_ENV"},
		RequireLogin: []string{"secret_op"},
		Identities:   ids,
		Resolver:     authresolve.OpenAPISecurityResolver{},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := identity.WithConversationID(context.Background(), "c1")
	hits, lastAuth = 0, ""
	_, isErr, err := reg.Invoke(ctx, "echo", nil)
	if err != nil || isErr {
		t.Fatal(err)
	}
	if lastAuth == "Bearer PLUGIN_ENV" {
		t.Fatal("conversation must not send plugin static token")
	}
	content, isErr, err := reg.Invoke(ctx, "secret_op", nil)
	if err != nil || !isErr || content["code"] != "login_required" {
		t.Fatalf("%v %v %v", content, isErr, err)
	}
	if _, err := ids.Upsert("c1", identity.Identity{
		Scheme: "bearer", Subject: "u",
		CredentialHeaders: map[string]string{"Authorization": "Bearer CAP"},
		Source:            identity.SourceLoginCapture,
		IsDefault:         true,
	}); err != nil {
		t.Fatal(err)
	}
	hits, lastAuth = 0, ""
	_, isErr, err = reg.Invoke(ctx, "secret_op", nil)
	if err != nil || isErr {
		t.Fatal(err)
	}
	if lastAuth != "Bearer CAP" {
		t.Fatalf("auth=%q", lastAuth)
	}
}
```

无 conv 的 `TestRegisterListsToolsAndInvoke` 仍应带 `Bearer T`。

- [ ] **步骤 2：运行确认失败**

`go test ./internal/connector/httpplugin -count=1 -run TestHTTPPluginConversationSkipsStaticAndRespectsLogin`

- [ ] **步骤 3：改 `register.go`**

- [ ] **步骤 4：全包通过**

`go test ./internal/connector/httpplugin -count=1`

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector/httpplugin
git commit -m "feat(plugin): HTTP 插件对话路径使用会话身份且不做登录捕获"
```

---

### 任务 4：`connector.Apply` + bootstrap 改走 Apply

**文件：**
- 创建：`internal/connector/capture.go`、`scheme.go`、`apply.go`、`apply_test.go`、`capture_test.go`
- 修改：`internal/bootstrap/bootstrap.go`（删除 `withCaptureDefaults` / `uniqueSecurityScheme` / 直接 `RegisterWithOpts`）
- 修改：`internal/bootstrap/bootstrap_capture_test.go`（capture 单测迁走后，bootstrap 只留 register 失败/vault 形状测，改为仍调 `registerConnector`）
- 修改：`internal/config/config.go`（`RequireLogin []string `yaml:"require_login"``）

`ApplyInput`：

```go
type ApplyInput struct {
	Store                   store.Store
	Registry                *tool.Registry
	Identities              identity.Store
	ID, Type, Spec, BaseURL string
	RequireApproval         []string
	RequireApprovalMutating bool
	RequireLogin            *[]string // nil=从 Registry 保留同名；非 nil=整表（空切片=全公开）
	Auth                    store.ConnectorAuth
}
```

`Apply` 顺序：

1. `authcred.ResolveDefaults`；失败 `fmt.Errorf("resolve connector auth: %w", err)`（bootstrap 现有测试依赖此前缀）
2. 若 `RequireLogin == nil`：扫描 `Registry.List()` 中 `ConnectorID==ID && RequireLogin` 的名字，排序后作为列表
3. `typ` 缺省 `openapi`
4. **openapi：** `CaptureDefaults`；若 `DefaultScheme==""` 则 `LoadTools` + `UniqueSecurityScheme`；`openapi.RegisterWithOpts`（Identities、`OpenAPISecurityResolver{}`、Capture、RequireLogin 列表、Headers、AuthMode、Auth 含补全后的 Capture）
5. **http：** 存盘前把 `Auth.Capture` 清零；`httpplugin.RegisterWithOpts`（Identities + Resolver，无 Capture）
6. 成功后若 Store 里 Connector 的 `RequireLogin` 未写，再 `UpsertConnector` 补上排序后的列表

`CaptureDefaults` 语义原样搬 `withCaptureDefaults`（`__none__` 因 glob 非空而保持）。

启动路径：`registerConnector` 构造 `login := cfg.Connector.RequireLogin` 后传 `RequireLogin: &login`（**指针非 nil**，即使切片为 nil，表示「用 YAML，不要保留」）。

- [ ] **步骤 1：写失败测试** `internal/connector/apply_test.go`

`TestCaptureDefaultsFillsEmpty` / `TestCaptureDefaultsKeepsNone`：从 `bootstrap_capture_test.go` 原断言搬来，包名 `connector_test`。

`TestApplyPreservesRequireLoginWhenOmitted`：

- 写含 `login` + `getMe` 的 spec，第一次 `Apply` 且 `RequireLogin: ptr([]string{"getMe"})`
- 第二次 `Apply` 同一 spec、`RequireLogin: nil`，断言 `reg.RequiresLogin("getMe")` 仍为 true
- 第三次换一份**多一个** `probe` 的 spec、`RequireLogin: nil`，`getMe` 仍 true，`probe` 为 false

`TestApplyEmptyRequireLoginClears`：第二次传 `ptr([]string{})`，`getMe` 变为 false。

`TestApplyOmitsCaptureUsesLoginGlob`：OpenAPI 含 `operationId: AdminAuthController_login` 的 POST；Apply 不设 capture；用 httptest 返回 `accessToken`；带 conv invoke 该工具后 `Identities.List` 非空。

需要最小 spec 夹具（可抄 `writeLoginGetMeSpec`）。`ptr` 辅助：`func ptr[T any](v T) *T { return &v }`。

`TestApplyHTTPIgnoresCapture`：httptest 插件；Apply `Type: http` 且 Auth.Capture 有 glob；GET connector/`st.GetConnector` 的 Capture 为空；invoke 插件「login」成功后 Identities 仍空。

- [ ] **步骤 2：运行确认失败**

`go test ./internal/connector -count=1`

- [ ] **步骤 3：实现 Apply 并改 bootstrap**

`registerConnector` 只组 `ApplyInput`（从 cfg 填 Auth 形状，含 Capture YAML 字段）然后 `_, _, err := connector.Apply(...)`。

- [ ] **步骤 4：测试通过**

```
go test ./internal/connector ./internal/bootstrap -count=1
```

`TestRegisterConnectorStaticMissingEnvFails` 仍须失败且不落库。

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector internal/bootstrap internal/config/config.go
git commit -m "refactor(connector): 抽出 Apply 供启动与 PUT 共用"
```

---

### 任务 5：PUT / GET / PATCH 控制面

**文件：**
- 修改：`internal/api/server.go`、`internal/api/server_test.go`

`handlePutConnector`：

- body 增加 `RequireLogin *[]string `json:"require_login"`` 与 `auth.capture`（扩 `authBody`）
- **不要**再直接调 `openapi.RegisterWithOpts` / `httpplugin.RegisterWithOpts`
- 调 `connector.Apply(ApplyInput{Store, Registry, Identities: s.Identities, ..., RequireLogin: body.RequireLogin, Auth: ...})`
- `require_approval` 仍每次用 body 切片（省略=空=全不审批，**保持现语义**）
- `RequireApprovalMutating` 不传（false）
- 成功后仍设置 `s.AuthMode` / `s.AuthWhitelist`
- 错误映射保持：`invalid_auth` / `invalid_spec` / `invalid_plugin` / `tool_conflict`
- JSON 响应增加 `require_login`

`handleGetConnector` 响应增加 `require_login`；OpenAPI 的 `auth.capture` 来自 Store（Apply 已写入补全后的形状）。

新路由：`PATCH /v0/tools/{name}`

```go
var body struct {
	RequireLogin *bool `json:"require_login"`
}
```

`RequireLogin == nil` → `400 invalid_request`。`Get` 失败 → `404 not_found`。成功：`SetRequireLogin`；按 `info.ConnectorID` 取 Connector，把该名加入或移出 `RequireLogin` 切片（排序），`UpsertConnector`；响应为更新后的 `tool.Info`。

- [ ] **步骤 1：写失败测试**（`server_test.go`）

`TestPutConnectorWiresCaptureWithoutRestart`：httptest 上游 login/getMe；PUT 不带 capture；`srv.Identities` 为 Memory；`reg.Invoke` 带 conv 先 login 再 getMe，Authorization 为捕获 JWT 而非 PUT 里的 static（PUT static 可写 `Authorization: Bearer PUT_STATIC`，且测试进程 `t.Setenv` 不要让展开失败——用字面 `Bearer PUT_STATIC` 而非 `${ENV}`）。

`TestPutConnectorNoneCaptureDisables`：`auth.capture.tool_name_glob: "__none__"`，login 成功后 identities 仍空。

`TestPutOmitsRequireLoginPreserves`：先 PUT `require_login:["create_ticket"]`，再 PUT 省略该键（同 spec），`RequiresLogin` 仍 true。

`TestPutEmptyRequireLoginClears`：第二次 `"require_login":[]`，变为 false。

`TestPatchToolRequireLogin`：PUT 后 PATCH `{"require_login":true}`，GET `/v0/tools` 为 true；再 PATCH false；未知名 404；`{}` 为 400。

`TestPutHTTPPluginUsesIdentitiesNoCapture`：插件 httptest；先往 `srv.Identities` Upsert；PUT type=http；带 conv invoke 侧车工具，请求头为身份而非 static。

- [ ] **步骤 2：运行确认失败**

`go test ./internal/api -count=1 -run "TestPutConnectorWiresCapture|TestPutConnectorNone|TestPutOmitsRequireLogin|TestPutEmptyRequireLogin|TestPatchToolRequireLogin|TestPutHTTPPluginUsesIdentities"`

- [ ] **步骤 3：改 server.go**

- [ ] **步骤 4：测试通过**

`go test ./internal/api -count=1`

- [ ] **步骤 5：Commit**

```powershell
git add internal/api
git commit -m "feat(api): PUT Connector 共用 Apply 并支持工具登录开关"
```

---

### 任务 6：Engine 登录门闸在 HITL 之前

**文件：**
- 修改：`internal/run/engine.go`、`internal/run/engine_test.go`
- 修改：`internal/bootstrap/bootstrap.go`（`engine.Identities = identities`）

在 `RequiresApproval` / `awaitHITL` **之前**：

```go
if e.blockedByLogin(ctx, tc.Name) {
    content, isError := tool.LoginRequiredContent(), true
    // 写 tool.result 事件（RedactSensitive）、把 tool 消息追加进 messages
    continue inner loop  // 不要 awaitHITL、不要 Invoke
}
```

```go
func (e *Engine) blockedByLogin(ctx context.Context, name string) bool {
	if e.Identities == nil || !e.Tools.RequiresLogin(name) {
		return false
	}
	conv := identity.ConversationIDFrom(ctx)
	if conv == "" {
		return false
	}
	res := authresolve.OpenAPISecurityResolver{}.Resolve(ctx, authresolve.ResolveInput{
		Identities:      e.Identities.List(conv),
		SecuritySchemes: e.Tools.SecuritySchemes(name),
		ForceIdentityID: identity.ForceIdentityIDFrom(ctx),
	})
	return !res.OK || len(res.Headers) == 0
}
```

`Engine` 增加 `Identities identity.Store`（可选，nil 则不做引擎侧门闸；invoker 仍会拦。本任务 bootstrap 必须注入，否则 HITL 测试 7 过不了）。

- [ ] **步骤 1：写失败测试**

```go
func TestLoginGateBeforeHITL(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	var calls atomic.Int32
	reg.RegisterMeta(tool.Meta{
		Spec: llm.ToolSpec{Name: "create_ticket"}, ConnectorID: "c", RequireLogin: true,
	}, func(context.Context, map[string]any) (map[string]any, bool, error) {
		calls.Add(1)
		return map[string]any{"id": "1"}, false, nil
	}, true) // require_approval + require_login
	ag := agent.Def{ID: "ticket-agent", System: "helper"}
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: ag.ID, Input: "创建工单", ConversationID: "conv_hitl",
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := &Engine{
		Store: st, LLM: &scriptLLM{}, Tools: reg, Gate: NewGate(),
		Identities: identity.NewMemoryStore(),
	}
	if err := eng.Execute(context.Background(), r.ID, ag, r.Input); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := st.GetRun(r.ID)
	if got.Status == store.StatusWaitingHuman {
		t.Fatal("must not enter waiting_human")
	}
	if calls.Load() != 0 {
		t.Fatalf("invoke=%d", calls.Load())
	}
	evs, _ := st.ListEvents(r.ID)
	for _, ev := range evs {
		if ev.Type == EventHITLWaiting {
			t.Fatal("hitl.waiting")
		}
		if ev.Type == EventToolResult {
			if ev.Data["is_error"] != true {
				t.Fatalf("%+v", ev.Data)
			}
		}
	}
}
```

现有 `TestEngineHITLRejectNoInvoke`（无 conv、Identities nil）必须仍进入 `waiting_human`。

- [ ] **步骤 2：运行确认失败**

`go test ./internal/run -count=1 -run TestLoginGateBeforeHITL`

- [ ] **步骤 3：改 engine + bootstrap 注入 Identities**

- [ ] **步骤 4：测试通过**

`go test ./internal/run ./internal/bootstrap -count=1`

- [ ] **步骤 5：Commit**

```powershell
git add internal/run internal/bootstrap/bootstrap.go
git commit -m "fix(run): 需登录且未登录时不进入人工审批"
```

---

### 任务 7：开箱配置不再要求 Connector Token

**文件：**
- 修改：`configs/default.yaml`、`configs/docker.yaml`
- 修改：`internal/config/docker_yaml_test.go`（断言开箱 yaml 无 `Authorization` static header）
- 不要改 `internal/config/auth_test.go`：那是内联 YAML 夹具，仍覆盖「显式写了 `${BAIZE_CONNECTOR_TOKEN}`」的解析。

从两个 yaml 删除：

```yaml
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
```

保留 `mode: static`。`static.headers` 整段可删。`docker-compose.yml` 的 `BAIZE_CONNECTOR_TOKEN` 可留作机器路径可选，不作为 Chat 开箱依赖。

- [ ] **步骤 1：写失败测试**

`internal/config/docker_yaml_test.go` 的 `TestDefaultYAMLUnchangedForLocalStart` 与 `TestDockerYAMLSidecarCompose` 追加：

```go
if len(cfg.Connector.Auth.Static.Headers["Authorization"]) > 0 {
	t.Fatalf("open-box yaml must not require connector token header: %+v", cfg.Connector.Auth.Static.Headers)
}
```

`internal/connector/apply_test.go`：

```go
func TestApplyEmptyStaticHeadersOK(t *testing.T) {
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.yaml")
	content := "openapi: 3.0.3\ninfo:\n  title: bs\n  version: 0.1.0\npaths:\n  /probe:\n    get:\n      operationId: probe\n      responses:\n        \"200\":\n          description: ok\n"
	if err := os.WriteFile(spec, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	reg := tool.NewRegistry()
	login := []string(nil)
	_, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: identity.NewMemoryStore(),
		ID: "bs", Type: "openapi", Spec: spec, BaseURL: "http://example.invalid",
		RequireLogin: &login,
		Auth:         store.ConnectorAuth{Mode: "static"},
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

显式 `${MISSING}` 的失败测试已在 bootstrap，不要改语义。

- [ ] **步骤 2–4：改 yaml，跑**

```
go test ./internal/connector ./internal/bootstrap ./internal/config ./tests/integration -count=1
```

集成测试若自己 `t.Setenv("BAIZE_CONNECTOR_TOKEN")` 并写 static header，保持即可（机器路径）。

- [ ] **步骤 5：Commit**

```powershell
git add configs/default.yaml configs/docker.yaml internal/connector internal/config/docker_yaml_test.go
git commit -m "fix(config): 开箱 Connector 不再硬依赖进程级 Token"
```

---

### 任务 8：Chat UI「需要登录」+ 文档

**文件：**
- 修改：`web/chat/src/api.ts`、`web/chat/src/pages/ToolsSettings.tsx`、`web/chat/src/style.css`（如需开关样式）
- 构建：`internal/ui/dist/**`
- 修改：`README.md`、`README.zh-CN.md`、`docs/architecture-and-plugin-protocol.md` §4.4

`api.ts`：`ToolInfo.require_login?: boolean`；

```ts
export async function patchToolRequireLogin(name: string, requireLogin: boolean): Promise<ToolInfo> {
  const res = await fetch(`/v0/tools/${encodeURIComponent(name)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ require_login: requireLogin }),
  })
  return parseJSON<ToolInfo>(res)
}
```

Tools 页：每行保留「需审批」徽章；增加 checkbox+文案「需要登录」。`onChange` 调 PATCH，失败写入现有 `settings-error`，成功则更新该行 state。加载中禁用开关。

文档必须做到：

- 删除 README 中英「PUT 未挂 Identities / Resolver / Capture」整段 Note
- 写明：带 `conversation_id` 的 Run（含 `/ui`）只用会话身份；配置 Token 可选，只给无会话的脚本
- 设置 → Tools 可标「需要登录」；默认公开，不读 OpenAPI security
- 架构草案 §4.4 第 2 点改为与规格 §7 相同的四步，并写「需要登录是操作员开关」

- [ ] **步骤 1：改前端并 `npm run build`**

```
cd web/chat
npm test
npm run build
```

`npm test` 现有 vitest 须通过。本页无测试库也可不做组件单测，以 build 为准。

- [ ] **步骤 2：改三份文档**（中英文空格、全角标点）

- [ ] **步骤 3：全量测试**

```
go test ./... -count=1
```

- [ ] **步骤 4：Commit**

```powershell
git add web/chat internal/ui/dist README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md
git commit -m "feat(ui): Tools 设置支持需要登录并同步文档"
```

---

## 规格覆盖对照

| 规格 | 任务 |
|------|------|
| §3.1 / §7 对话不用默认头 | 2、3 |
| §3.2 / §6.1 默认公开、不推断 security | 1、2 |
| §3.3 / §10.12 开箱无 Token | 7 |
| §4 / §5 Apply + Capture 缺省 / `__none__` / 插件忽略 capture | 4、5 |
| §6.2 PATCH + 设置页 | 5、8 |
| §6.3 PUT 省略 vs `[]` | 4、5 |
| §8 插件身份、无捕获 | 3、5 |
| §9 `login_required` + HITL 顺序 | 2、6 |
| §10.1 PUT 后捕获无需重启 | 5 |
| §11 文档 | 8 |
| 机器路径三种 mode | 不改 `tests/integration/connector_auth_modes_test.go` 的无 conv 用例；任务 2/7 跑全量时回归 |
