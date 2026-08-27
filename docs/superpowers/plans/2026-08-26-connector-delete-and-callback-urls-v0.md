# Connector 整删与侧车 callback_urls v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** Phase 1 实现整 Connector 硬删级联；Phase 2 为 HTTP 插件 invoke 注入签名 `callback_urls.event` 并接收 `plugin.callback` 事件。

**架构：** Store 级联删除 + Registry 注销 → 三设置页调用 DELETE；独立 `plugincallback` 包签发/校验 HMAC token → httpplugin 注入 URL → 无会话 ACL 的回调端点 AppendEvent。

**技术栈：** Go 1.25、现有 SQLite Store、Vite/React Chat UI、标准库 `crypto/hmac`。

**规格：** `docs/superpowers/specs/2026-08-26-connector-delete-and-callback-urls-v0-design.md`（已批准）  
**分支：** `feat/connector-delete-callback-urls`  
**全局约束：** commit 中文 Conventional；改 UI 后 `npm test` + `npm run build` 并提交 `internal/ui/dist/**`；**先完成 Phase 1 全部任务再开 Phase 2**。

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/store/store.go` | `Store` 增加 `DeleteConnector(id string) error` |
| `internal/store/sqlite.go` / `memory.go` | 事务/内存级联删 tools + connector |
| `internal/store/*_test.go` | 级联与 404 语义 |
| `internal/api/server.go` | `DELETE /v0/connectors/{id}`；`POST .../plugin-callbacks` |
| `internal/api/server_connector_delete_test.go` | Phase 1 API 测 |
| `internal/api/server_plugin_callback_test.go` | Phase 2 回调测 |
| `internal/controlplane/acl.go` + `acl_test.go` | DELETE connector → Admin；plugin-callbacks → RoleNone |
| `web/chat/src/api.ts` | `deleteConnector(id)` |
| `web/chat/src/pages/{OpenApi,Plugin,Mcp}Settings.tsx` | 删除按钮 + 二次确认 |
| `web/chat/src/pages/*Settings*.test.ts(x)` | 确认取消不请求等 |
| `internal/ui/dist/**` | 嵌入构建产物 |
| `internal/plugincallback/token.go` | HMAC 签发/校验（run_id、exp） |
| `internal/plugincallback/token_test.go` | token 单测 |
| `internal/plugincallback/limiter.go` | 每 run 计数限流（内存） |
| `internal/config/config.go` | `Runtime.PublicBaseURL`、`Runtime.CallbackHMACSecret`、TTL |
| `internal/connector/httpplugin/client.go` | context 注入 `callback_urls` |
| `internal/connector/httpplugin/register.go` / opts | 传入 PublicBase + Signer |
| `internal/bootstrap/bootstrap.go` | 装配 public base、secret、注入 Server/httpplugin |
| `configs/*.yaml` / example | `runtime.public_base_url` 注释 |
| `docs/architecture-and-plugin-protocol.md` | §4.2 标已实现 |
| `README.md` / `README.zh-CN.md` | DELETE + callback 一句 |
| 相关 specs「不做 DELETE」行 | 改为已实现 / 指向本规格 |

---

# Phase 1 — Connector 整删

### 任务 1：Store `DeleteConnector` 级联

**文件：**
- 修改：`internal/store/store.go`（接口）
- 修改：`internal/store/sqlite.go`、`memory.go`
- 修改/创建：对应 `*_test.go`

- [ ] **步骤 1：失败测试**

```go
func TestDeleteConnectorCascadesTools(t *testing.T) {
	// UpsertConnector + UpsertTool(s) under id "c1"
	// DeleteConnector("c1") → nil
	// GetConnector("c1") → err
	// ListToolsByConnector("c1") → empty
	// DeleteConnector("missing") → err (not found)
}
```

- [ ] **步骤 2：** `go test ./internal/store -count=1 -run DeleteConnector` → FAIL

- [ ] **步骤 3：实现**  
  - SQLite：事务内 `DELETE FROM tools WHERE connector_id=?` 再 `DELETE FROM connectors WHERE id=?`；影响行 0 → not found  
  - Memory：删 map 条目 + 过滤 tools  
  - 所有实现 Store 的 fake（测试 double）补方法，保证编译

- [ ] **步骤 4：** 测试 PASS

- [ ] **步骤 5：Commit** `feat(store): DeleteConnector 级联删除 tools`

---

### 任务 2：API `DELETE /v0/connectors/{id}` + ACL

**文件：**
- 修改：`internal/api/server.go`（路由 + `handleDeleteConnector`）
- 修改：`internal/controlplane/acl.go`、`acl_test.go`
- 创建：`internal/api/server_connector_delete_test.go`

- [ ] **步骤 1：失败测试**

```go
// Admin 删除已存在 connector → 204
// 之后 GET connector → 404；List tools 无该 connector_id；Registry 无其工具名
// 删除不存在 → 404 connector_not_found
// Operator token → 403（门禁开时）
```

对齐现有 `handleDeleteConnectorTool`：成功 **`204 No Content`**。

处理顺序（规格）：
1. `GetConnector` 失败 → 404 `connector_not_found`  
2. `Registry.UnregisterConnector(id)`  
3. `Store.DeleteConnector(id)`  

- [ ] **步骤 2：** 测试 FAIL

- [ ] **步骤 3：实现 handler + ACL 规则**  
  `DELETE` + `["v0","connectors","{id}"]` → `RoleAdmin`  
  （注意与 `DELETE .../tools/{name}` 规则共存；更长路径优先匹配已有逻辑）

- [ ] **步骤 4：** `go test ./internal/api ./internal/controlplane -count=1` 相关 PASS

- [ ] **步骤 5：Commit** `feat(api): DELETE /v0/connectors/{id} 整删级联`

---

### 任务 3：设置页删除 UI

**文件：**
- 修改：`web/chat/src/api.ts` — `deleteConnector(id: string): Promise<void>`（对标 `deleteConnectorTool`，期望 204）
- 修改：`OpenApiSettings.tsx`、`PluginSettings.tsx`、`McpSettings.tsx`
- 测试：各页或共享 helper 测试（确认对话框：取消不 fetch；确认调用 DELETE）
- `npm test`；`npm run build`；提交 `internal/ui/dist/**`

- [ ] **步骤 1：** 为删除确认逻辑写失败测试（若页内联难测，抽 `confirmDeleteConnector(id): string` 文案纯函数 + 测；点击流用现有测试风格）

- [ ] **步骤 2：** 实现按钮 + `window.confirm`（或现有 modal 模式，与仓库一致）文案含 id 与「将移除全部工具」

- [ ] **步骤 3：** `npm test`；`npm run build`

- [ ] **步骤 4：Commit** `feat(ui): Connector 设置页整删与二次确认`

---

### 任务 4：Phase 1 文档交叉引用

**文件：**
- `docs/superpowers/specs/2026-08-23-openapi-settings-v0-design.md`（「不做 DELETE」→ 指向本规格已实现）
- 若 plugin/mcp 设置规格有同类「不做」，同步改  
- README 中英控制面/设置：可删整 Connector 一句（可与任务 8 合并；本任务至少改规格交叉引用）

- [ ] **步骤 1：改文档**

- [ ] **步骤 2：Commit** `docs: Connector 整删已实现交叉引用`

**Phase 1 关卡：** `go test ./internal/store ./internal/api ./internal/controlplane -count=1` 与 `web/chat` npm test 全绿后再进入 Phase 2。

---

# Phase 2 — callback_urls

### 任务 5：`plugincallback` token + 配置

**文件：**
- 创建：`internal/plugincallback/token.go`、`token_test.go`
- 创建：`internal/plugincallback/limiter.go`、`limiter_test.go`（可选同包）
- 修改：`internal/config/config.go` + defaults

```go
// config
type RuntimeConfig struct {
	PublicBaseURL       string `yaml:"public_base_url"`
	CallbackHMACSecret  string `yaml:"callback_hmac_secret"` // 空则启动时生成并只打日志「ephemeral」
	CallbackTokenTTLSec int    `yaml:"callback_token_ttl_sec"` // 默认 3600
}

// plugincallback
func Issue(secret []byte, runID string, ttl time.Duration) (token string, exp time.Time, err error)
func Verify(secret []byte, runID, token string, now time.Time) error
```

Token 格式建议（写死一种并测）：`base64url(expUnix)|base64url(hmac-sha256(secret, runID+"|"+expUnix))` 或单段 signed payload；**必须** Verify 时比较 path 的 `run_id` 与 token 内绑定一致。

Limiter：`Allow(runID string, now time.Time) bool` — 默认 100/小时/run，滑动或固定窗口均可，单测覆盖超限。

- [ ] **步骤 1–4：** TDD Issue/Verify（过期、篡改、错 run）+ Limiter；Commit `feat(plugincallback): HMAC token 与每 Run 限流`

---

### 任务 6：httpplugin 注入 `callback_urls`

**文件：**
- 修改：`InvokeMeta` 增加可选 `CallbackEventURL string`（或 `CallbackURLs map`）
- 修改：`client.go`：非空则写入 `context.callback_urls.event`
- 修改：`register.go`：有 `run_id` 且 Signer+PublicBase 可用时 Issue 并拼 URL  
  `{publicBase}/v0/runs/{runID}/plugin-callbacks?token={token}`  
- 无 run_id 或无 publicBase/secret → **省略** `callback_urls`  
- 测试：`TestClientInvokeSendsContextAndHeaders` 扩展或新测断言 JSON 含/不含 callback_urls

PublicBase 规范化：去尾 `/`。

- [ ] TDD → 实现 → Commit `feat(httpplugin): invoke 注入 callback_urls.event`

---

### 任务 7：回调 HTTP 端点

**文件：**
- `acl.go`：`POST /v0/runs/{id}/plugin-callbacks` → **`RoleNone`**（仅 token；门禁开时也不要 Operator Bearer）
- `server.go`：注册路由；`handlePluginCallback`  
  1. 读 `token` query + path `run_id`  
  2. Verify；失败 401  
  3. GetRun；无 → 404  
  4. Limiter；拒绝 429  
  5. Decode body：`type`/`name`/`payload`；payload 序列化后 > 64KiB → 400 或 413  
  6. `AppendEvent` Type=`plugin.callback`，Data 含 name+payload  
  7. `204`  
- `bootstrap`：把 secret、publicBase、Signer 配到 Server 与 httpplugin 注册路径  
- `server_plugin_callback_test.go`

开发 publicBase：`cfg.Runtime.PublicBaseURL` 非空优先；否则可用现有 `localHTTPBase(cfg.Listen)` **仅当**明确处于本地开发（规格：未配置生产不注入——实现约定：`PublicBaseURL` 空则 **不注入**；本地 demo 在 yaml example 写 `http://127.0.0.1:8080` 或文档说明。**计划裁定：空 = 不注入**；测试里显式设 PublicBaseURL。）

- [ ] TDD 合法/401/404/429/超限 payload → 实现 → Commit `feat(api): plugin-callbacks 接收侧车事件`

---

### 任务 8：配置示例、架构文档、README

- [ ] `configs/*.yaml`、`default.local.yaml.example`：`runtime:` 下注释 `public_base_url` / `callback_hmac_secret` / `callback_token_ttl_sec`  
- [ ] `docs/architecture-and-plugin-protocol.md` §4.2：标明注入与回调已实现；示例 JSON 保留  
- [ ] README 中英：整删 Connector；侧车 callback 与 `public_base_url`  
- [ ] `go test ./internal/store ./internal/api ./internal/controlplane ./internal/plugincallback ./internal/connector/httpplugin -count=1`  
- [ ] Commit `docs: Connector 删除与 callback_urls 说明`

---

### 任务 9：收尾

- [ ] 按 `finishing-a-development-branch`：全量相关测绿、合并选项  
- [ ] 双仓推送仅在用户要求时执行  

---

## 自检

| 规格章节 | 任务 |
|----------|------|
| §2 Phase 1 API/Store/UI | 1–4 |
| §3.1–3.2 注入与 public_base | 5–6（空则不注入） |
| §3.3–3.4 鉴权与契约 | 5、7 |
| §3.5 / §4 文档与顺序 | 4、8、9 |
| 不做 SDK/软删/改 Run 状态 | 无对应任务（正确） |

**无占位符 TODO。** ACL：`plugin-callbacks` = RoleNone；`DELETE connectors/{id}` = Admin。

---

*计划就绪后选执行方式：子代理驱动 / 本会话内联。*
