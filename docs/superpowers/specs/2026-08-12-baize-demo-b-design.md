# Baize Demo B 设计规格：Chat UI + HITL

> 状态：已批准  
> 日期：2026-08-12  
> 前置：Demo A（OpenAPI + mock LLM + ReAct + REST）已完成  
> 依据：架构草案、grilling 共享理解、本轮头脑风暴（运营 Chat 入口为开箱必需）

---

## 1. 目标与成功标准

**目标：** 运营通过浏览器中的 Chat 窗口，用自然语言驱动遗留/模拟业务系统；写类操作在执行前可人工批准或驳回；HITL 状态经 SQLite 持久化，重启后可恢复。

**一句话成功标准：** `baize demo` 后打开 `/ui`，对话触发需审批的建单 → Chat 内批准 → mock-ticket 出现工单；驳回则无新单且 Run 失败；进程重启后未完成的 `waiting_human` 仍可 resume。

**产品叙事修正：** 开源「开箱」= Runtime + **运营 Chat 入口**，不是仅 curl/SDK。工作流**画布**仍不在开源本里程碑（属商业/后期）。

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| HITL | Run 状态 `waiting_human`；`POST /v0/runs/{id}/resume`；事件 `hitl.waiting` / `hitl.resumed` / `hitl.rejected` |
| 门闸 | Tool 级 `require_approval`（demo 默认对 `create_ticket` 开启）；批准后再发真实 HTTP |
| 存储 | SQLite 默认持久化 Run/Event/HITL 载荷；可选 `memory` 驱动供测试 |
| Chat | 同仓 `web/chat`（Vite + TypeScript）；构建产物由 Go `embed`，路由 `/ui` |
| Demo | `baize demo` 打印 `/ui` 地址；沿用 mock-ticket + mock LLM |
| 稳固 | Registry 并发锁；`demo.Run` 同步 ticket BaseURL；Connector 更新时清理该连接器旧 tools |

### 不做

- 工作流画布 / 低代码编辑器  
- 完整 YAML DSL 的通用 `wait_human` 步骤（本里程碑用 Tool 门闸足够演示第二页故事；DSL 留后续）  
- 登录/SSO/多租户  
- SSE/Webhook（Chat 用轮询）  
- 侧车插件协议、企微 Channel、TS/Python SDK  
- 桌面 App  

---

## 3. 架构

```
浏览器  GET /ui  (embed Vite dist)
   │
   │  REST（轮询）
   ▼
Baize Runtime ── store (sqlite|memory)
   │                Run / Event / HITL
   ├─ ReAct + Tool 审批门闸
   └─ OpenAPI Connector → mock-ticket / 遗留 HTTP
```

**双仓：** 规格与计划仅 `baize_real`；同步开源仓时排除 `docs/superpowers/**`，**包含** `web/chat` 与 embed 所需 `dist` 策略（见 §6）。

---

## 4. Run / HITL 语义

### 状态机

```
queued → running ⇄ waiting_human → running → succeeded | failed
                      │
                      └─ resume(approve) → 继续
                      └─ resume(reject)  → failed
```

### API

| 方法 | 路径 | 说明 |
|------|------|------|
| 已有 | `POST /v0/runs` 等 | 保持 Demo A |
| 新增 | `POST /v0/runs/{id}/resume` | body: `{ "decision": "approve"\|"reject", "comment": "" }` |

**错误：** Run 非 `waiting_human` 时 resume → `409` + `error.code=not_waiting`；未知 run → `404`。

### 事件

| type | data（要点） |
|------|----------------|
| `hitl.waiting` | `prompt`, `tool_name`, `arguments` 摘要 |
| `hitl.resumed` | `decision=approve`, `comment` |
| `hitl.rejected` | `decision=reject`, `comment` |

### Tool 审批门闸

- Connector/Tool 注册时增加 `require_approval bool`（配置或 OpenAPI 扩展位；demo 对 `create_ticket` 写死/配置为 true）。  
- Engine 在 Invoke 前若需审批：写 `hitl.waiting`，`UpdateRun(waiting_human)`，**阻塞该 Run** 直至 resume。  
- approve：执行原 Tool HTTP，写 `tool.result`，继续 ReAct。  
- reject：不调用 HTTP，写 `hitl.rejected`，`UpdateRun(failed)`。

