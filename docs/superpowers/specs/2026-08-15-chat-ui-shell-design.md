# Baize 设计规格：Chat UI 壳、轨迹卡片与 Run SSE

> 状态：已批准  
> 日期：2026-08-15  
> 前置：Demo B Chat + HITL、对话记忆、会话身份、`GET /v0/tools`、HTTP 插件 v0、README 旁挂定位已落地  
> 依据：Demo C 后续候选「操作员向样板 / 轨迹可视化」；架构草案 §3 事件推送 SSE；对标 ChatGPT / Claude / OpenClaw / Hermes 的壳，而不是把 Tools 堆在聊天页上

---

## 0. 动机

现有 `/ui` 是单栏原型：工具调用是纯文本，审批是底部大卡片，账号 / Tools 叠在对话上方。Network 里每 700ms 轮询 Run 与 events。这不像用户已经会用的 Agent 产品，也不适合作为开源第一版的宣传截图。

操作员用对话时不必理解 Tool 配置；调用时要能看见轨迹。Tools / MCP / 插件放在设置里，配一次很少再进。传输层应对齐常见产品：一条 SSE，而不是控制台刷屏。

---

## 1. 目标与成功标准

**目标：** `/ui` 成为 ChatGPT / Claude 一类的对话壳：左栏历史、中间可折叠工具卡片（含审批）、底栏输入、左下角进入设置；Run 事件走 SSE；前端改为 React + Vite，仍 embed。

**成功标准：**

1. 打开 `/ui/` 看到左栏对话列表、中间消息、底栏输入；不再在聊天主区堆 Tools / 账号面板  
2. 一次含 `create_ticket` 的演示 Run：消息流里出现状态条卡片（名字 + 状态）；`waiting_human` 时卡片上可批准 / 驳回；点开可见参数 / 结果  
3. `GET /v0/conversations` 返回当前有消息的对话；标题为该对话第一条 `role=user` 消息截断；刷新后左栏仍在；`DELETE .../messages` 后该条从列表消失  
4. `/ui/settings/tools` 只读展示 `GET /v0/tools`；`/ui/settings/identities` 承接现账号能力；`/ui/settings/mcp` 与 `/ui/settings/plugins` 为空状态「即将接入」，无假表单  
5. `GET /v0/runs/{id}/stream` 为 `text/event-stream`；一次 Run 浏览器只需一条 stream（另加创建 Run 的 POST）。SSE 不可用时回退轮询  
6. `web/chat` 为 React + TypeScript + Vite；`npm run build` 仍输出到 `internal/ui/dist`；`baize start` 的 `/ui/` 可打开  

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 壳 | 左栏仅对话列表 + 新对话；主区消息；底栏 composer（「+」可占位，本里程碑不实现连接器菜单）；左下角「设置」 |
| 轨迹 | 工具卡片样式 B：默认名字 + 状态；展开参数 / 结果；HITL 按钮在待确认卡片上，去掉主区底部大卡片 |
| 对话列表 | `conversation.Store` 增加列表；`GET /v0/conversations` |
| 设置 | React 路由；Tools 只读；身份 API 原样搬迁；MCP / 插件空状态 |
| SSE | 进程内 fan-out；`GET /v0/runs/{id}/stream`；先回放再增量；终态关闭；失败回退轮询 |
| 前端 | 现有 vanilla `main.ts` 改为 React 组件树；不引入 assistant-ui |
| 测试 | 列表 API、SSE（含重连下标）、卡片状态映射的纯函数单测（若抽离）；既有 HITL / 消息集成测试仍通过 |

### 不做

- MCP 真连接、插件市场、Connector / OpenAPI 编辑器  
- LLM token 打字机（`Chat()` 仍整段返回；SSE 只推已有 event 类型）  
- WebSocket、Webhook  
- 对话 LLM 自动标题、多租户、暗色主题切换（保持现有浅色）  
- 改 `configs/default.yaml`、Docker、Go 引擎 ReAct 逻辑  
- 把 HTTP 插件或 HITL 从产品里删掉  

---

## 3. 信息架构

与 ChatGPT / Claude / OpenClaw Control UI / Hermes 一致：**对话里「用」工具，设置里「看 / 配」工具。**

```
/ui/                      聊天
/ui/settings/tools        只读 Tools
/ui/settings/identities   账号（脱敏、默认、退出、清空）
/ui/settings/mcp          空状态
/ui/settings/plugins      空状态
```

`internal/ui` 已有 SPA fallback：未知路径回 `index.html`。用 `react-router-dom`，`basename` 为 `/ui`。

左栏不放 Workspace、不放 Tools 常驻列表。设置用独立 URL，便于以后加 MCP 页而不改聊天页。

Composer 的「+」本里程碑可渲染为禁用或点击后提示「连接器将在设置中配置」；不实现 Claude / OpenClaw 的按会话开关 MCP。

---

## 4. 对话列表

### 4.1 `GET /v0/conversations`

响应：

```json
{
  "conversations": [
    {
      "id": "conv_...",
      "title": "VPN 挂了，请建一条记录",
      "updated_at": "2026-08-15T10:00:00Z"
    }
  ]
}
```

