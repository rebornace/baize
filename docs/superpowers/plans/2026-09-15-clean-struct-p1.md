# CLEAN-STRUCT-P1 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 完成 AUDIT §5 **P1** 结构瘦身（及 P0 残留巨石）：拆 `internal/store/sqlite.go`、`internal/run/engine.go`、`internal/api/server_runs.go`、前端 Settings P1 页与 `useChatRun`/`api/settings.ts`；行为以测试为准。

**架构：** 同包/同目录文件搬迁；不改 HTTP/配置契约；不换框架。完成后更新 AUDIT/账本；**本计划结束后才允许执行 CLEAN-GATES**（拧紧 CI）。

**技术栈：** Go 1.25、React/TS。分支：`feat/clean-struct-p1`。Windows：`git commit -m "..."`。禁止 `move_agent_to_root`。

规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md) §4.3  
清单：[`../notes/2026-09-15-clean-audit.md`](../notes/2026-09-15-clean-audit.md) §5  
前置：STRUCT P0 已交付（[`2026-09-15-clean-struct.md`](./2026-09-15-clean-struct.md)）  
后续：[`2026-09-15-clean-gates.md`](./2026-09-15-clean-gates.md)

**本波勾选：**

| 热点 | 处置 |
|------|------|
| `internal/store`（尤其 `sqlite.go` ~1240） | **拆** |
| `internal/run`（尤其 `engine.go` ~1218） | **拆** |
| `internal/api/server_runs.go` ~1010 | **拆**（P0 残留） |
| `web/.../ToolsSettings` / `McpExportSettings` / `ModelSettings` | **拆** |
| `useChatRun.ts` ~500、`api/settings.ts` ~614 | **拆** |
| `internal/connector` / `llm` / `identity` / `conversation` / `openapi` | **本波尽力**：至少各把最大文件压到 <800，或写明「保留理由」 |
| AUDIT **P2**（webhook/bootstrap/Inbox…） | **明确延后** → 可选 STRUCT-P2 或「本版保留」进 GATES 前账本确认 |

**硬约束：** 无测不搬家；不夹带行为变更；gofmt；每任务相关 `go test` / 前端 lint+test+tsc 绿。

---

## 文件结构（目标摘要）

### store

| 文件 | 职责 |
|------|------|
| `internal/store/sqlite.go` | 打开 DB、公共 helper、迁移入口（瘦身后） |
| `internal/store/sqlite_runs.go` | Run/Event/HITL SQL |
| `internal/store/sqlite_conversations.go` | Conversation/Message SQL |
| `internal/store/sqlite_connectors.go` | Connector/Tool/Skill 相关 SQL（若块仍在 sqlite.go） |
| 既有 `*_sql.go` | 保持；勿无故合并 |

### run

| 文件 | 职责 |
|------|------|
| `internal/run/engine.go` | `Engine` 类型、`Run` 入口、循环骨架 |
| `internal/run/engine_step.go` | 单步 LLM/tool 调度 |
| `internal/run/engine_stream.go` | 流式/事件发射 |
| `internal/run/engine_tools.go` | tool 调用与结果写回（按实际剪切块命名） |

### api

| 文件 | 职责 |
|------|------|
| `internal/api/server_runs.go` | HTTP handlers 薄层 |
| `internal/api/server_runs_exec.go` | `runExecute`/`Dispatch`/`ExecuteJob`/lease 等执行族 |

### web

| 路径 | 职责 |
|------|------|
| `pages/tools/*` 或 `pages/ToolsSettings*.tsx` | ToolsSettings 拆列表/表单/hook |
| `pages/mcp-export/*` | McpExport 身份/密钥/列表 |
| `pages/models/*` | ModelSettings 列表+编辑 |
| `pages/chat/useChatRun.ts` + `useChatRunStream.ts` 等 | 流式与恢复拆分 |
| `api/settings.ts` → `api/settings/*.ts` + barrel | 按资源再切 |

---

### 任务 1：拆 `sqlite.go`

**文件：** `internal/store/sqlite*.go`；测试：`go test ./internal/store/ -count=1`

