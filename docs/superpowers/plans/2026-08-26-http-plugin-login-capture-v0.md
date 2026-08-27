# HTTP 插件登录捕获 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** HTTP 侧车登录工具成功后按 Connector `auth.capture` 写入会话身份；同会话 `require_login` 插件工具自动带捕获头。

**架构：** 废除 Apply 对 `type=http` 清空 Capture；`CaptureDefaults` 后写入 Store 并放入 `registerOneContext.capture`。在 `pluginInvokerClosure`（及对称的 `httpplugin.RegisterWithOpts`）于成功 invoke 后复用与 OpenAPI 相同的 `ExtractCredential` + `Upsert`。HITL 豁免已由 `registerOne` 中「匹配 capture glob 则跳过 blanket 审批」覆盖，一旦 http 传入非空 capture 即生效。

**技术栈：** Go；既有 `internal/identity`、`internal/connector`、`examples/http-plugin`；测试用 `httptest` + `go test`。

**规格：** `docs/superpowers/specs/2026-08-26-http-plugin-login-capture-v0-design.md`

**Git：** 过程提交进 `baize_real`；建议分支 `feat/http-plugin-login-capture`。

**环境（Windows）：**
```powershell
$env:PATH = "$env:USERPROFILE\sdk\go\bin;" + $env:PATH
$env:GOPROXY = "https://goproxy.cn,direct"
```

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/connector/capture_after.go`（新建，可选）或就地小函数 | 共享「成功后尝试捕获」逻辑，供 openapi / plugin 闭包调用 |
| `internal/connector/apply.go` | `type=http` 也跑 `CaptureDefaults` 并持久化 `auth.Capture` |
| `internal/connector/register_one.go` | `pluginInvokerClosure` 成功后捕获；`RegisterOneFromConnector` 对 http 加载 Capture |
| `internal/connector/apply_test.go` | 改写 `TestApplyHTTPIgnoresCapture` → 保留并捕获 |
| `internal/connector/httpplugin/register_opts.go` | `RegisterOpts.Capture` |
| `internal/connector/httpplugin/register.go` | `RegisterWithOpts` 闭包镜像捕获（与 Apply 路径一致） |
| `internal/connector/httpplugin/register_test.go`（或新测试文件） | RegisterWithOpts 捕获单测 |
| `examples/http-plugin/handler.go` + 测试 / README 旁注 | 最小 `login` 工具 |
| `docs/architecture-and-plugin-protocol.md`、`README.md`、`README.zh-CN.md` | 用户向说明 |
| `docs/superpowers/specs/2026-08-15-session-login-tool-gate-design.md` | 文首/§8 交叉引用本规格 |

刻意不改：MCP 路径、引擎、前端 capture 表单（Tools 页已有）。

---

### 任务 1：Apply 持久化 HTTP Capture + RegisterOneFromConnector 加载

**文件：**
- 修改：`internal/connector/apply.go`（http 分支设 `capture`；持久化分支对 http 写入 Capture）
- 修改：`internal/connector/register_one.go`（`RegisterOneFromConnector` 的 `case "http"` 加载 CaptureDefaults）
- 修改：`internal/connector/apply_test.go`（改写 `TestApplyHTTPIgnoresCapture`）

- [ ] **步骤 1：改写失败测试（先改断言为「应保留 Capture」）**

将 `TestApplyHTTPIgnoresCapture` 重命名为 `TestApplyHTTPPersistsAndCaptures`，核心断言改为：

```go
func TestApplyHTTPPersistsAndCaptures(t *testing.T) {
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[
				{"name":"login","description":"login"},
				{"name":"secure_ping","description":"needs auth"}
			]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			if strings.Contains(r.URL.Path, "/login/") || strings.HasSuffix(r.URL.Path, "/login/invoke") || strings.Contains(r.URL.Path, "tools/login") {
				_, _ = w.Write([]byte(`{"content":{"accessToken":"tok-http","email":"a@x.com"},"is_error":false}`))
				return
			}
			lastAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "side", Type: "http", BaseURL: srv.URL,
		RequireLogin: ptr([]string{"secure_ping"}),
		Auth: store.ConnectorAuth{
			Mode: "static",
			Capture: store.CaptureAuth{
				ToolNameGlob:   "*login*",
				TokenJSONPaths: []string{"accessToken"},
				LabelJSONPaths: []string{"email"},
				HeaderTemplate: "Bearer {{token}}",
			},
		},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	c, err := st.GetConnector("side")
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Capture.ToolNameGlob != "*login*" || len(c.Auth.Capture.TokenJSONPaths) == 0 {
		t.Fatalf("HTTP connector must persist Capture, got %+v", c.Auth.Capture)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_http")
	_, isErr, invErr := reg.Invoke(ctx, "login", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke login: isErr=%v err=%v", isErr, invErr)
	}
	if len(ids.List("conv_http")) == 0 {
		t.Fatal("expected captured identity after plugin login")
	}

	_, isErr, invErr = reg.Invoke(ctx, "secure_ping", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("secure_ping: isErr=%v err=%v", isErr, invErr)
	}
	if lastAuth != "Bearer tok-http" {
		t.Fatalf("Authorization=%q want Bearer tok-http", lastAuth)
	}
}
```

注意：`httpplugin.Client` 的 invoke 路径一般为 `/v0/tools/{name}/invoke`——匹配分支时用 `strings.Contains(r.URL.Path, "/tools/login/")` 或 `PathValue` 等价判断，与仓库现有 httpplugin 测试一致即可。

- [ ] **步骤 2：运行测试确认失败**

```powershell
go test ./internal/connector/ -count=1 -run TestApplyHTTPPersistsAndCaptures
```

预期：FAIL（Store Capture 仍被清空，和/或 Identities 仍空）。

- [ ] **步骤 3：Apply 对 http 计算并持久化 Capture**

在 `apply.go` 的 `case "http":` 发现工具成功后（`client = cl` 附近）增加：

```go
capture = CaptureDefaults(identity.CaptureConfig{
	ToolNameGlob:   in.Auth.Capture.ToolNameGlob,
	TokenJSONPaths: in.Auth.Capture.TokenJSONPaths,
	LabelJSONPaths: in.Auth.Capture.LabelJSONPaths,
	HeaderTemplate: in.Auth.Capture.HeaderTemplate,
	DefaultScheme:  in.Auth.Capture.DefaultScheme,
})
```

将持久化条件从「仅 openapi」改为「openapi **或** http」：

```go
if typ == "openapi" || typ == "http" {
	auth.Capture = store.CaptureAuth{
		ToolNameGlob:   capture.ToolNameGlob,
		TokenJSONPaths: capture.TokenJSONPaths,
		LabelJSONPaths: capture.LabelJSONPaths,
		HeaderTemplate: capture.HeaderTemplate,
		DefaultScheme:  capture.DefaultScheme,
	}
} else {
	auth.Capture = store.CaptureAuth{}
}
```

`rctx` 已传 `capture: capture`——http 现在非零后，`registerOne` 的 HITL 豁免自动生效。

- [ ] **步骤 4：`RegisterOneFromConnector` 对 http 加载 Capture**

在 `register_one.go` 的 `case "http":`：

```go
case "http":
	client = httpplugin.NewClient(c.BaseURL)
	capture = CaptureDefaults(identity.CaptureConfig{
		ToolNameGlob:   c.Auth.Capture.ToolNameGlob,
		TokenJSONPaths: c.Auth.Capture.TokenJSONPaths,
		LabelJSONPaths: c.Auth.Capture.LabelJSONPaths,
		HeaderTemplate: c.Auth.Capture.HeaderTemplate,
		DefaultScheme:  c.Auth.Capture.DefaultScheme,
	})
```

- [ ] **步骤 5：在 `pluginInvokerClosure` 成功路径加入捕获**

抽出共享函数（推荐放 `internal/connector/capture_after.go`）：

```go
package connector

func maybeCaptureLogin(conv string, ids identity.Store, cfg identity.CaptureConfig, toolName string, content map[string]any, isError bool) {
	if conv == "" || isError || ids == nil || !identity.MatchToolName(cfg.ToolNameGlob, toolName) {
		return
	}
	h, label, sub, claims, ok := identity.ExtractCredential(cfg, content)
	if !ok {
		return
	}
	_, _ = ids.Upsert(conv, identity.Identity{
		Label:             label,
		Scheme:            cfg.DefaultScheme,
		Subject:           sub,
		CredentialHeaders: h,
		Source:            identity.SourceLoginCapture,
		ClaimsSummary:     claims,
		IsDefault:         true,
	})
}
```

在 `openapiInvokerClosure` 用该函数替换内联块（行为不变）。

在 `pluginInvokerClosure` 两处成功返回前调用（含 `callbackURL` 企业回调分支与 `client.Invoke` 分支）：

```go
maybeCaptureLogin(conv, ctx.identities, ctx.capture, name, out.Content, out.IsError)
```

放在 `Touch` 之后、`return` 之前。

- [ ] **步骤 6：运行测试确认通过**

```powershell
go test ./internal/connector/ -count=1 -run "TestApplyHTTPPersistsAndCaptures|TestApplyOmitsCaptureUsesLoginGlob"
```

预期：PASS。再跑全包：

```powershell
go test ./internal/connector/ -count=1
```

- [ ] **步骤 7：Commit**

```powershell
git add internal/connector/apply.go internal/connector/register_one.go internal/connector/capture_after.go internal/connector/apply_test.go
git commit -m "feat(connector): HTTP 插件 Apply 持久化 capture 并在 invoke 后捕获登录"
```

---

### 任务 2：边界用例测试（`__none__` / 无会话 / is_error）

**文件：**
- 修改：`internal/connector/apply_test.go`（或新建 `apply_http_capture_test.go`）

- [ ] **步骤 1：编写失败测试**

```go
func TestApplyHTTPCaptureNoneDisables(t *testing.T) {
	// 同任务 1 的 httptest，login 返回 accessToken
	// Auth.Capture.ToolNameGlob = "__none__"
	// Apply 后 Invoke login：ids.List(conv) 必须为空
}

func TestApplyHTTPCaptureSkipsWithoutConversation(t *testing.T) {
	// 配置 *login* capture；无 WithConversationID 调用 login
	// ids 在任意 conv 下仍空（或 List("") 空）
}

func TestApplyHTTPCaptureSkipsOnToolError(t *testing.T) {
	// login invoke 返回 {"content":{},"is_error":true}
	// 有 conv 仍不写入身份
}
```

- [ ] **步骤 2：运行确认失败或已通过**

若任务 1 实现正确，部分可能已 PASS；未覆盖的补实现（通常无需再改生产代码）。

```powershell
go test ./internal/connector/ -count=1 -run "TestApplyHTTPCapture"
```

- [ ] **步骤 3：Commit**

```powershell
git add internal/connector/
git commit -m "test(connector): HTTP 插件登录捕获边界用例"
```

---

### 任务 3：`httpplugin.RegisterWithOpts` 对称捕获

**文件：**
- 修改：`internal/connector/httpplugin/register_opts.go`（增加 `Capture identity.CaptureConfig`）
- 修改：`internal/connector/httpplugin/register.go`（invoke 成功后捕获；匹配 glob 时 HITL 豁免与 openapi 一致）
- 测试：`internal/connector/httpplugin/register_test.go`

说明：生产主路径是 `connector.Apply`，但 `RegisterWithOpts` 仍有单测与可能的直接调用方，必须语义一致。

- [ ] **步骤 1：失败测试**

```go
func TestRegisterWithOptsCapturesLogin(t *testing.T) {
	// httptest：list login + ping；login 返回 accessToken
	// RegisterWithOpts(..., Capture: {ToolNameGlob:"*login*", TokenJSONPaths:["accessToken"], ...}, Identities, Resolver, RequireLogin: ["ping"])
	// conv 下 Invoke login → List 非空；Invoke ping → 请求头 Bearer
}
```

- [ ] **步骤 2：运行确认 FAIL**

```powershell
go test ./internal/connector/httpplugin/ -count=1 -run TestRegisterWithOptsCapturesLogin
```

- [ ] **步骤 3：实现**

`RegisterOpts` 增加字段 `Capture identity.CaptureConfig`。

在 `register.go` 注册循环中（对齐 openapi）：

```go
if !approval[name] && identity.MatchToolName(opts.Capture.ToolNameGlob, name) {
	needApproval = false
}
```

invoke 成功、`Touch` 之后：

```go
if conv != "" && !out.IsError && opts.Identities != nil && identity.MatchToolName(opts.Capture.ToolNameGlob, name) {
	if h, label, sub, claims, ok := identity.ExtractCredential(opts.Capture, out.Content); ok {
		_, _ = opts.Identities.Upsert(conv, identity.Identity{ /* 同 openapi */ })
	}
}
```

（可选：若不想跨包依赖 `connector.maybeCaptureLogin`，此处保持内联复制，与规格「小共享函数优先在 connector 包」不冲突。）

持久化：`RegisterWithOpts` 写入的 `store.Connector.Auth` 应包含 `opts.Auth` 已有 Capture；若调用方把 Capture 只放在 `opts.Capture`，写入 Store 时合并进 `Auth.Capture`。

- [ ] **步骤 4：测试通过并 Commit**

```powershell
go test ./internal/connector/httpplugin/ -count=1
git add internal/connector/httpplugin/
git commit -m "feat(httpplugin): RegisterWithOpts 支持登录捕获"
```

---

### 任务 4：示例侧车 `login` + 文档

**文件：**
- 修改：`examples/http-plugin/handler.go`、`handler_test.go`
- 修改：`docs/architecture-and-plugin-protocol.md`
- 修改：`README.md`、`README.zh-CN.md`
- 修改：`docs/superpowers/specs/2026-08-15-session-login-tool-gate-design.md`（交叉引用）

- [ ] **步骤 1：示例增加 `login` 工具**

在 `handleListTools` 的 tools 数组增加：

```go
{
	"name":        "login",
	"description": "Demo login; returns accessToken for Baize capture",
	"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
},
```

在 `handleInvoke` 对 `name == "login"`：

```go
writeJSON(w, http.StatusOK, map[string]any{
	"content": map[string]any{
		"accessToken": "demo-plugin-token",
		"email":       "demo@example.com",
	},
	"is_error": false,
})
return
```

补充 `handler_test.go`：GET tools 含 login；POST login invoke 含 accessToken。

- [ ] **步骤 2：文档**

架构草案 §4.4 或插件小节附近补一句：HTTP 插件 invoke 成功后亦可按 Connector `auth.capture` 写入会话身份（与 OpenAPI 同配置）。

README / README.zh-CN：
- 功能列表中「OpenAPI 可配置登录捕获」改为「OpenAPI / HTTP 插件 Connector 可配置登录捕获」
- 会话登录小节注明插件 `*login*` 工具同样可捕获

旧规格 `2026-08-15-session-login-tool-gate-design.md` 文首增加：

```markdown
> **更新（2026-08-26）：** HTTP 插件「不捕获」边界已由 `2026-08-26-http-plugin-login-capture-v0-design.md` 取代；实现以新规格为准。
```

- [ ] **步骤 3：跑相关测试**

```powershell
go test ./examples/http-plugin/ -count=1
go test ./internal/connector/ ./internal/connector/httpplugin/ -count=1
```

- [ ] **步骤 4：Commit**

```powershell
git add examples/http-plugin/ docs/architecture-and-plugin-protocol.md README.md README.zh-CN.md docs/superpowers/specs/2026-08-15-session-login-tool-gate-design.md
git commit -m "docs: HTTP 插件登录捕获说明与示例 login 工具"
```

---

### 任务 5：规格自检对照与收尾

- [ ] **步骤 1：对照规格 §成功标准 1–5 勾选**

| 规格项 | 任务 |
|--------|------|
| GET 回显 capture | 任务 1 |
| login → 身份 → require_login 头 | 任务 1 |
| `__none__` / 无会话 / is_error | 任务 2 |
| 凭证不进 events | 既有引擎红线；本里程碑不把 token 写入事件（捕获只写 Identity Store） |
| OpenAPI / 既有插件身份回归 | 任务 1 全包测试 |

- [ ] **步骤 2：可选更新本计划 checkbox 为完成；合并分支按 finishing 流程（用户指示后再做）**

---

## 自检（写作时）

1. **规格覆盖：** 持久化、invoke 捕获、HITL 豁免（经由 capture 传入 registerOne）、边界、RegisterWithOpts 对称、示例、文档、旧规格交叉引用——均有任务。  
2. **无占位符：** 步骤含具体代码与命令。  
3. **类型名：** `CaptureDefaults`、`ExtractCredential`、`SourceLoginCapture`、`RegisterOneFromConnector` 与代码库一致。  
4. **主路径提醒：** 生产走 `Apply`→`pluginInvokerClosure`；勿只改 `httpplugin.RegisterWithOpts`。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-26-http-plugin-login-capture-v0.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 任务间审查  
2. **内联执行** — 本会话按计划推进并设检查点  

选 **1** 还是 **2**？