- 仅包含 **messages 表里至少有一条消息** 的 `conversation_id`  
- `updated_at`：该对话消息的最大 `created_at`  
- `title`：按 `created_at` 升序第一条 `role=user` 的 `content`，超 40 字截断加省略号；若没有 user 消息，标题为「新对话」  
- 排序：`updated_at` 降序  
- 不做分页（本里程碑）  

`DELETE /v0/conversations/{id}/messages` 仍只删消息、不删身份；列表不再出现该 id（直到再产生消息）。

### 4.2 Store

`conversation.Store` 增加：

```go
type Summary struct {
    ID        string
    Title     string
    UpdatedAt time.Time
}

ListSummaries() []Summary
```

SQLite：对 `messages` 按 `conversation_id` 聚合，不新建表。Memory：遍历现有 map。`Clear` 语义不变。

---

## 5. 工具卡片（消息流）

事件仍来自现有 types，UI 映射：

| Event | 卡片 |
|-------|------|
| `llm.tool_call` | 出现卡片，状态「进行中」，可展开 arguments |
| `tool.result` | 状态「已完成」或「错误」（`is_error`），可展开 content |
| `hitl.waiting` | 状态「待确认」，展开 arguments，**批准 / 驳回** 在卡片内；调用现有 `POST /v0/runs/{id}/resume` |
| `hitl.resumed` / `hitl.rejected` | 状态改为已批准 / 已驳回，按钮消失 |
| `llm.message` | 助手气泡，不是卡片 |
| `llm.error` | 系统提示，不是工具卡片 |

同一 `tool` 名 + 顺序：一次 tool_call 与随后的 result / HITL 合成 **一张** 卡片，不要拆成「调用一行 + 结果一行」。

默认收起参数；待确认时卡片用强调边框，按钮可见。

---

## 6. Run SSE

### 6.1 `GET /v0/runs/{id}/stream`

- 响应头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`  
- 不存在的 run：`404`  
- 每条已有 `store.Event` 编码为 SSE `data:`（JSON 与 `GET /v0/runs/{id}/events` 单条相同），`id:` 为该 run 事件列表的 **从 0 开始的下标**  
- 先同步回放当前已有事件，再推增量  
- 客户端可带 `Last-Event-ID`（或查询参数 `after=<index>`）：从下一则开始推，避免重连重复  
- Run 进入 `succeeded` / `failed` 后：再发一条 `event: run.ended`，`data` 含 `{ "status": "..." }`，然后关闭连接  
- `waiting_human` **不**关连接：批准后引擎继续，同一条 stream 继续推（若浏览器在等待时不断开）。若客户端在等待期间断开，resume 成功后重新 `EventSource`  
- 空闲时每 15s 发送 SSE 注释行（`: ping`），避免代理切断长连接  

### 6.2 Fan-out

在 `AppendEvent` 成功路径上做 **进程内** 订阅（按 `run_id`）。SSE handler 订阅该 run；禁止让每个浏览器连接再去 700ms 扫库作为主路径。测试可用 httptest 对一份内存 Store 验回放与增量。

### 6.3 前端

优先 `EventSource`（同源 `/v0/runs/{id}/stream`）。`onerror` 且无法恢复时回退现有轮询（700ms，`GET` run + events）。创建 Run 仍是 `POST /v0/runs`。

本里程碑 **不** 把 LLM 改成 token 流。

---

## 7. 前端结构

`web/chat`：

- 增加 `react`、`react-dom`、`react-router-dom` 与对应 types；Vite 继续 `base: '/ui/'`、`outDir: ../../internal/ui/dist`  
- 用组件拆分：壳、对话列表、消息列表、`ToolCard`、composer、设置布局  
- 删除作为唯一入口的巨型 `main.ts` DOM 字符串（逻辑迁到组件与 `api.ts`）  
- 身份 / tools / messages / resume 的 HTTP 契约不变  

外观：浅色、宽对话栏、左栏灰底，对齐已批准线框；不引入组件库主题包（无 MUI / Ant Design 整包）。

---

## 8. 错误与空态

| 场景 | 行为 |
|------|------|
| 无对话 | 左栏空，主区短欢迎句 + 输入框 |
| Tools 列表空 | 设置页说明尚未注册 Connector |
| SSE 404 | 提示 Run 不存在 |
| resume 失败 | 卡片内显示错误，按钮可再点 |
| 构建未跑 | 与现在相同：需 `npm run build` 才更新 embed |

---

## 9. 测试

- `ListSummaries`：两条对话、标题截断、Clear 后消失；memory + sqlite  
- `GET /v0/conversations` 与 Store 一致  
- SSE：回放已有事件、`AppendEvent` 后增量到达、`Last-Event-ID` 跳过已推、终态 `run.ended`  
- 既有 `tests/integration` 中 HITL / 对话记忆 / 身份 **不因删底部 HITL 大卡片而改契约**（API 不变）  

无强制 Playwright。手工：`baize start` → `/ui` 发一条会审批的消息 → 卡片批准 → 左栏出现标题。

---

## 10. 非目标回顾

之后操作员应能回答：「对话里 Agent 调了什么、我要不要批；Tools 去设置里看。」  
不承诺 MCP 可配、打字机、暗色主题、Workspace 主导航。

---

*本文档经头脑风暴分节批准后落盘。*