- [ ] **步骤 1：基线**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
(Get-Content internal\store\sqlite.go).Count
```

目标：拆完 **< 700** 行。

- [ ] **步骤 2：按实体剪切**（同 `package store`）

把 Run/Conversation/Connector 等大段方法迁到 `sqlite_runs.go` / `sqlite_conversations.go` / …（按实际函数聚类命名）。保持接收者与签名不变。

```powershell
gofmt -w internal/store
go test ./internal/store/ -count=1
```

- [ ] **步骤 3：Commit**

```powershell
git checkout -b feat/clean-struct-p1   # 若已在则跳过
git add internal/store
git commit -m "refactor(store): 按实体拆分 sqlite 实现文件"
```

---

### 任务 2：拆 `engine.go` + `server_runs.go`

**文件：** `internal/run/engine*.go`、`internal/api/server_runs*.go`  
测试：`go test ./internal/run/ ./internal/api/ -count=1`

- [ ] **步骤 1：拆 engine.go**

目标：`engine.go` **< 700**。迁出 step/stream/tools 相关方法到新文件（`package run`）。

```powershell
gofmt -w internal/run
go test ./internal/run/ -count=1
```

Commit：`refactor(run): 拆分 engine 步进与流式模块`

- [ ] **步骤 2：拆 server_runs.go**

目标：`server_runs.go` **< 700**。执行族（`runExecute`/`Dispatch`/…）→ `server_runs_exec.go`；HTTP handler 留薄文件。

```powershell
go test ./internal/api/ ./internal/run/ -count=1
```

Commit：`refactor(api): 拆分 runs 执行辅助与 HTTP handlers`

---

### 任务 3：Settings P1 三页 + useChatRun + api/settings

**文件：** 见 Web 表；测试：全量 `web/chat` lint/test/tsc

- [ ] **步骤 1：ToolsSettings**

拆 `useToolsSettings` + 列表/批量子组件；页面编排 < 400 行。跑相关 test 文件（若有 `ToolsSettings*.test`）+ `tsc`。

Commit：`refactor(ui): 拆分 ToolsSettings`

- [ ] **步骤 2：McpExportSettings**

同理；Commit：`refactor(ui): 拆分 McpExportSettings`

- [ ] **步骤 3：ModelSettings**

同理；Commit：`refactor(ui): 拆分 ModelSettings`

- [ ] **步骤 4：useChatRun**

目标：`useChatRun.ts` **< 350**。流式订阅与 live-restore 可拆 `useChatRunStream.ts`。

- [ ] **步骤 5：api/settings.ts**

拆成 `api/settings/{runtime,credentials,store,inbox,webhook,mcpExport,models,memory,weixin}.ts`（可合并过小文件），`settings.ts` 或 `api/settings/index.ts` 做 barrel；保持 `from '../api'` 可用。

```powershell
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
```

Commit：`refactor(ui): 拆分 useChatRun 与 api/settings`

---

### 任务 4：其余 Go P1 包（最低达标）

**范围：** `internal/connector`（父包）、`internal/llm`、`internal/identity`、`internal/conversation`、`internal/connector/openapi`

- [ ] **步骤 1：逐包检查最大文件**

若某包最大生产 `.go`（非 `_test`）**≥ 800** 行：必须拆到 <800。  
若已 <800：在报告写「本波免拆」并更新 AUDIT 该行「P1 · 体量已可接受 / 保留」。

优先候选（当前快照）：`connector/register_one.go` ~556、`apply.go` ~435 — 可能已达标；以实测为准。`llm`/`identity`/`conversation`/`openapi` 扫最大文件。

- [ ] **步骤 2：需要则拆 + 测**

```powershell
go test ./internal/connector/... ./internal/llm/ ./internal/identity/ ./internal/conversation/ -count=1
```

- [ ] **步骤 3：Commit**（若有改动）

```powershell
git commit -m "refactor: 收口其余 P1 Go 包文件体量"
```

---

### 任务 5：AUDIT/账本/目录地图收口

- [ ] **步骤 1：更新 AUDIT §5** — P1 行标 **已拆** 或 **保留（理由）**；P2 仍延后或「本版保留待 GATES」  
- [ ] **步骤 2：账本** — STRUCT-P1 **已交付**；下一动作 **CLEAN-GATES**  
- [ ] **步骤 3：更新 `docs/developers/getting-started.md` 目录地图**（补充 store/run 文件格局一句）  
- [ ] **步骤 4：回归**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
go test ./internal/store/ ./internal/run/ ./internal/api/ -count=1
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
```

- [ ] **步骤 5：Commit** `docs: CLEAN-STRUCT-P1 交付与账本收口`  
- [ ] **步骤 6：停住** — 提示执行 GATES 计划

---

## 自检

| 规格/清单 | 任务 |
|-----------|------|
| store/run P1 | 1–2 |
| server_runs 残留 | 2 |
| Web Settings P1 | 3 |
| 其余 Go P1 最低体量 | 4 |
| 目录地图 + 账本 | 5 |
| 不做 GATES/P2 大拆 | 头部 |

无占位步骤。
