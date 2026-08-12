# Baize Demo B 实现计划（Chat UI + HITL）

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 运营经 `/ui` Chat 用自然语言驱动系统；`create_ticket` 等工具可 HITL 审批；SQLite 持久化；approve 后写回业务，reject 则 Run failed。

**架构：** 异步 Run + Tool 审批门闸 + resume API；store 接口化为 memory/sqlite；Vite `web/chat` 构建产物 embed 到 Go；demo 打印 `/ui`。

**技术栈：** Go 1.22+、`modernc.org/sqlite`、现有 kin-openapi、Vite + TypeScript（web/chat）。

**规格：** `docs/superpowers/specs/2026-08-12-baize-demo-b-design.md`  
**工作目录：** 在 `feat/demo-b` 分支 / worktree 中实现（勿直接在未确认的 main 上开工，除非用户同意）。

---

## 文件结构（将创建/修改）

| 路径 | 职责 |
|------|------|
| `internal/store/store.go` | 抽出 `Store` 接口；`StatusWaitingHuman`；HITL 载荷字段 |
| `internal/store/memory.go` | 现有内存实现迁入 |
| `internal/store/sqlite.go` | SQLite 实现 |
| `internal/store/*_test.go` | memory + sqlite 测试 |
| `internal/run/hitl.go` | 按 run_id 的 approve/reject waiter |
| `internal/run/engine.go` | 异步友好；Invoke 前门闸；等待 resume |
| `internal/tool/registry.go` | `RequireApproval`；`RWMutex`；`Unregister`/`ReplaceConnectorTools` |
| `internal/api/server.go` | 异步 Start Run；`POST .../resume`；挂载 `/ui` |
| `internal/ui/embed.go` | `//go:embed dist` + FileServer |
| `internal/ui/dist/**` | 构建产物（可提交预构建占位 index） |
| `internal/config/config.go` | store/ui/require_approval 配置 |
| `configs/demo.yaml` | sqlite + approval 列表 |
| `internal/demo/demo.go` | BaseURL 同步；打印 `/ui` |
| `web/chat/**` | Vite+TS Chat |
| `tests/integration/hitl_test.go` | HITL API 闭环 |
| `README.md` | Node build + `/ui` 说明 |

---

### 任务 1：Store 接口 + `waiting_human` + memory 迁移

**文件：**
- 修改：`internal/store/store.go`
- 创建：`internal/store/memory.go`（从现实现迁出）
- 修改：`internal/store/store_test.go`
- 修改：所有引用 `store.New` 的包（改为 `store.NewMemory()` 或工厂）

- [ ] **步骤 1：写失败测试** — `StatusWaitingHuman` 常量；`UpdateRun` 可设该状态；`SetHITL`/`GetHITL` 或 Run 上 `HITLPayload` 往返。

- [ ] **步骤 2：定义接口**

```go
type Store interface {
	UpsertAgent(Agent)
	GetAgent(id string) (Agent, error)
	UpsertConnector(Connector)
	GetConnector(id string) (Connector, error)
	CreateRun(agentID, input string) (*Run, error)
	GetRun(id string) (*Run, error)
	UpdateRun(id string, status Status, output, errMsg string) error
	AppendEvent(runID string, ev Event) error
	ListEvents(runID string) ([]Event, error)
	SetHITL(runID string, payload *HITLPayload) error
	GetHITL(runID string) (*HITLPayload, error)
}

type HITLPayload struct {
	Prompt    string         `json:"prompt"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

