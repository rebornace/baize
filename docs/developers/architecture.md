**中文** | [English](./architecture.en.md)

# 架构与插件边界

Baize（白泽）是独立进程的 **Agent Runtime**（侧车 / 网关）：把 LLM、Tool、遗留 HTTP API 与人机卡点收成可审计的 `Run`。平台用 REST / HTTP 集成；不提供官方语言 SDK（集成方直接调控制面 HTTP）。

| 做 | 不做 |
|----|------|
| OpenAPI → Tool、HTTP 插件 / MCP 连接器、执行回调、HITL、可选线性 workflow | 桌面 App、官方 TS/Python SDK、重画布 / 分支工作流、绑死单一云或 Agent 框架 |
| SQLite 默认可跑；可选 Postgres / Redis / S3 | 把 OTel 或特定 APM 当作运行时必选能力 |

**五抽象：** `Runtime` · `Agent` · `Tool` · `Connector` · `Run`  
Skill / Memory / Channel / 线性 workflow 是配置或插件形态，不升格为第六抽象。

## 逻辑架构

```
企业平台 / 集成方
  REST 控制面 · SSE / Webhook 事件 · Chat UI
          │
          ▼
Baize Runtime (Go)
  Agent · LLM（OpenAI-compatible / 库内 model profile）
  Tool 目录 · Run 引擎（默认 ReAct · 可选线性 workflow）· HITL
  Connector 客户端
     │           │            │
     ▼           ▼            ▼
 OpenAPI 通配  HTTP 插件 v0   MCP 桥
 遗留 REST     侧车 Connector  MCP Server
```

- **MCP 桥（客户端）**：`PUT /v0/connectors/{id}` 注册 `type: mcp`；`tools/list` → 目录 `source=mcp`，Run 内 `tools/call`。
- **MCP 导出**：Runtime 作为 MCP 服务端（`/v0/mcp/export`），把工具目录的只读子集提供给 Cursor、Claude Desktop 等支持 MCP 的 Agent 客户端；与「MCP 桥」方向相反，不经白泽自己的对话模型。
- **存储**：默认 SQLite；支持 `postgres` / `memory`。Blob：`file` / `s3` / `memory`。
- **控制面鉴权**：可选 operator / admin 口令；详见 [http-api](./http-api.md) 与 [configuration](./configuration.md)。

## 插件协议 v0（HTTP + JSON）

侧车与 Runtime **同网 HTTP**；内建 OpenAPI Connector 不走本协议。MCP 映射为内部 Tool，不替代本协议。

### 约定

- Base URL 在注册 Connector 时声明。
- 请求头：`Authorization`（透传或注入）、`X-Baize-Run-Id`、`X-Baize-Tenant-Id`（可空）、`X-Baize-Protocol: v0`。
- 错误体：`{ "error": { "code", "message", "retryable" } }`。

### 侧车必须实现

```http
GET  /healthz                     → 200 { "status": "ok" }
GET  /v0/tools                    → { "tools": [ ToolDesc, ... ] }
POST /v0/tools/{tool_name}/invoke → ToolResult
```

Invoke 请求含 `arguments` 与 `context`（`run_id`、`agent_id`、`tenant_id`，以及可选的 `callback_urls`）。配置了 `runtime.public_base_url`（及 HMAC）且带 `run_id` 时，Runtime 注入短期签名的 `callback_urls.event`；侧车可 POST 写入 Run 事件（`plugin.callback`）。未配置 `public_base_url` 则不注入。

### 企业执行回调

Connector 的 `execution_callback_url`：Runtime POST 工具名、参数、`run_id`、`idempotency_key` 与可选 `callback_urls`；企业无需实现 `/v0/tools` 发现。

### 内建 OpenAPI Connector

1. 导入 OpenAPI 3.x → 每 operation → 一个 Tool（名优先 `operationId`）。
2. 凭证优先级（摘要）：强制 `identity_id` → 会话内未过期 Identity → **仅无** `conversation_id` 时用 Connector 默认头 → 空头。有会话且工具 `require_login` 又无凭证时不发下游 HTTP。完整凭证不出现在 events / GET run。
3. 覆盖不了的遗留逻辑 → 侧车或执行回调。

协议主版本在路径与头中的 `v0`；未知主版本 → `400 protocol_unsupported`。

## 工具目录与 Registry

- 目录行在 store（`source`：`spec` / `plugin` / `mcp` / `extra`）。`spec`/`plugin`/`mcp` 可启停不可删；`extra` 可删。
- Registry **只**挂 `enabled=true` 的行；引擎与 HITL 读 Registry，`GET /v0/tools` 读目录（可见停用行）。
- Agent **不**绑定 Connector 子集：默认每次 Run 把全部启用工具交给模型（跨 Connector 工具名全局唯一）。开启 DP-2a 工具收敛且工具数超过阈值时，会先经关键词预筛 + 决策层收敛，只把候选子集下发给主模型（详见「运行参数」页的工具收敛设置）。

## Agent 运行形态

| 模式 | 行为 |
|------|------|
| 默认 | 单 Agent ReAct：选 Tool → 执行 → 写轨迹 → 结束 |
| 可选 | Skill 包内线性 `workflow.yaml`（顺序步骤 + 可选 HITL；无 branch / 循环） |
| Skill | 配置形态：`SKILL.md` + tools；`agent.skills` 默认激活；`activate_skill` 仅扩大本 Run |
| Channel | 带签名的来信入口；即时消息等为进程外适配器 + 声明式 `channels`（见 [部署](./deployment.md)）。**当前仓库已提供个人微信适配器**，渠道能力本身不限于微信 |

## HITL

写操作等可标 `require_approval`：Run 进入 `waiting_human`，运营 / API `POST /v0/runs/{id}/resume` 后继续。Chat UI（`/ui`）提供运营入口。
