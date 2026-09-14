# UI-LOGIN-AT：登录入口直达实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 会话级 `login-entries` / `login-invoke` + Chat `@`/`/` 登录区与 `login_required`「去登录」，确定性调用登录工具并走既有 capture，且不破坏模型帮登。

**架构：** 纯函数包 `internal/loginentry` 从 Store 工具目录 + capture 推导入口；`run.Engine.ExecuteForcedTool` 跳过 LLM、跳过 login 门闸（仅针对已校验的登录入口）、保留 HITL；API 挂会话路由；Chat 弹窗/表单/ToolCard 调同一 API。身份仍按 `conversation_id`。

**技术栈：** Go、既有 `tool.Registry` / `identity` / `authresolve`、React/Vitest；中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-14-ui-login-at-design.md`

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/loginentry/entry.go` | `Entry` DTO、`List`、`Find`、`RequiredFromSchema`、`ValidateArgs`、敏感键 redact |
| `internal/loginentry/entry_test.go` | 目录推导 / 过滤 / 校验单测 |
| `internal/run/forced_tool.go` | `ExecuteForcedTool`：强制单工具回合 |
| `internal/run/forced_tool_test.go` | 无 LLM、capture、HITL、busy 之外的引擎行为 |
| `internal/api/server_login_entry.go` | `handleListLoginEntries` / `handleLoginInvoke` |
| `internal/api/server_login_entry_test.go` | HTTP 契约测 |
| `internal/api/server.go` | 注册路由 |
| `internal/controlplane/acl.go` + `acl_test.go` | Operator 可读/可调 |
| `web/chat/src/api.ts` | `listLoginEntries` / `loginInvoke` |
| `web/chat/src/strings.ts` | 登录弹窗 / 表单 / 去登录文案 |
| `web/chat/src/loginEntry.ts` | 过滤、表单字段推导（纯函数） |
| `web/chat/src/loginEntry.test.ts` | 前端纯函数测 |
| `web/chat/src/components/Composer.tsx` | `@`/`/` 弹窗混入登录区 |
| `web/chat/src/components/LoginParamsModal.tsx` | 必填参数表单 |
| `web/chat/src/components/LoginPicker.tsx` | 可复用选择器（弹窗与「去登录」共用） |
| `web/chat/src/components/ToolCard.tsx` | `login_required` →「去登录」 |
| `web/chat/src/pages/ChatPage.tsx` | 拉 entries、invoke、订阅 run、接线 Composer/ToolCard |
| 对应 `*.test.tsx` | Composer / ToolCard / LoginPicker 冒烟 |
| `docs/superpowers/notes/2026-09-13-spec-ledger.md` 等 | 状态「计划已写」 |
| `internal/ui/dist/**` | `npm run build` 嵌入 |

**API 相对规格的实务补充（写入实现，不另开规格）：** `POST login-invoke` body 增加必填 `agent_id`（与 `POST /v0/runs` 相同，Chat 已有）；无 agent 无法 `CreateRun`。

---

### 任务 1：`loginentry` 目录推导

**文件：**
- 创建：`internal/loginentry/entry.go`、`internal/loginentry/entry_test.go`

- [ ] **步骤 1：编写失败测试**

