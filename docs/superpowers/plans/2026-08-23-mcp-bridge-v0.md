# MCP 桥 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** Runtime 可作为 MCP 客户端注册 `type: mcp` Connector（stdio 子进程 + Streamable HTTP），发现工具写入目录 `source=mcp`，Run 内 `tools/call`；设置页 MCP 可管理；compose 示例仅 DB（DBHub + Postgres）；搜索 MCP 仅文档说明。

**架构：** 新包 `internal/connector/mcp` 使用官方 `github.com/modelcontextprotocol/go-sdk/mcp`。每个 stdio Connector 在 Registry 层持有一条长驻 `ClientSession`（`CommandTransport`）；HTTP Connector 用 Streamable HTTP 传输按 URL 调用。`connector.Apply` 分流 `type=mcp`，发现失败返回 `ErrInvalidMCP` → API `400 invalid_mcp`。`registerOne` 增加 `mcpInvokerClosure`，HITL / `require_login` 语义与 HTTP 插件对齐（MCP 忽略 `auth.mode`，鉴权在 `mcp.env` / `mcp.headers`）。

**技术栈：** Go 1.22+、`github.com/modelcontextprotocol/go-sdk/mcp`、httptest、React + Vite、现有 store / connector / controlplane。

**规格：** `docs/superpowers/specs/2026-08-23-mcp-bridge-v0-design.md`（已批准）

**全局约束：**
- 不在 `main` 上改实现：先 `git checkout -b feat/mcp-bridge`
- 不做内置 DB 驱动、搜索 compose、MCP 市场、Webhook（`DELETE /v0/connectors/{id}` 已实现，见 `docs/superpowers/specs/2026-08-26-connector-delete-and-callback-urls-v0-design.md`）
- 不修改 `configs/minimal.yaml` 默认（生产 start 不带 MCP）
- commit 中文 `type(scope): 说明`；PowerShell 不用 bash HEREDOC
- Go：`$env:GOPROXY='https://goproxy.cn,direct'`；`$env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH`
- UI 变更后：`cd web/chat && npm ci && npm run build`，提交 `internal/ui/dist/**`
- 集成测试 mock MCP 用仓库内 `examples/mcp-mock`（Go 编译二进制），**不**依赖 npx/Node 跑 CI

