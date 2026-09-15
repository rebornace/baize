# 控制面 HTTP 鸟瞰

Baize Runtime 暴露 REST 风格控制面，路径前缀 **`/v0`**（另有 `GET /healthz`、可选静态 `/ui/`）。

**权威路由表**以 `internal/api/server.go` 中的 `HandleFunc` / `Handle` 注册为准；渠道入站等由 bootstrap / channel 动态 `RegisterRoute`。仓库**没有**独立的 OpenAPI 规范文件描述整份控制面。

## 鉴权

可选控制面口令（YAML `control_plane.operator_token` / `admin_token`，或具名 `operators[]`）：

- Gate 开启后挡住 Runtime 的 `/v0`（开发态 Gate 全空则不挡）。
- **操作员**：跑 Run / HITL / 会话身份等。
- **管理员**：改 Agent、Connector、Tools、渠道与多数设置写接口；目录写（如 `PATCH /v0/tools/{name}`）需 admin，否则 `403`。
- 具名操作员下 `/ui` 会话按 `owner_id` 互不可见（admin 可看全部）。

这不是下游业务 IAM，也不是多租户 SSO。

## 主要资源组

| 组 | 代表路径 | 说明 |
|----|----------|------|
| Agents | `PUT/GET /v0/agents/{id}` | Agent 定义 |
| Connectors | `PUT/GET/DELETE /v0/connectors/{id}`、`GET /v0/connectors` | OpenAPI / HTTP 插件 / MCP 等 |
| Connector tools | `POST /v0/connectors/{id}/tools`、`DELETE .../tools/{name}` | 手加 / 删 `extra` REST 工具 |
| MCP OAuth | `POST .../mcp/oauth/start` 等 | MCP 连接器 OAuth |
| Tools | `GET /v0/tools`、`PATCH /v0/tools/{name}` | 工具目录（含停用行） |
| Skills | `GET/POST /v0/skills`、`GET/DELETE /v0/skills/{id}` | 技能包 |
| Runs | `POST /v0/runs`、`GET /v0/runs/{id}`、`.../events`、`.../stream`（SSE）、`.../resume`、`.../cancel`、`.../plugin-callbacks` | 执行与轨迹 |
| Inbox | `POST /v0/inbox/{channel_id}` | Webhook Inbox 入站 |
| Channels inbound | `POST /v0/channels/{name}/inbound` | 声明式渠道入站（如微信适配器） |
| Conversations | 列表 / 删会话、消息、身份、fork、rollback 等 | 会话与身份 |
| Artifacts / media | `GET /v0/artifacts/{id}`、`GET /v0/channels/media/...` | 产物与渠道媒体 |
| Settings | `/v0/settings/*` | webhook、inbox、store、channels、runtime、credentials、models、memory、mcp-export… |
| MCP export | `HANDLE /v0/mcp/export`（及尾斜杠） | 作为 MCP Server 导出工具目录只读子集 |
| 元信息 | `GET /v0/me`、`GET /v0/ui-config` | 当前身份与 UI 配置 |

`Run` 状态机（最小）：`queued` → `running` →（可 `waiting_human` ↔ `running`）→ `succeeded` | `failed` | `cancelled`。

集成时优先读代码注册表与现有集成测试，而不是假定未列出的路径存在。