```go
package loginentry_test

import (
	"testing"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/loginentry"
	"github.com/rebornace/baize/internal/store"
)

func TestListMatchesCaptureEnabledOpenAPIHTTP(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "crm", Type: "openapi",
		Auth: store.ConnectorAuth{Capture: store.CaptureAuth{ToolNameGlob: "*login*"}},
	})
	st.UpsertConnector(store.Connector{ID: "side", Type: "http"})
	st.UpsertConnector(store.Connector{ID: "mcp1", Type: "mcp"})
	st.ReplaceConnectorTools("crm", []store.Tool{
		{ConnectorID: "crm", Name: "user_login", Enabled: true, InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"username": map[string]any{"type": "string"},
				"password": map[string]any{"type": "string"},
			},
			"required": []any{"username", "password"},
		}},
		{ConnectorID: "crm", Name: "list", Enabled: true},
		{ConnectorID: "crm", Name: "old_login", Enabled: false},
	})
	st.ReplaceConnectorTools("side", []store.Tool{
		{ConnectorID: "side", Name: "plugin_login", Enabled: true},
	})
	st.ReplaceConnectorTools("mcp1", []store.Tool{
		{ConnectorID: "mcp1", Name: "mcp_login", Enabled: true},
	})

	ids := identity.NewMemoryStore()
	entries := loginentry.List(st, ids, "conv1", "")
	if len(entries) != 2 {
		t.Fatalf("len=%d want 2 (crm user_login + side plugin_login via defaults)", len(entries))
	}
	if entries[0].ID != "crm/user_login" || len(entries[0].Required) != 2 {
		t.Fatalf("first=%+v", entries[0])
	}
	if entries[0].LoggedIn {
		t.Fatal("no identity => logged_in false")
	}
}

func TestListFilterConnectorAndLoggedIn(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "crm", Type: "openapi"})
	st.ReplaceConnectorTools("crm", []store.Tool{
		{ConnectorID: "crm", Name: "login", Enabled: true},
	})
	ids := identity.NewMemoryStore()
	_, _ = ids.Upsert("conv1", identity.Identity{
		Label: "u", Scheme: "Bearer", Source: identity.SourceLoginCapture,
		CredentialHeaders: map[string]string{"Authorization": "Bearer t"},
		IsDefault: true,
	})
	all := loginentry.List(st, ids, "conv1", "crm")
	if len(all) != 1 || !all[0].LoggedIn {
		t.Fatalf("%+v", all)
	}
	empty := loginentry.List(st, ids, "conv1", "missing")
	if len(empty) != 0 {
		t.Fatalf("missing connector => [] got %d", len(empty))
	}
}
```

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/loginentry/ -count=1
```

预期：包不存在 / 编译失败。

- [ ] **步骤 3：最少实现**

`entry.go` 要点：

```go
package loginentry

type Entry struct {
	ID              string         `json:"id"`
	ConnectorID     string         `json:"connector_id"`
	ConnectorTitle  string         `json:"connector_title"`
	ConnectorType   string         `json:"connector_type"`
	ToolName        string         `json:"tool_name"`
	Title           string         `json:"title"`
	LoggedIn        bool           `json:"logged_in"`
	Parameters      map[string]any `json:"parameters"`
	Required        []string       `json:"required"`
}

// List: type ∈ {openapi,http}；CaptureDefaults；MatchToolName；仅 Enabled；
// LoggedIn = OpenAPISecurityResolver 对会话身份 OK 且 Headers 非空（与 engine.blockedByLogin 反义、无 SecuritySchemes）。
// connectorFilter 非空则只该 id；不存在 → 空切片。
// 排序 connector_id, tool_name。
// Find(st, ids, conv, connectorID, toolName) (*Entry, bool)
// RequiredFromSchema / ValidateArgs / RedactArgs（password|passwd|secret|token|api_key 大小写不敏感 → "***"）
```

依赖：`connector.CaptureDefaults`、`identity.MatchToolName`、`authresolve.OpenAPISecurityResolver`。

- [ ] **步骤 4：测试通过**

```bash
go test ./internal/loginentry/ -count=1
```

- [ ] **步骤 5：Commit**

```bash
git add internal/loginentry/
git commit -m "feat(loginentry): 从 capture 推导会话登录入口目录"
```

---

### 任务 2：`ExecuteForcedTool`

**文件：**
- 创建：`internal/run/forced_tool.go`、`internal/run/forced_tool_test.go`
- 修改：`internal/run/engine.go`（将 `invokeTool` 增加 `skipLoginGate bool`，或抽共享内核供 forced 调用）

- [ ] **步骤 1：编写失败测试**

用 Memory Store + Registry 注册 `login`（匹配 capture、写身份）与假 LLM（**不得被调用**）：

```go
func TestExecuteForcedToolInvokesWithoutLLM(t *testing.T) {
	// LLM provider that t.Fatal on Chat
	// Register login tool: on invoke Upsert identity via same pattern as connector tests OR simple invoker
	// CreateRun with ConversationID
	// Engine.ExecuteForcedTool(ctx, runID, "login", map[string]any{"u":"a"})
	// Assert: events contain llm.tool_call + tool.result；无 llm.message 来自模型
	// Assert: LLM Chat 计数为 0
	// Assert: run StatusSucceeded
}

