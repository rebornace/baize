# CLEAN-STRUCT 实现计划（P0 波）

**交付状态（2026-09-15）：** P0 波已交付（`feat/clean-struct`）；账本见 [`2026-09-13-spec-ledger.md`](../notes/2026-09-13-spec-ledger.md) §5b；AUDIT §5 P0 已标「已拆」，P1+ 延后 **CLEAN-STRUCT-P1**。

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 按 AUDIT §5 完成 **P0** 结构瘦身：拆开 `internal/api/server.go` 残留巨石、按资源拆分 `web/chat/src/api.ts`、按 UI 职责拆分 `ChatPage.tsx`；行为以现有测试为准，不改对外 HTTP 契约。

**架构：** 只做**文件/模块搬迁**（同包 `package api` 或同目录 barrel 导出），不换框架、不改路由语义。Go 侧继续 `package api`；前端 `api.ts` 变为薄 re-export，调用方 import 路径可保持 `from '../api'`。ChatPage 抽 hook + 子组件，页面文件只编排。

**技术栈：** Go 1.25、React/TS（`web/chat`）、Vitest。分支：`feat/clean-struct`（自 `main`）。Windows：`git commit -m "..."`。**禁止** `move_agent_to_root`。本波**不做** GATES（仍保留 CI `only-new-issues`）、不做 PERF、不拆 `internal/store`/`run` 等 P1 包。

规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md) §4.3  
清单：[`../notes/2026-09-15-clean-audit.md`](../notes/2026-09-15-clean-audit.md) §5  

**本波范围（勾选完成）：**

| 热点 | 处置 |
|------|------|
| `internal/api` / 尤其 `server.go`（~3114 行） | **P0 已拆** |
| `web/chat/src/api.ts`（~1400 行） | **P0 已拆** |
| `web/chat/src/pages/ChatPage.tsx`（~1312 行） | **P0 已拆** |
| locales 文案包 | **保留**（AUDIT 已定） |
| weixin-adapter | **保留** |
| AUDIT P1/P2（store/run/Settings 页等） | **本波明确延后** → 另开 `CLEAN-STRUCT-P1` 计划 |

**硬约束：**

- 禁止无测大搬家；每任务搬完即跑相关测试  
- 禁止夹带行为/文案/API 语义变更（发现 bug 另开 fix）  
- 禁止顺便删 `@deprecated` helper（AUDIT 标保留的仍保留）  
- `gofmt` 所有改动的 `.go`；前端 `lint` + `tsc` + `test`  

---

## 文件结构（目标）

### Go（仍 `package api`）

| 文件 | 职责 |
|------|------|
| `internal/api/server.go` | `Server` 结构体、构造、`routes`/`Handler`、鉴权/ACL 小函数、`handleHealthz`/`handleUIConfig`/`handleMe` |
| `internal/api/server_agents_skills.go` | Agent + Skills handlers（从 `server.go` 迁出） |
| `internal/api/server_connectors_tools.go` | Connector CRUD、tools patch/post/delete、`registerOne`、`connectorResponse`、login skill sync |
| `internal/api/server_events_webhook.go` | events-webhook 设置与 deliveries/retry（若仍在 server.go） |
| `internal/api/server_runs.go` | `handlePostRun`、cancel/resume、get run/events/stream、plugin callback、artifact、`runExecute`/`Dispatch`/`ExecuteJob` 等 run 执行族 |
| `internal/api/server_conversations.go` | conversations list/delete、messages、fork/rollback、identities |
| 已有 `server_*.go` | **不合并回** `server.go`；本波只往外抽 |

> 若迁出后某文件仍 >1200 行，允许再按子域二次切开，但**同一任务内**完成并测绿。

### Web API

| 文件 | 职责 |
|------|------|
| `web/chat/src/api/http.ts` | `authHeaders`、`parseJSON`、gate token、`setGateEnabled`、基础 `fetch` 包装 |
| `web/chat/src/api/types.ts` | 共享类型（`Run`、`Event`、`ChatMessage`…）— 从 `api.ts` 原类型迁出 |
| `web/chat/src/api/runs.ts` | createRun/getRun/listEvents/resume/cancel/openRunStream/isTerminal |
| `web/chat/src/api/conversations.ts` | conversations / messages / fork / rollback / identities |
| `web/chat/src/api/connectors.ts` | connectors / tools / mcp oauth |
| `web/chat/src/api/skills.ts` | skills / agent |
| `web/chat/src/api/settings.ts` | runtime/credentials/store/inbox/events-webhook/mcp-export/models/memory/weixin |
| `web/chat/src/api/attachments.ts` | `fileToAttachment` / `inferMediaType` / `isImageAttachment` |
| `web/chat/src/api.ts` | **仅** `export * from './api/...'`（保持既有 import 路径） |

