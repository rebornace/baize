# callback_urls 扩展 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** MCP `tools/call` 与企业 execution_callback 注入与 HTTP 侧车相同的签名 `callback_urls.event`，复用 `POST /v0/runs/{id}/plugin-callbacks` 接收端。

**架构：** 在 `plugincallback` 增加 `EventURL` 统一签发；`httpplugin.SignCallbackEventURL` 改为调用它；`mcp.CallTool` 通过 `_meta` 写入 `io.baize/*` 键；`executecallback.Client` body 增加 `callback_urls`；`register_one` 的 MCP / enterprise 闭包传入同一组 signer 配置。

**技术栈：** Go 1.25；`github.com/modelcontextprotocol/go-sdk/mcp`；现有 `plugincallback` / `httpplugin` / `executecallback`。

**规格：** `docs/superpowers/specs/2026-08-27-callback-urls-extension-v0-design.md`

**Git：** 建议分支 `feat/callback-urls-extension-v0`；过程提交进 `baize_real`。

**环境（Windows）：**
```powershell
$env:PATH = "$env:USERPROFILE\sdk\go\bin;" + $env:PATH
$env:GOTOOLCHAIN = "go1.25.0"
$env:GOPROXY = "https://goproxy.cn,direct"
```

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/plugincallback/event_url.go` | `EventURL(signer, secret, publicBase, ttl, runID) string` |
| `internal/plugincallback/event_url_test.go` | 签发/省略边界 |
| `internal/connector/httpplugin/register.go` | `SignCallbackEventURL` 委托 `plugincallback.EventURL` |
| `internal/connector/mcp/meta.go` | `io.baize/*` 常量 + `BuildCallMeta(runID, agentID, callbackEventURL) mcp.Meta` |
| `internal/connector/mcp/meta_test.go` | Meta 形状单测 |
| `internal/connector/mcp/session.go` | `CallTool` 接受 `CallToolOpts`（含 meta 字段） |
| `internal/connector/register_one.go` | `mcpInvokerClosure` 传 callback URL；`invokeEnterpriseCallback` 传 URL |
| `internal/connector/executecallback/client.go` | `InvokeMeta.CallbackEventURL` → body `callback_urls` |
| `internal/connector/executecallback/client_test.go` | body 含/不含 callback_urls |
| `internal/connector/mcp/callback_test.go`（或扩展现有测） | CallTool 带 meta 集成测 |
| `examples/enterprise-callback/main.go` | 演示可选 POST `callback_urls.event` |
| `docs/architecture-and-plugin-protocol.md`、README 中英 | 文档 |
| `docs/superpowers/specs/2026-08-27-callback-urls-extension-v0-design.md` | 状态 → 已批准 |

---

### 任务 1：`plugincallback.EventURL` + httpplugin 委托

**文件：**
- 创建：`internal/plugincallback/event_url.go`
- 创建：`internal/plugincallback/event_url_test.go`
- 修改：`internal/connector/httpplugin/register.go`

- [ ] **步骤 1：编写失败测试**

`event_url_test.go`：

```go
package plugincallback_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/plugincallback"
)

func TestEventURLIssuesWhenConfigured(t *testing.T) {
	url := plugincallback.EventURL(plugincallback.Issue, []byte("secret"), "https://runtime.example", time.Hour, "run_42")
	if url == "" {
		t.Fatal("want non-empty URL")
	}
	if !strings.HasPrefix(url, "https://runtime.example/v0/runs/run_42/plugin-callbacks?token=") {
		t.Fatalf("url=%q", url)
	}
}

func TestEventURLEmptyWhenMissingRunID(t *testing.T) {
	url := plugincallback.EventURL(plugincallback.Issue, []byte("secret"), "https://runtime.example", time.Hour, "")
	if url != "" {
		t.Fatalf("url=%q", url)
	}
}

func TestEventURLEmptyWhenMissingPublicBase(t *testing.T) {
	url := plugincallback.EventURL(plugincallback.Issue, []byte("secret"), "", time.Hour, "run_42")
	if url != "" {
		t.Fatalf("url=%q", url)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

```powershell
go test ./internal/plugincallback/... -run TestEventURL -count=1
```

预期：FAIL（`EventURL` undefined）

- [ ] **步骤 3：实现 `EventURL`**

`event_url.go`：

```go
package plugincallback

import (
	"strings"
	"time"
)

type Signer func(secret []byte, runID string, ttl time.Duration) (token string, exp time.Time, err error)

func EventURL(signer Signer, secret []byte, publicBase string, ttl time.Duration, runID string) string {
	if runID == "" || signer == nil || len(secret) == 0 || strings.TrimSpace(publicBase) == "" {
		return ""
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	token, _, err := signer(secret, runID, ttl)
	if err != nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	return FormatTokenURL(base, runID, token)
}
```

修改 `httpplugin/register.go` 中 `SignCallbackEventURL` 为：

```go
func SignCallbackEventURL(signer CallbackSigner, secret []byte, publicBase string, ttl time.Duration, runID string) string {
	return plugincallback.EventURL(plugincallback.Signer(signer), secret, publicBase, ttl, runID)
}
```

（保留 `defaultCallbackTTL` 逻辑：若 `ttl <= 0` 在调用前设为 default，或让 `EventURL` 内默认 1h 与现行为一致。）

- [ ] **步骤 4：运行测试验证通过**

```powershell
go test ./internal/plugincallback/... ./internal/connector/httpplugin/... -count=1
```

预期：PASS（含现有 `register_callback_test.go`）

- [ ] **步骤 5：Commit**

```powershell
git add internal/plugincallback/event_url.go internal/plugincallback/event_url_test.go internal/connector/httpplugin/register.go
git commit -m "refactor(plugincallback): 抽取 EventURL 供多路径复用"
```

---

### 任务 2：MCP `_meta` 注入

**文件：**
- 创建：`internal/connector/mcp/meta.go`
- 创建：`internal/connector/mcp/meta_test.go`
- 修改：`internal/connector/mcp/session.go`
- 修改：`internal/connector/register_one.go`

- [ ] **步骤 1：编写失败测试 — meta 构建**

`meta_test.go`：

```go
package mcp_test

import (
	"testing"

	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
)

func TestBuildCallMetaWithCallback(t *testing.T) {
	meta := mcpbridge.BuildCallMeta("run_1", "agent_1", "https://runtime.example/v0/runs/run_1/plugin-callbacks?token=abc")
	urls, ok := meta["io.baize/callback_urls"].(map[string]any)
	if !ok {
		t.Fatalf("meta=%+v", meta)
	}
	if urls["event"] != "https://runtime.example/v0/runs/run_1/plugin-callbacks?token=abc" {
		t.Fatalf("event=%v", urls["event"])
	}
	if meta["io.baize/run_id"] != "run_1" {
		t.Fatalf("run_id=%v", meta["io.baize/run_id"])
	}
}

func TestBuildCallMetaOmitsCallbackWhenEmpty(t *testing.T) {
	meta := mcpbridge.BuildCallMeta("run_1", "agent_1", "")
	if _, ok := meta["io.baize/callback_urls"]; ok {
		t.Fatalf("meta=%+v", meta)
	}
}
```

- [ ] **步骤 2：运行失败**

```powershell
go test ./internal/connector/mcp/... -run TestBuildCallMeta -count=1
```

- [ ] **步骤 3：实现 meta + 扩展 CallTool**

`meta.go`：

```go
package mcp

const (
	MetaKeyRunID        = "io.baize/run_id"
	MetaKeyAgentID      = "io.baize/agent_id"
	MetaKeyCallbackURLs = "io.baize/callback_urls"
)

func BuildCallMeta(runID, agentID, callbackEventURL string) map[string]any {
	meta := map[string]any{}
	if runID != "" {
		meta[MetaKeyRunID] = runID
	}
	if agentID != "" {
		meta[MetaKeyAgentID] = agentID
	}
	if callbackEventURL != "" {
		meta[MetaKeyCallbackURLs] = map[string]any{"event": callbackEventURL}
	}
	return meta
}
```

`session.go` — 增加：

```go
type CallToolOpts struct {
	RunID             string
	AgentID           string
	CallbackEventURL  string
}

func CallToolWithOpts(ctx context.Context, session *mcp.ClientSession, name string, arguments any, opts CallToolOpts) (*mcp.CallToolResult, error) {
	meta := BuildCallMeta(opts.RunID, opts.AgentID, opts.CallbackEventURL)
	params := &mcp.CallToolParams{Name: name, Arguments: arguments}
	if len(meta) > 0 {
		params.Meta = meta
	}
	// ... timeout + session.CallTool
}

// CallTool 保留为 CallToolWithOpts(ctx, session, name, arguments, CallToolOpts{})
func CallTool(ctx context.Context, session *mcp.ClientSession, name string, arguments any) (*mcp.CallToolResult, error) {
	return CallToolWithOpts(ctx, session, name, arguments, CallToolOpts{})
}
```

`register_one.go` — `mcpInvokerClosure` 内两处 `mcpbridge.CallTool` 改为：

```go
eventURL := httpplugin.SignCallbackEventURL(
	ctx.callbackSigner, ctx.callbackSecret, ctx.callbackPublicBase, ctx.callbackTTL, identity.RunIDFrom(c),
)
opts := mcpbridge.CallToolOpts{
	RunID:            identity.RunIDFrom(c),
	AgentID:          identity.AgentIDFrom(c),
	CallbackEventURL: eventURL,
}
result, err = mcpbridge.CallToolWithOpts(c, ctx.mcpSession, name, args, opts)
```

- [ ] **步骤 4：运行 MCP 包测试**

```powershell
go test ./internal/connector/mcp/... -count=1
```

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector/mcp/meta.go internal/connector/mcp/meta_test.go internal/connector/mcp/session.go internal/connector/register_one.go
git commit -m "feat(mcp): tools/call 注入 io.baize callback_urls"
```

---

### 任务 3：企业 execution_callback body 扩展

**文件：**
- 修改：`internal/connector/executecallback/client.go`
- 修改：`internal/connector/executecallback/client_test.go`
- 修改：`internal/connector/register_one.go`（`invokeEnterpriseCallback`）

- [ ] **步骤 1：编写失败测试**

在 `client_test.go` 追加：

```go
func TestClientInvokeIncludesCallbackURLs(t *testing.T) {
	var raw map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		_ = json.NewEncoder(w).Encode(map[string]any{"content": map[string]any{}, "is_error": false})
	}))
	defer srv.Close()

	const eventURL = "https://runtime.example/v0/runs/run_1/plugin-callbacks?token=tok"
	client := executecallback.NewClient(srv.URL)
	_, err := client.Invoke(context.Background(), "echo", nil, executecallback.InvokeMeta{
		RunID: "run_1", CallbackEventURL: eventURL,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	urls, ok := raw["callback_urls"].(map[string]any)
	if !ok || urls["event"] != eventURL {
		t.Fatalf("body=%+v", raw)
	}
}

func TestClientInvokeOmitsCallbackURLsWhenEmpty(t *testing.T) {
	var raw map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &raw)
		_ = json.NewEncoder(w).Encode(map[string]any{"content": map[string]any{}, "is_error": false})
	}))
	defer srv.Close()

	client := executecallback.NewClient(srv.URL)
	_, _ = client.Invoke(context.Background(), "echo", nil, executecallback.InvokeMeta{RunID: "run_1"})
	if _, ok := raw["callback_urls"]; ok {
		t.Fatalf("body=%+v", raw)
	}
}
```

- [ ] **步骤 2：运行失败**

```powershell
go test ./internal/connector/executecallback/... -run TestClientInvokeIncludes -count=1
```

- [ ] **步骤 3：实现**

`InvokeMeta` 增加 `CallbackEventURL string`。

`client.go` `Invoke` 内，在 `json.Marshal` 前：

```go
if meta.CallbackEventURL != "" {
	payload["callback_urls"] = map[string]any{"event": meta.CallbackEventURL}
}
```

`invokeEnterpriseCallback`：

```go
eventURL := httpplugin.SignCallbackEventURL(
	// 需要从 registerOneContext 传入 signer 字段 — 改函数签名为接收 ctx registerOneContext 或单独传 eventURL
)
```

**计划裁定：** 将 `invokeEnterpriseCallback` 改为接收 `callbackEventURL string` 参数；openapi/plugin 闭包调用前计算 `SignCallbackEventURL(...)`。

- [ ] **步骤 4：运行通过 + connector 相关测**

```powershell
go test ./internal/connector/executecallback/... ./internal/connector/... -count=1
```

（范围可收窄为 executecallback + apply 子集若全量过慢）

- [ ] **步骤 5：Commit**

```powershell
git add internal/connector/executecallback/client.go internal/connector/executecallback/client_test.go internal/connector/register_one.go
git commit -m "feat(executecallback): 企业回调 body 注入 callback_urls"
```

---

### 任务 4：企业回调示例 + API 端到端

**文件：**
- 修改：`examples/enterprise-callback/main.go`
- 可选：`internal/api/server_plugin_callback_test.go` 增加 enterprise 路径注释或复用现有 token 测

- [ ] **步骤 1：扩展 `examples/enterprise-callback`**

`executeBody` 增加：

```go
CallbackURLs struct {
	Event string `json:"event"`
} `json:"callback_urls"`
```

处理逻辑（异步 goroutine 或同步均可，示例用同步便于演示）：

```go
if body.CallbackURLs.Event != "" {
	go func(url string) {
		req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"type":"event","name":"enterprise.note","payload":{"ok":true}}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Baize-Protocol", "v0")
		http.DefaultClient.Do(req)
	}(body.CallbackURLs.Event)
}
```

README 或文件头注释一句：需配置 `runtime.public_base_url` 且 Run 带 `run_id`。

- [ ] **步骤 2：可选 API 集成测**

在 `server_plugin_callback_test.go` 新增 `TestPluginCallbackAcceptsEnterpriseIssuedToken`：用 `plugincallback.EventURL` 签发 token，POST 合法 body，断言事件追加。（与 HTTP 侧车测等价，文档化 enterprise/MCP 共用接收端。）

- [ ] **步骤 3：运行**

```powershell
go test ./internal/api/... -run TestPluginCallback -count=1
```

- [ ] **步骤 4：Commit**

```powershell
git add examples/enterprise-callback/main.go internal/api/server_plugin_callback_test.go
git commit -m "feat(examples): 企业回调演示 callback_urls 回投"
```

---

### 任务 5：文档与规格状态

**文件：**
- 修改：`docs/architecture-and-plugin-protocol.md`
- 修改：`README.md`、`README.zh-CN.md`
- 修改：`docs/superpowers/specs/2026-08-27-callback-urls-extension-v0-design.md`

- [ ] **步骤 1：架构 §4.2**

在 HTTP 侧车段落补充：MCP `tools/call` 的 `_meta.io.baize/callback_urls` 与企业 execution_callback body 的 `callback_urls` 使用同一 URL 与 token。

- [ ] **步骤 2：架构 §4.3**

企业回调 JSON 示例增加 `callback_urls.event` 字段（可选，有 run_id 且 Runtime 配置 public_base 时）。

- [ ] **步骤 3：README 中英**

各加一句：MCP / 企业执行回调亦可在 invoke 时收到 `callback_urls.event`，用于异步回投 Run 事件。

- [ ] **步骤 4：规格状态**

`> 状态：已批准（2026-08-27）`

- [ ] **步骤 5：全量相关测试**

```powershell
go test ./internal/plugincallback/... ./internal/connector/mcp/... ./internal/connector/executecallback/... ./internal/connector/httpplugin/... ./internal/api/... -count=1
```

- [ ] **步骤 6：Commit**

```powershell
git add docs/architecture-and-plugin-protocol.md README.md README.zh-CN.md docs/superpowers/specs/2026-08-27-callback-urls-extension-v0-design.md
git commit -m "docs: callback_urls 扩展至 MCP 与企业回调说明"
```

---

## 规格覆盖自检

| 规格需求 | 任务 |
|---------|------|
| 共享 EventURL | 1 |
| MCP `_meta` 注入 | 2 |
| 企业 body 注入 | 3 |
| 复用 plugin-callbacks 接收端 | 4（测 + 示例） |
| 无 run_id / 无 public_base 省略 | 1–3 测试 |
| OpenAPI 直连不注入 | 无任务（刻意不做） |
| HTTP 侧车回归 | 1 跑 httpplugin 全测 |
| 文档 | 5 |
| examples 演示 | 4 |

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-27-callback-urls-extension-v0.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 每个任务一个新子代理 + 任务间审查。必需子技能：`subagent-driven-development`。

**2. 内联执行** — 当前会话用 `executing-plans` 批量执行。必需子技能：`executing-plans`。

**选哪种方式？**
