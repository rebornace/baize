# MCP 导出（X1）v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 白泽作为 MCP Server（`/v0/mcp/export` Streamable HTTP），按导出策略暴露工具目录；多把可撤销专用 Key（必绑导出身份）；`tools/call` 经 Registry 注入身份；不耗白泽 LLM、不开 Run。

**架构：** 新包 `internal/mcpexport`（策略 / Key / 导出身份 / Streamable HTTP 门面）；管理 API 与挂载在 `internal/api`；工具行增加 `export` 字段；调用时用合成 `conversation_id=mcp-export-id:<identityID>` 将导出身份 Upsert 进既有 `identity.Store`，复用 OpenAPI/侧车 `require_login` 路径。

**技术栈：** Go 1.25、`github.com/modelcontextprotocol/go-sdk/mcp` v1.7、既有 store/tool/identity/controlplane、React 设置页。

**规格：** `docs/superpowers/specs/2026-08-29-mcp-export-v0-design.md`（已批准）

## Global Constraints

- 分支：`feat/mcp-export-v0`（从最新 `main`）；**不在 main 上直接实现**。
- 中文 Conventional Commits；PowerShell here-string 提交，不用 bash heredoc。
- Go：优先 `GOPROXY=https://goproxy.cn,direct`；PATH 含本机 `go`。
- UI 变更后：`cd web/chat; npm ci; npm run build`，提交 `internal/ui/dist/**`。
- 不做：stdio、OAuth、按 Key 工具集、Resources/Prompts、开 Run/调 LLM、改微信。
- Gate：`/v0/mcp/export` → ACL `RoleNone`（handler 内验导出 Key）；管理路由 → `RoleAdmin`。
- 与 `/settings/mcp`（客户端）文案/路由严格分离。

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/mcpexport/policy.go` | `ExportMode`、启发式、`AllowExport(tool, connectorMeta)` |
| `internal/mcpexport/policy_test.go` | 策略矩阵单测 |
| `internal/mcpexport/write_detect.go` | MCP 写工具名/描述启发式 |
| `internal/mcpexport/keys.go` | 生成/哈希/校验 Key（bcrypt 或 sha256+pepper；与项目现有 secret 习惯一致则跟随） |
| `internal/mcpexport/server.go` | `NewHandler(deps)` → Streamable HTTP；list/call |
| `internal/mcpexport/server_test.go` | httptest + `ConnectHTTP` 假客户端 |
| `internal/mcpexport/identity_bridge.go` | 合成 conv + Upsert 导出身份到 `identity.Store` |
| `internal/store/store.go` | `Tool.Export`；`MCPExportKey` / `MCPExportIdentity` 类型与 Store 方法；`Setting` 可选 |
| `internal/store/sqlite.go` / `postgres.go` / `memory.go` | 迁移与 CRUD |
| `internal/config/config.go` | `MCPExport.Enabled` 默认 true |
| `internal/controlplane/acl.go` | 新路由角色 |
| `internal/api/server_mcp_export.go` | 设置 CRUD + 挂载 export handler |
| `internal/api/server.go` | routes、Server 字段、PATCH body `export` |
| `internal/bootstrap/bootstrap.go` | 注入 deps、`mcp_export.enabled` |
| `web/chat/src/pages/McpExportSettings.tsx` | 新设置页 |
| `web/chat/src/pages/ToolsSettings.tsx` / `api.ts` / `settingsNav.ts` / `main.tsx` | 导出状态 UI + 导航 |
| `README.md` / `README.zh-CN.md` | MCP 导出章节 |
| `docs/architecture-and-plugin-protocol.md` | 一句：MCP 导出 v0 |
| `docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md` | X1 → 实现中/已交付 |

---

### 任务 0：分支

- [ ] **步骤 1**

```powershell
cd C:\Users\Administrator\Desktop\baize
git checkout main
git pull real main
git checkout -b feat/mcp-export-v0
```

---

### 任务 1：导出策略（TDD）

**Files:**
- Create: `internal/mcpexport/policy.go`
- Create: `internal/mcpexport/write_detect.go`
- Create: `internal/mcpexport/policy_test.go`

- [ ] **步骤 1：失败测试**

```go
package mcpexport_test