异步模型：Demo A 的 `POST /v0/runs` 为同步 Execute。Demo B 改为：

- **`POST /v0/runs` 立即返回** `{ run_id, status }`（多为 `running` 或很快 `waiting_human`）；  
- Execute 在 **goroutine** 中运行，以便 HTTP 可被 resume 打断等待。  

实现可用：channel/条件变量，按 `run_id` 注册 waiter；resume 时 signal。单测覆盖「挂起 → resume → 完成」。

---

## 5. 存储

```yaml
store:
  driver: sqlite   # 或 memory
  sqlite_path: ./data/baize.db
```

- Schema：runs、events、hitl_pending（或等价列在 runs 上：`status`, `hitl_payload_json`）。  
- SQLite 驱动可用 `modernc.org/sqlite`（纯 Go）以免 CGO。  
- `memory` 驱动保持与 Demo A 行为接近，供 `go test` 默认。

---

## 6. Chat 前端（`web/chat`）

### 技术

- Vite + TypeScript  
- 无重型 UI 框架依赖（可加极薄样式）  
- 仅调用公开 REST  

### 页面行为

1. 输入框发送 → `POST /v0/runs`  
2. 轮询 `GET /v0/runs/{id}` 与 `.../events`（间隔 ~500ms–1s，直到终态）  
3. 渲染用户消息、助手文本、`tool.*` / `hitl.*` 卡片  
4. `waiting_human` → 审批卡（批准 / 驳回 + 可选备注）→ `resume`  
5. 「新对话」清空本地会话并允许新 Run  

### 构建与嵌入

- 源码：`web/chat/`  
- 构建输出：`internal/ui/dist/`（或 `web/chat/dist` 由 go:embed 指向）  
- Go：`GET /ui/`、`GET /ui/*` 提供静态资源；SPA fallback `index.html`  
- 开发：Vite dev server + proxy `/v0` → Runtime  
- README：说明需 Node 20+ 执行 `npm ci && npm run build`（或 CI 生成 dist）；开源发布物可包含预构建 dist，使「仅 Go」可编译运行  

### 扩展约定

UI 只依赖 REST 契约；日后可替换前端实现而不改 Runtime。不拆独立 Git 仓。

---

## 7. 配置与 Demo

在 `configs/demo.yaml` 增加：

```yaml
store:
  driver: sqlite
  sqlite_path: ./data/baize.db
ui:
  enabled: true
# connector tools:
# create_ticket.require_approval: true  （实现可用显式列表）
```

`baize demo` 日志增加：`open http://127.0.0.1:8080/ui`

---

## 8. 测试与验收

1. **单元：** HITL waiter（approve/reject）；SQLite store round-trip；门闸在 approve 前不发 HTTP。  
2. **API 集成：** 创建 Run → waiting_human → resume approve → succeeded 且 ticket 存在；reject → failed 且无新单。  
3. **持久化：** waiting 时停进程（或重开 store）→ resume 仍成功。  
4. **前端：** `npm run build` 成功；可选：检查 embed 后 `/ui` 返回 200（httptest）。  
5. **不强制：** Playwright 全 E2E（可列为后续）。

---

## 9. 与 Demo A / 架构文档关系

- 兑现架构「验收故事 2」的运营路径（Chat 替代纯 curl）。  
- 修正「开源无界面」：开源提供 **Chat 壳**，不提供画布。  
- 完整工作流 DSL、SSE、插件协议等仍按原路线图后置。

---

## 10. 决议摘要

| 项 | 结论 |
|----|------|
| 运营入口 | 开源必须有 Chat |
| 画布 | 本里程碑不做 |
| 打包 | Chat + HITL 同一里程碑 |
| UI 工程 | 同仓 Vite `web/chat`，产物 embed |
| HITL 触发 | Tool `require_approval` 门闸 |
| reject | Run → failed |
| 存储 | SQLite 默认 + memory 可选 |
| Execute | 改为异步，以支持等待中 resume |