func TestExecuteForcedToolSkipsLoginGate(t *testing.T) {
	// Tool RequireLogin=true, conv 无身份
	// 普通 blockedByLogin 会挡；Forced 仍应 Invoke 成功
}

func TestExecuteForcedToolHITL(t *testing.T) {
	// RequireApproval=true；goroutine ExecuteForcedTool；Resume approve；工具被调用
}

func TestExecuteForcedToolRedactsArgsInEvent(t *testing.T) {
	// args password=secret；AppendEvent llm.tool_call 里 arguments.password == "***"
}
```

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/run/ -run ForcedTool -count=1
```

- [ ] **步骤 3：实现**

```go
// ExecuteForcedTool runs one tool for an existing run: run.started already appended by API.
// skipLoginGate=true when appending tool call; still honors require_approval via awaitHITL.
// On completion: StatusSucceeded (即使 tool is_error)；HITL reject 走既有 rejected。
// Output 可空（不写助手长文）；依赖 events 给 Chat。
func (e *Engine) ExecuteForcedTool(ctx context.Context, runID, toolName string, args map[string]any) error
```

复用 `injectAuthCtxFromRun`、cancel 注册、`persistToolResult`。`llm.tool_call` 的 Data 使用 `loginentry.RedactArgs(args)`。

- [ ] **步骤 4：测试通过 + 邻近回归**

```bash
go test ./internal/run/ -count=1
```

- [ ] **步骤 5：Commit**

```bash
git add internal/run/forced_tool.go internal/run/forced_tool_test.go internal/run/engine.go
git commit -m "feat(run): 强制单工具回合 ExecuteForcedTool"
```

---

### 任务 3：API + ACL

**文件：**
- 创建：`internal/api/server_login_entry.go`、`internal/api/server_login_entry_test.go`
- 修改：`internal/api/server.go`、`internal/controlplane/acl.go`、`internal/controlplane/acl_test.go`

- [ ] **步骤 1：ACL 测试条目**

```go
{"GET", "/v0/conversations/c1/login-entries", RoleOperator},
{"POST", "/v0/conversations/c1/login-invoke", RoleOperator},
```

- [ ] **步骤 2：HTTP 测试（失败先行）**

```go
func TestLoginEntriesAndInvoke(t *testing.T) {
	// 启动与 session-login 类似的 test Server：Store、Registry、Identities、Runner=Engine、Messages
	// PUT/注册 openapi 或手写 connector+tool+RegisterMeta login 捕获
	// GET /v0/conversations/c1/login-entries → 含条目
	// GET ?connector_id=other → entries:[]
	// POST login-invoke 缺 agent_id → 400
	// POST 合法 → 200 {run_id}；poll events 有 tool.result；GET identities 有 login_capture
	// 并发：先 CreateRun 占住会话，再 invoke → 409 conversation_busy
	// POST 非目录工具 → 400 not_a_login_entry
}
```

路由：

```go
s.mux.HandleFunc("GET /v0/conversations/{id}/login-entries", s.handleListLoginEntries)
s.mux.HandleFunc("POST /v0/conversations/{id}/login-invoke", s.handleLoginInvoke)
```

`handleLoginInvoke` 流程：