import (
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
	"github.com/rebornace/baize/internal/store"
)

func TestAllowExportMatrix(t *testing.T) {
	cases := []struct {
		name   string
		tool   store.Tool
		dbRO   bool // connector export_db_readonly
		want   bool
	}{
		{"get_default", store.Tool{Enabled: true, Method: "GET", Export: "default"}, false, true},
		{"head_default", store.Tool{Enabled: true, Method: "HEAD", Export: "default"}, false, true},
		{"post_default", store.Tool{Enabled: true, Method: "POST", Export: "default"}, false, false},
		{"post_force", store.Tool{Enabled: true, Method: "POST", Export: "force_allow"}, false, true},
		{"get_deny", store.Tool{Enabled: true, Method: "GET", Export: "force_deny"}, false, false},
		{"empty_method", store.Tool{Enabled: true, Method: "", Export: "default"}, false, false},
		{"empty_force", store.Tool{Enabled: true, Method: "", Export: "force_allow"}, false, true},
		{"disabled", store.Tool{Enabled: false, Method: "GET", Export: "default"}, false, false},
		{"approval", store.Tool{Enabled: true, Method: "GET", RequireApproval: true, Export: "default"}, false, false},
		{"mcp_write_force", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "insert_row", Description: "insert into t", Export: "force_allow"}, true, false},
		{"mcp_query", store.Tool{Enabled: true, Source: store.ToolSourceMCP, Name: "query", Description: "run select", Export: "default"}, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mcpexport.AllowExport(tc.tool, mcpexport.PolicyOpts{DBReadonlyConnector: tc.dbRO})
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
```

- [ ] **步骤 2：运行确认失败**

```powershell
go test ./internal/mcpexport/ -count=1
```

预期：包不存在或 `AllowExport` 未定义。

- [ ] **步骤 3：实现**

`Export` 空字符串视为 `default`。  
`IsMCPWriteTool(name, desc string) bool`：名称或描述（小写）含 `insert`/`update`/`delete`/`drop`/`truncate`/`execute_write` 等（实现时固定列表写在 `write_detect.go` 注释）。  
`DBReadonlyConnector==true` 且 `source=mcp`：写类 → false；非写且 `Export` 非 `force_deny` 时，空 method 允许（DB 工具无 HTTP method）。  
非 DB 的 mcp + 空 method：仍走「空 method 默认拒」，除非 `force_allow`。  
`RequireApproval==true` → 一律 false。

- [ ] **步骤 4：测试通过并提交**

```powershell
go test ./internal/mcpexport/ -count=1
git add internal/mcpexport
git commit -m @"
feat(mcpexport): 导出策略启发式与 MCP 写硬拒
"@
```

---

### 任务 2：Store — `Tool.Export` 字段

**Files:**
- Modify: `internal/store/store.go` — `Tool.Export string`（`default`|`force_allow`|`force_deny`，JSON `export`）
- Modify: `internal/store/sqlite.go`、`postgres.go`、`memory.go` — 列 `export_mode TEXT`（避免 SQL 关键字 `export`）、读写、迁移
- Modify: `internal/store/sqlite_test.go`（或 `store_test.go`）
- Modify: `internal/connector/catalog.go` — `MergeCatalog` **保留**已有 `Export`（与 Enabled/Title 同类）

- [ ] **步骤 1：失败测试** — SQLite round-trip `Export: "force_allow"`；缺列旧库 Open 后默认 `""`/`default`。

- [ ] **步骤 2：实现迁移与 Upsert/Get/List**

- [ ] **步骤 3：测试通过并提交**

```powershell
go test ./internal/store/ -count=1
git add internal/store internal/connector/catalog.go
git commit -m @"
feat(store): 工具目录增加 export 覆盖字段
"@
```

---

### 任务 3：Store — 导出身份与 Key CRUD

**Files:**
- Modify: `internal/store/store.go` 增加类型与接口方法：

```go
type MCPExportIdentity struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Scheme    string            `json:"scheme,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"` // 落盘；GET 管理 API 可脱敏策略与 auth 静态头一致
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type MCPExportKey struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	IdentityID string    `json:"identity_id"`
	KeyHash    string    `json:"-"` // 永不 JSON 出站
	Prefix     string    `json:"prefix"` // 前 8 字符提示
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store 新增：
// UpsertMCPExportIdentity / Get / List / DeleteMCPExportIdentity
// InsertMCPExportKey / GetMCPExportKey / ListMCPExportKeys / RevokeMCPExportKey
// LookupMCPExportKeyByHash(hash) (*MCPExportKey, error) // 或 Verify 在 mcpexport 包用 List+比较
```

- SQLite/PG：表 `mcp_export_identities`、`mcp_export_keys`  
- Memory：map 实现  

- [ ] **步骤 1：失败测试** — 创建身份 → 创建 Key（hash）→ List 无明文 → Revoke → Get 见 `RevokedAt`。

- [ ] **步骤 2：实现**

- [ ] **步骤 3：提交**

```powershell
go test ./internal/store/ -count=1
git add internal/store
git commit -m @"
feat(store): MCP 导出身份与 Key 持久化
"@
```

---

### 任务 4：Key 密码学与身份桥

**Files:**
- Create: `internal/mcpexport/keys.go`、`keys_test.go`
- Create: `internal/mcpexport/identity_bridge.go`、`identity_bridge_test.go`

- [ ] **步骤 1：Key API**

```go
func GenerateKey() (plaintext string, hash string, prefix string, err error)
func HashKey(plaintext string) string
func CheckKey(plaintext, hash string) bool
```

明文格式：`bze_` + 足够长随机（如 32 bytes hex）。哈希：`sha256` hex（或项目已有 secret hash 工具则复用）。

- [ ] **步骤 2：身份桥**

```go
const ExportConvPrefix = "mcp-export-id:"

func ConversationIDForIdentity(identityID string) string {
	return ExportConvPrefix + identityID
}

// EnsureExportIdentityInStore upserts id into identity.Store under ConversationIDForIdentity.
func EnsureExportIdentityInStore(ids identity.Store, export store.MCPExportIdentity) error

func InvokeContext(parent context.Context, ids identity.Store, export store.MCPExportIdentity) (context.Context, error) {
	if err := EnsureExportIdentityInStore(ids, export); err != nil {
		return parent, err
	}
	ctx := identity.WithConversationID(parent, ConversationIDForIdentity(export.ID))
	ctx = identity.WithForceIdentityID(ctx, export.ID)
	return ctx, nil
}
```

将 `export.Headers` 映射为 `identity.Identity{ID: export.ID, Scheme, CredentialHeaders: Headers, Source: "manual", Label: export.Name}`。

- [ ] **步骤 3：单测** — Generate/Check；Memory identity store + InvokeContext 后 `List(conv)` 含该身份。

- [ ] **步骤 4：提交**

```powershell
go test ./internal/mcpexport/ -count=1
git add internal/mcpexport
git commit -m @"
feat(mcpexport): 导出 Key 哈希与 require_login 身份桥
"@
```

---

### 任务 5：Streamable HTTP MCP Server 门面

**Files:**
- Create: `internal/mcpexport/server.go`、`server_test.go`

**Deps 结构：**

```go
type ServerDeps struct {
	Store     store.Store // 需能 ListTools / Get MCP export key+identity；若接口过大可拆小接口
	Registry  *tool.Registry
	Identities identity.Store
	Enabled   bool
	// ConnectorDBReadonly func(connectorID string) bool  — 读 Connector.MCP 或新字段；v0 可用：source=mcp 且名称/类型暗示 DB 时 true，或 Connector 上 bool ExportDBReadonly
}
```

规格：Connector 可标 `export_db_readonly`。在 `store.Connector` / `MCPConfig` 增加 `ExportDBReadonly bool`（任务 5 一并改 store 若任务 3 未加）。

- [ ] **步骤 1：失败集成测试**（同包或 `_test`）

仿 `internal/connector/mcp/client_http_test.go`：  
1. Memory store + Registry 注册一个 GET 工具 invoker  
2. 插入 export identity + key  
3. `httptest` 挂 `mcpexport.NewHTTPHandler(deps)`  
4. `ConnectHTTP` + Bearer（若 SDK 客户端 headers 支持；否则用自定义 RoundTripper 加头）  
5. `ListTools` 含该工具；`CallTool` 返回预期  
6. 无 Key / 撤销 Key → 连接或 call 失败  
7. POST 默认工具不在 list  

鉴权：在 `NewStreamableHTTPHandler` 外包裹 `http.HandlerFunc`，先校验 `Authorization: Bearer`，失败 `401`；通过后把 `keyID`/`identity` 放入 `context`，供 tool handler 使用。

动态工具：每次 `tools/list` 从 `Store.ListTools()` 过滤 `AllowExport`；`tools/call` 再查策略 + `Registry.Invoke(InvokeContext(...), name, args)`。  
若 go-sdk 要求启动时 `AddTool` 静态注册：可用「单通配代理 tool」不符合 MCP —— 应在 list 回调中动态返回。查阅 go-sdk `Server` 是否支持动态 list；若仅静态，则 **list 时按当前目录 AddTool 到 per-request Server 实例**（`NewStreamableHTTPHandler` 的 `func(*http.Request) *mcp.Server` 每次构建 Server 并注册当前可导出工具）。

- [ ] **步骤 2：实现 `NewHTTPHandler`**

- [ ] **步骤 3：测试通过并提交**

```powershell
go test ./internal/mcpexport/ -count=1
git add internal/mcpexport internal/store
git commit -m @"
feat(mcpexport): Streamable HTTP 导出门面 list/call
"@
```

---

### 任务 6：管理 API、ACL、Bootstrap 挂载

**Files:**
- Create: `internal/api/server_mcp_export.go`、`server_mcp_export_test.go`
- Modify: `internal/api/server.go` — `routes()`、Server 字段 `MCPExportEnabled`、`Identities` 已有则复用
- Modify: `internal/controlplane/acl.go` + `acl_test.go`
- Modify: `internal/config/config.go` — 

```yaml
mcp_export:
  enabled: true
```

- Modify: `internal/bootstrap/bootstrap.go` — 读配置、挂载 handler

**路由：**

| Method | Path | Role |
|--------|------|------|
| * | `/v0/mcp/export` | RoleNone（前缀匹配：acl 增加 `/v0/mcp/export` 与 `/v0/mcp/export/{*}` 若需要） |
| GET | `/v0/settings/mcp-export` | Admin |
| GET/POST | `/v0/settings/mcp-export/identities` | Admin |
| GET/PATCH/DELETE | `/v0/settings/mcp-export/identities/{id}` | Admin |
| GET/POST | `/v0/settings/mcp-export/keys` | Admin |
| DELETE | `/v0/settings/mcp-export/keys/{id}` | Admin |

POST keys body: `{ "name", "identity_id" }` → 201 `{ "id", "name", "identity_id", "token": "<plaintext once>", "prefix" }`  
POST 无 identity_id → 400。  
GET settings：`{ "enabled", "endpoint_path": "/v0/mcp/export" }`。

- [ ] **步骤 1：API 测试** — admin 创建身份与 Key；operator 403；用 token 调 export（可 httptest 全 Server）；Gate token 调 export → 401。

- [ ] **步骤 2：实现 + ACL**

注意：`authorize` 对 RoleNone **不剥离**？核对现有 Inbox：RoleNone 时不跑 Bearer Gate。Export handler 自己读 `Authorization`。若中间件仍剥离，需与 Inbox 同样处理——**读 `server.go` authorize，保证 RoleNone 保留 Authorization 头。**

- [ ] **步骤 3：提交**

```powershell
go test ./internal/api/ ./internal/controlplane/ ./internal/mcpexport/ -count=1
git add internal/api internal/controlplane internal/config internal/bootstrap
git commit -m @"
feat(api): MCP 导出设置与 /v0/mcp/export 挂载
"@
```

---

### 任务 7：PATCH `/v0/tools/{name}` 的 `export` + Tools UI

**Files:**
- Modify: `internal/api/server.go` `handlePatchTool` body 增加 `Export *string`；校验枚举；`UpsertTool`；**不必**因 export 变更重注册 Registry
- Modify: `web/chat/src/api.ts` — `ToolInfo.export`；`patchTool` 参数
- Modify: `web/chat/src/pages/ToolsSettings.tsx` — 下拉：默认 / 强制允许 / 强制拒绝
- 测试：`server` 既有 patch 测试扩展

- [ ] **步骤 1–4：TDD 后端 → UI → build dist → 提交**

```powershell
go test ./internal/api/ -count=1 -run Patch
cd web/chat; npm ci; npm test -- --run; npm run build
cd ../..
git add internal/api web/chat internal/ui/dist
git commit -m @"
feat(ui): Tools 页可配置 MCP 导出覆盖
"@
```

---

### 任务 8：设置页「MCP 导出」

**Files:**
- Create: `web/chat/src/pages/McpExportSettings.tsx`（+ 单测若有纯函数）
- Modify: `settingsNav.ts`、`main.tsx`（AdminOnly）、`api.ts`、`settingsNav.test.ts`
- Build `internal/ui/dist`

页内容：启用状态展示、endpoint 复制、身份列表/创建（headers 用 `parseKeyValueLines`）、Key 列表/创建（展示一次性 token 对话框）、撤销。

- [ ] **步骤 1：实现 UI + 测试导航含「MCP 导出」**

- [ ] **步骤 2：build + 提交**

```powershell
cd web/chat; npm test -- --run; npm run build
cd ../..
git add web/chat internal/ui/dist
git commit -m @"
feat(ui): MCP 导出设置页（身份与 Key）
"@
```

---

### 任务 9：文档与边界笔记

**Files:**
- Modify: `README.md`、`README.zh-CN.md` — 新节「MCP 导出」：与「MCP 桥（客户端）」对照；Cursor 配置示例；只读策略；专用 Key；不耗白泽 LLM；HTTPS
- Modify: `docs/architecture-and-plugin-protocol.md` — MCP 导出一句
- Modify: `docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md` — X1 实现中/已交付

- [ ] **步骤 1：撰写并提交**

```powershell
git add README.md README.zh-CN.md docs
git commit -m @"
docs: MCP 导出（X1）使用说明
"@
```

---

### 任务 10：全量验证

- [ ] `go test ./... -count=1`
- [ ] `cd web/chat; npm test -- --run`
- [ ] 手工清单（记入 PR/提交说明）：创建身份与 Key → Cursor 或假客户端 list/call → 撤销 Key → POST 工具默认不可见 → `require_login` 工具用导出身份头成功（可用 mock HTTP 上游）
- [ ] finishing：合并策略按 `finishing-a-development-branch`（用户选合并/PR）；双仓同步仅在用户要求时执行

---

## Self-Review（对照规格）

| 规格 | 任务 |
|------|------|
| 方案 A 目录门面 | 5 |
| Streamable HTTP `/v0/mcp/export` | 5–6 |
| 专用多 Key + 必绑身份 | 3–4、6、8 |
| 导出策略 + 手动覆盖 + 库写硬拒 | 1–2、7 |
| 导出身份设置维护 + require_login | 3–4、6、8 |
| 不调 LLM / 不开 Run | 5（仅 Registry.Invoke） |
| Gate 分离 RoleNone | 6 |
| UI 与文档 | 7–9 |
| 测试 | 1–6、10 |

无 TBD；身份桥用合成 conversation_id，避免改所有 invoker 闭包。

---

## 执行交接

计划已完成并保存到 `docs/superpowers/plans/2026-08-29-mcp-export-v0.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 每个任务新子代理，任务间审查  

**2. 内联执行** — 本会话用 executing-plans，批量推进并设检查点  

选哪种方式？