const StatusWaitingHuman Status = "waiting_human"
```

- [ ] **步骤 3：memory 实现通过测试；全仓 `go test ./...` 绿**

- [ ] **步骤 4：Commit** `refactor(store): Store 接口与 waiting_human`

---

### 任务 2：SQLite Store

**文件：**
- 创建：`internal/store/sqlite.go`、`sqlite_test.go`
- 依赖：`go get modernc.org/sqlite`

- [ ] **步骤 1：测试** — 临时文件 DB：CreateRun → AppendEvent → SetHITL → 关闭再 Open → GetRun status/events/HITL 仍在。

- [ ] **步骤 2：实现** schema：

```sql
CREATE TABLE runs (
  id TEXT PRIMARY KEY, agent_id TEXT, input TEXT, status TEXT,
  output TEXT, error TEXT, created_at TEXT, hitl_json TEXT
);
CREATE TABLE events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id TEXT, type TEXT, timestamp TEXT, data_json TEXT
);
```

- [ ] **步骤 3：`store.Open(driver, path)` 工厂** — `memory` | `sqlite`

- [ ] **步骤 4：Commit** `feat(store): SQLite 持久化 Run/Event/HITL`

---

### 任务 3：HITL waiter + Engine 门闸

**文件：**
- 创建：`internal/run/hitl.go`、`hitl_test.go`
- 修改：`internal/run/engine.go`、`engine_test.go`
- 修改：`internal/tool/registry.go` — `ToolSpec` 扩展或 registry entry 增加 `RequireApproval bool`；`RegisterSpec` 增加参数或新方法 `RegisterSpecApproved(spec, inv, requireApproval bool)`

- [ ] **步骤 1：waiter 测试**

```go
// Start wait in goroutine; after 50ms ResumeApprove; Wait returns approve
// ResumeReject → Wait returns reject; UpdateRun failed path in engine test
```

```go
type Gate struct { /* map[runID]chan Decision */ }
func (g *Gate) Wait(ctx context.Context, runID string) (Decision, error)
func (g *Gate) Resume(runID string, d Decision) error // not waiting → error
```

- [ ] **步骤 2：Engine**  
  - 字段：`Gate *Gate`；`Approvals map[string]bool` 或从 Registry 读  
  - 在 `Tools.Invoke` 前：若需审批 → AppendEvent `hitl.waiting` → UpdateRun waiting_human → SetHITL → `Gate.Wait`  
  - approve → Invoke → 继续；reject → hitl.rejected → failed → return  

- [ ] **步骤 3：单测** — fake tool 计数：reject 时 Invoke 次数为 0；approve 时为 1。

- [ ] **步骤 4：Commit** `feat(run): Tool 审批门闸与 HITL waiter`

---

### 任务 4：API 异步 Run + resume + Registry 锁/替换

**文件：**
- 修改：`internal/api/server.go`、`server_test.go`
- 修改：`internal/tool/registry.go`

- [ ] **步骤 1：Registry** — 加 `sync.RWMutex`；`ReplaceTools(names []string, register func())` 或 `Unregister(name)` + connector 级清理；PUT connector 先卸旧再注册。

- [ ] **步骤 2：`handlePostRun`** — CreateRun 后 `go s.Runner.Execute(...)`；立即返回当前 status（GetRun）。

- [ ] **步骤 3：`POST /v0/runs/{id}/resume`** — 解析 decision；校验 status==waiting_human；`engine.Gate.Resume`；409/404 错误码按规格。

- [ ] **步骤 4：测试** — httptest：post run → poll until waiting_human → resume approve → succeeded；reject 路径；非 waiting resume → 409。

- [ ] **步骤 5：Commit** `feat(api): 异步 Run 与 resume；Registry 并发与更新清理`

---

### 任务 5：配置 / demo / BaseURL 同步

**文件：**
- 修改：`internal/config/config.go`、`configs/demo.yaml`、`internal/demo/demo.go`

- [ ] **步骤 1：Config 增加**

```go
Store struct {
  Driver     string `yaml:"driver"`
  SQLitePath string `yaml:"sqlite_path"`
}
UI struct {
  Enabled bool `yaml:"enabled"`
}
Connector.RequireApproval []string `yaml:"require_approval"` // ["create_ticket"]
```

- [ ] **步骤 2：demo** — `store.Open`；注册 tool 时设 approval；`ticketURL` 写入 connector BaseURL（与 StartForTest 一致）；日志 `open http://.../ui`。

- [ ] **步骤 3：`go test ./...` 绿；Commit** `feat(demo): sqlite 配置、审批列表与 /ui 提示`