1. Decode `{agent_id, connector_id, tool_name, arguments}`  
2. `Find` 目录；否则 `400 not_a_login_entry`  
3. `ValidateArgs`；否则 `400`  
4. `HasActiveRun` → `409 conversation_busy`  
5. `startRun` 或等价：`BubbleContent`/`Input` = `已发起登录 · {entry.Title}`（**不含** arguments）  
6. `Dispatch` 调 `ExecuteForcedTool`（与 `runExecute` 相同异步模型）  
7. `200` `{run_id, status, conversation_id}`

`prepareConversationMeta` / 会话 owner 校验与 `startRun` 一致。

- [ ] **步骤 3：实现至测试绿**

```bash
go test ./internal/controlplane/ ./internal/api/ -run Login -count=1
go test ./internal/api/ -count=1
```

（若全量 api 过慢，至少 `-run 'Login|Identit'` 加上强制相关。）

- [ ] **步骤 4：Commit**

```bash
git commit -m "feat(api): 会话 login-entries 与 login-invoke"
```

---

### 任务 4：前端 API + 纯函数 + 文案

**文件：**
- 修改：`web/chat/src/api.ts`、`web/chat/src/strings.ts`
- 创建：`web/chat/src/loginEntry.ts`、`web/chat/src/loginEntry.test.ts`

- [ ] **步骤 1：Vitest**

```ts
import { describe, expect, it } from 'vitest'
import { filterLoginEntries, fieldsFromEntry } from './loginEntry'

describe('filterLoginEntries', () => {
  it('filters by query against title and tool_name', () => {
    const entries = [
      { id: 'crm/login', title: '登录 · CRM / login', tool_name: 'login', required: [] as string[] },
      { id: 'crm/other', title: '登录 · CRM / other', tool_name: 'other_login', required: ['x'] },
    ]
    expect(filterLoginEntries(entries as any, 'crm').length).toBe(2)
    expect(filterLoginEntries(entries as any, 'other')).toEqual([entries[1]])
  })
})
```

- [ ] **步骤 2：实现 `listLoginEntries` / `loginInvoke` 与 strings**

```ts
// api.ts
export type LoginEntry = {
  id: string
  connector_id: string
  connector_title: string
  connector_type: string
  tool_name: string
  title: string
  logged_in: boolean
  parameters?: Record<string, unknown>
  required: string[]
}
export async function listLoginEntries(conversationId: string, connectorId?: string): Promise<LoginEntry[]>
export async function loginInvoke(
  conversationId: string,
  body: { agent_id: string; connector_id: string; tool_name: string; arguments?: Record<string, unknown> },
): Promise<{ run_id: string; status: string; conversation_id: string }>
```

文案键（`strings.ts`）：`LOGIN_AT.sectionLogin`、`sectionSkills`、`loggedInBadge`、`goLogin`、`paramsTitle`、`paramsSubmit`、`startedNote` 等。

- [ ] **步骤 3：测试通过**

```bash
cd web/chat && npx vitest run src/loginEntry.test.ts
```

- [ ] **步骤 4：Commit**

```bash
git commit -m "feat(webui): login-entries API 客户端与过滤辅助"
```

---

### 任务 5：Composer 登录区 + 参数表单

**文件：**
- 创建：`web/chat/src/components/LoginParamsModal.tsx`、`LoginPicker.tsx`（若希望弹层独立）
- 修改：`web/chat/src/components/Composer.tsx`、`Composer.completion.test.tsx`（或新 `Composer.login.test.tsx`）

- [ ] **步骤 1：行为测试**

- 提供 `loginEntries` + 输入 `@` → 出现「登录」区与条目；标「已登录」  
- 选中 `required: []` → 调用 `onPickLogin(entry)`，**不** `replaceMention` 技能逻辑  
- 选中有 required → 打开 `LoginParamsModal`；提交带 arguments 调 `onPickLogin(entry, args)`  
- `/` 同样打开  
- 仅技能时行为与现网一致  

- [ ] **步骤 2：实现**

扩展 `ComposerProps`：