**实现选定（相对规格）：**
- stdio 会话：每个 Connector id 在 `SessionPool` 中单例；`PUT` 同 id 先 `Close` 再起新会话
- HTTP：v0 使用 go-sdk 的 Streamable HTTP 客户端（`StreamableClientTransport` 或 SDK 当前等价 API）
- GET connector：`mcp.env` / `mcp.headers` 回显时 `${VAR}` / `env:` 引用保留形状，不展开明文（与 `auth.static` 一致）
- compose 示例：Baize 容器内 **不** 装 Node；Postgres + 文档中 `PUT` 片段；DBHub 由用户在宿主机 `npx` 或单独文档说明（compose 只起 Postgres + Baize，避免 CI 拉 Node）

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/store/store.go` | `ToolSourceMCP`、`Connector.MCP`（`MCPConfig`） |
| `internal/store/sqlite.go` | `mcp_json` 列、读写、`migrateConnectorsColumns` |
| `internal/store/memory.go` | 内存 Connector MCP 字段 |
| `internal/connector/mcp/config.go` | `MCPConfig`、`ResolveEnv`（复用 authcred 解析） |
| `internal/connector/mcp/session.go` | stdio `CommandTransport` 会话池 |
| `internal/connector/mcp/client_http.go` | HTTP list/call |
| `internal/connector/mcp/discover.go` | MCP tool → `store.Tool` |
| `internal/connector/mcp/errors.go` | `ErrInvalidMCP` |
| `internal/connector/mcp/*_test.go` | stdio + HTTP mock |
| `internal/connector/apply.go` | `type=mcp` 发现分支 |
| `internal/connector/register_one.go` | `mcpInvokerClosure`、`registerOneContext.mcpSession` |
| `internal/connector/catalog.go` | 合并时 `ToolSourceMCP` 与 spec/plugin 同等保护 |
| `internal/api/server.go` | PUT body `mcp`、GET 回显、`invalid_mcp` |
| `internal/config/config.go` | （可选）bootstrap 不预置 mcp |
| `examples/mcp-mock/cmd/mcp-mock/main.go` | 集成测试用 stdio MCP（单 tool `echo`） |
| `tests/integration/mcp_bridge_test.go` | PUT→GET tools→Run |
| `web/chat/src/pages/McpSettings.tsx` | 设置页 |
| `web/chat/src/api.ts` | `putConnector` / `getConnector` MCP 类型 |
| `web/chat/src/main.tsx` | 路由替换 ComingSoon |
| `docker-compose.mcp-demo.yml` | Postgres demo（无 DBHub 容器） |
| `README.md` / `README.zh-CN.md` | MCP 章节 + DB compose + 搜索自行接入 |
| `docs/architecture-and-plugin-protocol.md` | MCP 桥已实现 |

---

### 任务 0：功能分支

- [ ] **步骤 1：** `git checkout -b feat/mcp-bridge`
- [ ] **步骤 2：** `go get github.com/modelcontextprotocol/go-sdk@latest`；`go mod tidy`

---

### 任务 1：Store — `mcp_json` 与 `ToolSourceMCP`

**文件：**
- 修改：`internal/store/store.go`
- 修改：`internal/store/memory.go`
- 修改：`internal/store/sqlite.go`
- 修改：`internal/store/store_test.go`
- 测试：`internal/store/sqlite_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `sqlite_test.go` 追加：

```go
func TestSQLiteConnectorMCPJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.db")
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := store.Connector{
		ID:   "m1",
		Type: "mcp",
		MCP: store.MCPConfig{
			Transport: "stdio",
			Command:   "echo",
			Args:      []string{"ok"},
			Env:       map[string]string{"K": "v"},
		},
	}
	s.UpsertConnector(c)
	got, err := s.GetConnector("m1")
	if err != nil {
		t.Fatal(err)
	}
	if got.MCP.Transport != "stdio" || got.MCP.Command != "echo" {
		t.Fatalf("mcp=%+v", got.MCP)
	}
}
```

- [ ] **步骤 2：** 运行测试确认失败

`go test ./internal/store -run TestSQLiteConnectorMCPJSONRoundTrip -count=1`

- [ ] **步骤 3：实现**

`store.go`：

```go
const ToolSourceMCP = "mcp"

type MCPConfig struct {
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

type Connector struct {
	// ... existing fields ...
	MCP MCPConfig `json:"mcp,omitempty"`
}
```

`sqlite.go`：`connectors` 表读写增加 `mcp_json`；`migrateConnectorsColumns` 对旧库 `ALTER TABLE connectors ADD COLUMN mcp_json TEXT`（忽略 duplicate）。

- [ ] **步骤 4：** `go test ./internal/store -count=1` 全绿

- [ ] **步骤 5：Commit** `feat(store): Connector MCP 配置与 source=mcp`

---

### 任务 2：mock MCP Server（集成测试用）

**文件：**
- 创建：`examples/mcp-mock/main.go`

- [ ] **步骤 1：实现最小 stdio Server**

使用 go-sdk 注册 tool `echo`：`arguments.message` → content `{message: ...}`。

```go
// examples/mcp-mock/main.go — 仅用于 tests/integration 与本地调试
mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echo"}, echoHandler)
server.Run(ctx, &mcp.StdioTransport{})
```

- [ ] **步骤 2：** `go build -o bin/mcp-mock ./examples/mcp-mock` 确认可编译

- [ ] **步骤 3：Commit** `chore(examples): mcp-mock stdio 服务供集成测试`

---

### 任务 3：MCP 客户端包（stdio 会话 + HTTP）

**文件：**
- 创建：`internal/connector/mcp/errors.go`
- 创建：`internal/connector/mcp/config.go`
- 创建：`internal/connector/mcp/session.go`
- 创建：`internal/connector/mcp/client_http.go`
- 创建：`internal/connector/mcp/discover.go`
- 创建：`internal/connector/mcp/session_test.go`
- 创建：`internal/connector/mcp/client_http_test.go`

- [ ] **步骤 1：编写失败的 stdio 测试**

`session_test.go`：构建 `go build` 出的 `../../bin/mcp-mock`（测试里用 `runtime.GOOS` 路径或 `go test` 前编译一次），`DiscoverTools` 应含 `echo`。

- [ ] **步骤 2：实现 `SessionPool`**

- `OpenStdio(ctx, connectorID, command, args, env)` → `*mcp.ClientSession`
- `Close(connectorID)`、`CloseAll()`
- `ListTools` / `CallTool` 包装 30s 超时

- [ ] **步骤 3：HTTP 测试**

`client_http_test.go`：httptest 返回 MCP Streamable HTTP 最小 JSON（或启动 go-sdk test server）；`DiscoverTools` + `CallTool` 成功。

- [ ] **步骤 4：** `go test ./internal/connector/mcp -count=1`

- [ ] **步骤 5：Commit** `feat(mcp): stdio 会话池与 HTTP 客户端`

---

### 任务 4：Apply + Registry invoke

**文件：**
- 修改：`internal/connector/apply.go`
- 修改：`internal/connector/register_one.go`
- 修改：`internal/connector/catalog.go`
- 测试：`internal/connector/apply_mcp_test.go`

- [ ] **步骤 1：编写失败的 Apply 测试**

`PUT` 等价：直接调 `connector.Apply`，`Type: mcp`，stdio 指向 `examples/mcp-mock` 构建路径；期望 `ListTools` 含 `source=mcp` 行；Registry `echo` 可调用。

- [ ] **步骤 2：apply.go `case "mcp"`**

- 校验 transport / command / url
- `mcp.ResolveEnv(cfg.Env)`、`ResolveHeaders(cfg.Headers)`
- stdio：`pool.OpenStdio` → `DiscoverTools` → 空列表 → `ErrInvalidMCP`
- http：`DiscoverToolsHTTP`
- 发现行 `Source: store.ToolSourceMCP`
- 注册阶段把 `session` 或 `httpClient` 传入 `registerOneContext`

- [ ] **步骤 3：register_one.go**

- `mcpInvokerClosure`：`CallTool`；失败 `is_error`；`require_login` 对 MCP 工具仍走门闸（通常无 identity 需求，默认公开）
- `RegisterOneFromConnector`：`type=mcp` 时拒绝 `extra`

- [ ] **步骤 4：catalog.go**

合并删除/保留：`ToolSourceMCP` 与 plugin/spec 同等（不可手加删）

- [ ] **步骤 5：** 失败路径测试：坏 command → Apply 错误且 Registry 无新工具

- [ ] **步骤 6：** `go test ./internal/connector -count=1`

- [ ] **步骤 7：Commit** `feat(connector): Apply 与 invoke 支持 type=mcp`

---

### 任务 5：HTTP API

**文件：**
- 修改：`internal/api/server.go`
- 测试：`internal/api/server_mcp_test.go`

- [ ] **步骤 1：扩展 `handlePutConnector` body**

```go
MCP store.MCPConfig `json:"mcp"`
```

`type=mcp` 时忽略 `auth`；错误映射 `errors.Is(err, mcp.ErrInvalidMCP)` → `400 invalid_mcp`。

- [ ] **步骤 2：`handleGetConnector` 回显 `mcp`**

- [ ] **步骤 3：httptest**

PUT stdio mcp → 200 + tools；坏配置 → 400 `invalid_mcp`。

- [ ] **步骤 4：Commit** `feat(api): PUT/GET mcp Connector 与 invalid_mcp`

---

### 任务 6：集成测试

**文件：**
- 创建：`tests/integration/mcp_bridge_test.go`

- [ ] **步骤 1：** 编译 `mcp-mock`；`bootstrap.StartForTest` 或 HTTP API PUT
- [ ] **步骤 2：** `GET /v0/tools` 含 `echo`；`POST /v0/runs` mock LLM 调用 echo；events 有 `tool.result`
- [ ] **步骤 3：** `go test ./tests/integration -run MCP -count=1`
- [ ] **步骤 4：Commit** `test(integration): MCP stdio 注册与 Run 调用`

---

### 任务 7：设置页 MCP

**文件：**
- 修改：`web/chat/src/api.ts`
- 创建：`web/chat/src/pages/McpSettings.tsx`
- 修改：`web/chat/src/main.tsx`
- 测试：`web/chat/src/pages/McpSettings.test.ts`（表单校验纯函数，可选）

- [ ] **步骤 1：`api.ts`**

```ts
export interface MCPConfig {
  transport: 'stdio' | 'http'
  command?: string
  args?: string[]
  env?: Record<string, string>
  url?: string
  headers?: Record<string, string>
}
export async function putConnector(id: string, body: { type: string; mcp?: MCPConfig; require_approval?: string[] }): Promise<ConnectorInfo>
export async function getConnector(id: string): Promise<ConnectorInfo>
```

- [ ] **步骤 2：`McpSettings.tsx`**

- 列表：从 `listTools()` 过滤 `source===mcp` 聚合 `connector_id`，再 `getConnector`
- 表单：transport 切换 stdio/http 字段；保存 `PUT`；错误展示 `error.code`
- 链到 `/settings/tools`

- [ ] **步骤 3：`main.tsx`** `/settings/mcp` → `McpSettings`

- [ ] **步骤 4：** `npm test`；`npm run build`；提交 `internal/ui/dist/**`

- [ ] **步骤 5：Commit** `feat(ui): MCP 设置页`

---

### 任务 8：compose + 文档

**文件：**
- 创建：`docker-compose.mcp-demo.yml`
- 修改：`README.md`、`README.zh-CN.md`
- 修改：`docs/architecture-and-plugin-protocol.md`

- [ ] **步骤 1：`docker-compose.mcp-demo.yml`**

服务：`postgres`（初始化脚本 `examples/mcp-demo/init.sql` 可选）、`baize`（build Dockerfile，不预置 MCP connector）

- [ ] **步骤 2：README 章节**

- MCP 概念、stdio/HTTP PUT 示例
- DBHub：用户宿主机 `npx @bytebase/dbhub` + DSN 指向 compose Postgres
- **搜索 MCP 仅文档**：Tavily 远程 URL、Brave stdio 命令片段；无 compose
- `docker compose -f docker-compose.mcp-demo.yml up`

- [ ] **步骤 3：架构 §2** MCP 桥标为已实现

- [ ] **步骤 4：Commit** `docs: MCP 桥 v0 说明与 mcp-demo compose`

---

### 任务 9：全量回归与 finishing

- [ ] **步骤 1：** `go test ./... -count=1`
- [ ] **步骤 2：** `baize demo` 路径集成测试仍绿（`tests/integration` 全量）
- [ ] **步骤 3：** 按 `finishing-a-development-branch` 合并前自检（可选 PR 到 `main`）

---

## 规格覆盖自检

| 规格需求 | 任务 |
|----------|------|
| stdio + HTTP | 任务 3、4、5 |
| `source=mcp` | 任务 1、4 |
| `invalid_mcp` / 不污染 Registry | 任务 4、5、6 |
| HITL `require_approval` | 任务 4 |
| 设置页 MCP | 任务 7 |
| 无搜索 compose | 任务 8 仅文档 |
| DBHub 文档 + postgres compose | 任务 8 |
| minimal 无预置 MCP | 全局约束 |
| admin ACL | 沿用现网 PUT connector |
| 集成 mock MCP | 任务 2、6 |

无 TODO/待定占位。

---

## 执行方式

计划已保存。两种执行方式：

1. **子代理驱动（推荐）** — 每任务新子代理 + 审查（与 Agent Skills 相同）
2. **内联执行** — 本会话按任务顺序实现，每 2～3 任务汇报

请回复 **1** 或 **2** 开始实现；或指定先只做后端（任务 1～6）再 UI。