---

### 任务 6：web/chat Vite 应用

**文件：**
- 创建：`web/chat/package.json`、`vite.config.ts`、`tsconfig.json`、`index.html`、`src/main.ts`、`src/api.ts`、`src/style.css`

- [ ] **步骤 1：脚手架** — Vite vanillats；`server.proxy['/v0']` → `http://127.0.0.1:8080`；`build.outDir` → `../../internal/ui/dist`

- [ ] **步骤 2：UI** — 消息列表、输入框、发送；轮询 run+events；waiting_human 审批卡；新对话按钮。文案中文。

- [ ] **步骤 3：`npm ci && npm run build` 成功**

- [ ] **步骤 4：Commit** `feat(web): Chat 前端（Vite）`

---

### 任务 7：Go embed `/ui` + 占位 dist

**文件：**
- 创建：`internal/ui/embed.go`
- 确保：`internal/ui/dist/index.html` 存在（构建生成；若无 Node 则提交最小占位并在 README 说明）

```go
//go:embed dist/*
var DistFS embed.FS

func Handler() http.Handler { /* strip prefix, SPA fallback */ }
```

- [ ] **步骤 1：api.Server 挂载** `mux.Handle("/ui/", ...)`（注意尾斜杠）

- [ ] **步骤 2：测试** — httptest `GET /ui/` 或 `/ui/index.html` → 200

- [ ] **步骤 3：Commit** `feat(ui): embed Chat 静态资源到 /ui`

---

### 任务 8：集成测试 + README

**文件：**
- 创建：`tests/integration/hitl_test.go`
- 修改：`tests/integration/demo_a_test.go`（异步：轮询至终态）
- 修改：`README.md`

- [ ] **步骤 1：HITL 集成** — StartForTest + require_approval → waiting → approve → ticket≥1；reject → 无新单。

- [ ] **步骤 2：持久化测** — sqlite temp：waiting 时只关 engine/store，新 store+gate 恢复（若 gate 内存态无法跨进程，规格允许「同进程重开 store + 从 DB 读 HITL 再 Wait」——实现选：**进程内**关闭 DB 连接再 Open 验证；跨进程完整恢复列为 follow-up 若 gate 仅内存）。  
  **裁定（写入实现）：** Demo B 验收「重启 Runtime」= 新进程加载 SQLite 后，对 `waiting_human` 的 Run 调用 resume 仍可用——因此 Gate 在 Resume 前可不依赖旧 Wait 阻塞，Engine 应支持：**若 status 已是 waiting_human 且收到 resume，则从 HITL 载荷继续执行挂起的 tool**（状态机驱动，而非仅靠 channel）。实现时采用 **「resume 驱动续跑」**：Wait 可用 channel 优化同进程；跨重启以 `Resume` API 读取 HITLPayload 并 `ContinueFromHITL(runID, decision)`。任务 3/4 需按此模型实现。

- [ ] **步骤 3：README** — `/ui`、Node build、GOPROXY、HITL 说明；更新架构「第二页」链接。

- [ ] **步骤 4：`go test ./... -count=1` 全绿；Commit** `test: HITL 集成与 README Demo B`

---

## 自检对照规格

| 规格项 | 任务 |
|--------|------|
| waiting_human + resume API | 3、4 |
| Tool require_approval | 3、5 |
| hitl.* 事件 | 3 |
| reject → failed | 3 |
| SQLite + memory | 1、2 |
| 异步 POST /runs | 4 |
| 跨重启 resume | 3+4+8（resume 驱动续跑） |
| web/chat Vite embed /ui | 6、7 |
| demo 打印 /ui、BaseURL | 5 |
| Registry 锁与更新清理 | 4 |
| 画布/登录/SSE/插件 | 未列入 |

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-12-baize-demo-b.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 审查  
2. **内联执行** — 本会话按 executing-plans 推进  

**选哪种？** 开始前请确认是否在 **worktree `feat/demo-b`** 上开发（推荐是）。