### Chat UI

| 文件 | 职责 |
|------|------|
| `web/chat/src/pages/chat/useChatSession.ts` | 会话列表、当前 conv、scope、加载 messages、删除/fork/rollback |
| `web/chat/src/pages/chat/useChatRun.ts` | createRun、stream、cancel、live run 恢复 |
| `web/chat/src/pages/chat/ChatSidebar.tsx` | 左侧对话列表 + 新对话 |
| `web/chat/src/pages/chat/ChatMessageList.tsx` | 消息区渲染（blocks、ToolCard、Markdown…） |
| `web/chat/src/pages/chat/ChatTopBar.tsx` | 顶栏：模型/思考/语言 chip、菜单 |
| `web/chat/src/pages/ChatPage.tsx` | 组装上述 hook/组件 + Composer |

### 文档

| 文件 | 职责 |
|------|------|
| `docs/developers/architecture.md` 或 `getting-started.md` | 增补「目录地图」一小节（真实路径） |
| AUDIT §5 / 账本 | 标注 P0 已拆；P1+ 延后 |

---

### 任务 1：拆 `server.go` → 域文件（Go）

**文件：** 见上方 Go 表；测试：`go test ./internal/api/ -count=1`

- [ ] **步骤 1：记录基线行数**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
(Get-Content internal\api\server.go).Count
```

写入任务报告（目标：拆完后 `server.go` **< 900** 行，理想 < 700）。

- [ ] **步骤 2：先迁 Agent + Skills**

新建 `server_agents_skills.go`（`package api`），剪切 `handlePutAgent`/`handleGetAgent`/`handleListSkills`/`handleGetSkill`/`handlePostSkill`/`handleDeleteSkill` 及仅被其使用的私有 helper。  
`server.go` 只留函数声明处为空（已搬走）。

```powershell
gofmt -w internal/api/server.go internal/api/server_agents_skills.go
go test ./internal/api/ -count=1
```

预期：PASS。

- [ ] **步骤 3：Commit**

```powershell
git checkout -b feat/clean-struct   # 若已在分支则跳过
git add internal/api
git commit -m "refactor(api): 迁出 agents/skills handlers"
```

- [ ] **步骤 4：迁 Connectors + Tools**

新建 `server_connectors_tools.go`：`handlePutConnector` 至 `handleDeleteConnector` 一段（含 `syncLoginManagedSkill`、`connectorResponse`、`registerOne`、tools handlers）。  
跑 `go test ./internal/api/ -count=1` → commit：

```powershell
git commit -m "refactor(api): 迁出 connectors/tools handlers"
```

- [ ] **步骤 5：迁 events-webhook（若仍在 server.go）**

新建 `server_events_webhook.go`（或并入已有 settings 文件若更贴切——**优先独立文件**）。测绿后 commit。

- [ ] **步骤 6：迁 Runs 执行族**

新建 `server_runs.go`：从 `handlePostRun` 到 stream/artifact/plugin-callback 以及 `runExecute`/`Dispatch`/`ExecuteJob`/`finalizeRunError`/`leaseHeartbeat`/`resolveJob`/`createAndExecuteRun`/`writeAttachmentError` 等。  
注意：已有 `run_start.go` — **不要重复定义**；把相关函数合并进合理文件或从 `run_start.go` 再导出调用，避免符号重复。

```powershell
go test ./internal/api/ -count=1
go test ./tests/integration/ -count=1 -timeout 120s
```

若 integration 因环境失败，至少 `internal/api` 必须绿；报告注明。

Commit：`refactor(api): 迁出 runs/stream handlers`

- [ ] **步骤 7：迁 Conversations + Identities**

新建 `server_conversations.go`。测绿 → commit：`refactor(api): 迁出 conversations/identities handlers`

- [ ] **步骤 8：验收 server.go 体量**

```powershell
(Get-Content internal\api\server.go).Count
```

若仍 ≥900：继续抽出明显成块的 helper，再测再 commit，直到 <900 或报告写明剩余内容清单与「本波停止」理由。

---

### 任务 2：拆 `api.ts` → `api/*` + barrel

**文件：** 见上方 Web API 表；测试：`web/chat` lint/test/tsc

- [ ] **步骤 1：建 `api/http.ts` + `api/types.ts`**

把共享类型与底层 fetch/gate 迁入；暂时让 `api.ts` 从新文件 import 再 export（过渡），跑：

```powershell
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
```

- [ ] **步骤 2：按资源切开并改 barrel**

逐个创建 `runs.ts`、`conversations.ts`、`connectors.ts`、`skills.ts`、`settings.ts`、`attachments.ts`，从原 `api.ts` 剪切函数。最终 `api.ts` **仅**：

```typescript
export * from './api/http'
export * from './api/types'
export * from './api/runs'
export * from './api/conversations'
export * from './api/connectors'
export * from './api/skills'
export * from './api/settings'
export * from './api/attachments'
```

（若 `http` 与 `types` 有循环依赖：允许 `types.ts` 不依赖 fetch；`http.ts` import 类型。）

- [ ] **步骤 3：全量前端回归**

```powershell
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
```

预期：576+ tests 绿（数量可变，但不得引入失败）。

- [ ] **步骤 4：Commit**

```powershell
git add web/chat/src/api.ts web/chat/src/api
git commit -m "refactor(ui): 按资源拆分 api 客户端并保留 barrel"
```

---

### 任务 3：拆 `ChatPage.tsx`

**文件：** 见上方 Chat UI 表；优先复用已有 `Composer`/`ToolCard` 等，**不要**重写业务。

- [ ] **步骤 1：抽 `useChatSession`**

把会话列表、当前 ID、scope、listMessages、delete/fork/rollback 状态与回调迁入 hook；`ChatPage` 改为调用 hook。  
保留 localStorage key 名：`baize.conversation_id` / `baize.conversation_scope`（字符串勿改）。

跑与 Chat 相关的测试（若有 `ChatPage*.test` / Composer 测）：

```powershell
Push-Location web\chat
npx vitest run src/pages src/components/Composer.tsx src/components/Composer.submit.test.tsx
npx tsc --noEmit
Pop-Location
```

- [ ] **步骤 2：抽 `useChatRun`**

stream / createRun / cancel / live run 恢复逻辑入 hook。测绿。

- [ ] **步骤 3：抽 Sidebar / MessageList / TopBar 组件**

纯展示 + 回调 props；样式 class 名尽量原样搬迁，避免视觉回归。

- [ ] **步骤 4：全量前端回归 + 行数验收**

```powershell
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
(Get-Content web\chat\src\pages\ChatPage.tsx).Count
```

目标：`ChatPage.tsx` **< 450** 行（理想 < 350）。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/pages/ChatPage.tsx web/chat/src/pages/chat
git commit -m "refactor(ui): ChatPage 拆 session/run hooks 与子组件"
```

---

### 任务 4：目录地图 + 账本收口

**文件：**
- 修改：`docs/developers/getting-started.md` 或 `architecture.md`（择一，增「仓库目录地图」）
- 修改：`docs/superpowers/notes/2026-09-15-clean-audit.md` §5（P0 行注明已拆；P1+ 注明延后至 STRUCT-P1）
- 修改：账本 CLEAN 行；规格实现计划回链

- [x] **步骤 1：目录地图（公开开发者文档）**

追加简短小节，至少包含：

```markdown
## 目录地图（贡献者）

| 路径 | 说明 |
|------|------|
| `internal/api/` | HTTP 控制面；`server.go` 路由注册，handlers 按域分文件 |
| `web/chat/src/api/` | 浏览器客户端；`api.ts` 为 barrel |
| `web/chat/src/pages/chat/` | Chat 页 hooks/子组件；入口 `pages/ChatPage.tsx` |
| `internal/run/` | Run 引擎（本波未拆，见后续 STRUCT-P1） |
| `internal/store/` | 持久化（本波未拆） |
```

- [x] **步骤 2：回归**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
go test ./internal/api/ -count=1
Push-Location web\chat; npm test; npx tsc --noEmit; Pop-Location
```

- [x] **步骤 3：账本**

CLEAN：**STRUCT P0 已交付**；下一动作开 **CLEAN-STRUCT-P1**（或直接 **CLEAN-GATES**——若产品决定跳过 P1，须在账本写明「P1 热点本版保留」）。  
**默认建议：** 下一刀仍开 STRUCT-P1（store/run + Settings P1 页），然后再 GATES。

- [ ] **步骤 4：Commit**

```powershell
git add docs
git commit -m "docs: CLEAN-STRUCT P0 交付与目录地图"
```

- [ ] **步骤 5：停住**

报告用户：P0 完成；请选择合入方式；是否继续 STRUCT-P1 或跳到 GATES。

---

## 自检（对照规格 §4.3）

| 需求 | 本计划 |
|------|--------|
| 降贡献成本、行为以测试为准 | 任务 1–3 + 每步测试 |
| 优先热点 api / ChatPage / api.ts | 本波 P0 |
| 按职责拆文件 | 文件结构表 |
| 不做无测大搬家 / 换框架 | 硬约束 |
| 开发者文档目录地图 | 任务 4 |
| 热点表勾选或保留理由 | 任务 4；P1+ 延后写明 |
| 不做 GATES/PERF | 头部非目标 |

无占位「待定」步骤；二次切开仅在单文件仍 >1200 时允许。