```ts
loginEntries?: LoginEntry[]
onPickLogin?: (entry: LoginEntry, args?: Record<string, unknown>) => void | Promise<void>
```

弹窗结构：登录区（上）+ 技能区（下）；query 同时过滤。选中登录后清除 `@`/`/` 查询片段（`setText` 去掉 active mention），**不**插入 `@login:...`。

`LoginParamsModal`：按 `required` 渲染 input；`type=password` 若字段名匹配敏感模式。

- [ ] **步骤 3：vitest 绿**

```bash
cd web/chat && npx vitest run src/components/Composer
```

- [ ] **步骤 4：Commit**

```bash
git commit -m "feat(webui): Composer @/ 弹窗混入登录入口与参数表单"
```

---

### 任务 6：ToolCard「去登录」+ ChatPage 接线

**文件：**
- 修改：`ToolCard.tsx`、`ToolCard.hitl.test.tsx` 或新测试、`ChatPage.tsx`
- 可选：`foldEvents` 无需改（已有 tool.result）

- [ ] **步骤 1：ToolCard 测试**

`content.code === 'login_required'` 且非 readOnly → 显示「去登录」；点击调用 `onGoLogin?.(block)`（由父级打开 LoginPicker 并带 `connector_id`）。

从 `catalog` 取 `connector_id`：`catalog.find(t => t.name === block.name)?.connector_id`。

- [ ] **步骤 2：ChatPage**

- 会话 id 变化时 `listLoginEntries(conversationId)`  
- `onPickLogin` → `loginInvoke` → 用现有 `createRun` 后相同的 stream/poll 路径订阅 `run_id`（抽一小函数 `attachRun(runId)` 以免复制）  
- **禁止**把 args 写入 `setMessages` 用户气泡  
- ToolCard `onGoLogin` → 打开过滤后的 `LoginPicker` Modal  

- [ ] **步骤 3：测试 + build**

```bash
cd web/chat && npx vitest run src/components/ToolCard src/components/Composer src/loginEntry.test.ts
cd web/chat && npm run build
```

将 dist 拷入 / 由既有脚本嵌入 `internal/ui/dist`（与仓库惯例一致）。

- [ ] **步骤 4：Commit**

```bash
git commit -m "feat(webui): login_required 去登录与 Chat 强制登录接线"
```

---

### 任务 7：账本与回归烟测

**文件：**
- `docs/superpowers/notes/2026-09-13-spec-ledger.md`
- `docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md`
- 规格文首状态可改为「实现中」或交付后「已交付」

- [ ] **步骤 1：更新账本** — UI-LOGIN-AT：计划已写 / 实现中  

- [ ] **步骤 2：后端烟测**

```bash
go test ./internal/loginentry/ ./internal/run/ ./internal/api/ ./internal/controlplane/ -count=1
```

- [ ] **步骤 3：Commit**

```bash
git commit -m "docs(UI-LOGIN-AT): 账本标记计划已就绪"
```

---

## 自检（对照规格）

| 规格项 | 任务 |
|--------|------|
| GET login-entries + 推导规则 | 1、3 |
| logged_in 谓词 | 1 |
| POST login-invoke 强制调用 | 2、3 |
| 409 busy / 400 not_a_login_entry | 3 |
| capture / 账号页同会话 | 2、3（identities 断言） |
| 模型帮登不破坏 | 2 不改 ReAct 主路径；3 回归可选 |
| require_approval | 2 HITL 测 |
| 跳过自身 require_login 门闸 | 2 skipLoginGate |
| @ 与 / | 5 |
| 多条 / 已登录标注 | 1、5 |
| 参数表单 | 5 |
| login_required 去登录过滤 | 6 |
| 密码不进用户气泡 + 事件 redact | 3 BubbleContent、2 RedactArgs |
| 不做跨会话 / MCP | 范围外 |
| agent_id | 任务 3 实务补充 |

无「待定」占位；类型名统一 `loginentry.Entry` / 前端 `LoginEntry`。
