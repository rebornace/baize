**中文** | [English](./http-api.en.md)

# 控制面 HTTP 鸟瞰

Baize Runtime 暴露 REST 风格控制面，路径前缀 **`/v0`**（另有 `GET /healthz`、**`GET /metrics`**（Prometheus 文本；需 Admin Bearer）、可选静态 `/ui/`）。

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
| Connectors | `PUT/GET/DELETE /v0/connectors/{id}`、`GET /v0/connectors`、`GET /v0/connectors/{id}/catalog-revisions` | OpenAPI / HTTP 插件 / MCP 等；`PUT` 可含 `catalog_diff` / `catalog_revision`；非空 Diff 且配置了 `events.webhook` 时另异步 POST `type=catalog.diff`；修订历史可查 |
| Connector tools | `POST /v0/connectors/{id}/tools`、`DELETE .../tools/{name}` | 手加 / 删 `extra` REST 工具 |
| Hosted login (connector) | `POST /v0/connectors/{id}/hosted-login/candidates`、`.../suggest`、`.../draft`、`.../test-session` | 规则候选 / AI 建议 / 草稿 / 测试托管 session（admin；对应设置「对外登录」） |
| MCP OAuth | `POST .../mcp/oauth/start` 等 | MCP 连接器 OAuth |
| Tools | `GET /v0/tools`、`PATCH /v0/tools/{name}` | 工具目录（含停用行） |
| Skills | `GET/POST /v0/skills`、`GET/DELETE /v0/skills/{id}` | 技能包 |
| Runs | `POST /v0/runs`、`GET /v0/runs/{id}`、`.../events`、`.../stream`（SSE）、`.../resume`、`.../cancel`、`.../binding`、`.../plugin-callbacks` | 执行与轨迹；`GET .../binding` 为 R1 依赖闭包 |
| Inbox | `POST /v0/inbox/{channel_id}` | Webhook Inbox 入站 |
| Channels inbound | `POST /v0/channels/{name}/inbound` | 声明式渠道入站（当前示例：微信适配器；渠道类型可扩展） |
| Conversations | 列表 / 删会话、消息、身份、fork、rollback 等；`POST /v0/conversations/{id}/hosted-login` | 会话与身份；列表可带 `?workspace_id=`；签发对话作用域托管登录 URL |
| Workspaces | `GET/POST /v0/workspaces` | Web 工作区；同组共享登录身份（设置 → 账号） |
| Artifacts / media | `GET /v0/artifacts/{id}`、`GET /v0/channels/media/...` | 产物与渠道媒体 |
| Settings | `/v0/settings/*` | webhook、inbox、store、channels、runtime、credentials、models、memory、**mcp-export**（对外提供能力）、tool-retrieval… |
| MCP export admin | `/v0/settings/mcp-export`、`.../identities`、`.../keys`、`.../invocations` | 导出开关、身份、Key、调用审计（契约指纹） |
| MCP export | `HANDLE /v0/mcp/export`（及尾斜杠） | 作为 MCP 服务端，把工具目录只读子集导出给 Cursor 等 Agent 客户端 |
| Hosted auth（公网） | `GET/POST /v0/auth/hosted/{id}`、`POST .../method`、`GET .../oauth/callback` | 托管登录页 / 提交 / 切换登录方式 / OAuth 回调（不经 `/ui`） |
| 元信息 | `GET /v0/me`、`GET /v0/ui-config` | 当前身份与 UI 配置 |
| 观测 | `GET /metrics` | Prometheus 文本；需 Admin Bearer；默认开启 |

`Run` 状态机（最小）：`queued` → `running` →（可 `waiting_human` ↔ `running`）→ `succeeded` | `failed` | `cancelled`。

## 匹配与决策（工具预筛 + System One）

设置页「匹配与决策」同时管理增强匹配与决策模型；下列接口均需 **admin**（读接口在 Gate 下亦需已登录角色）。

### 工具匹配（增强 / 标准）

| 路径 | 作用 |
|------|------|
| `GET /v0/settings/tool-retrieval` | 当前模式 / 阶段、Ollama 是否已装/在跑、模型是否在库、路径与下载进度 |
| `POST /v0/settings/tool-retrieval/enable` | 空体或 `{"provider":"local"}`：异步本机 Ollama 一键安装并拉取嵌入模型；`{"provider":"api","base_url","model","api_key?"}`：云端 / 自建 Embedding |
| `POST /v0/settings/tool-retrieval/disable` | 改回标准匹配（不删本机文件） |
| `POST /v0/settings/tool-retrieval/cleanup` | 关增强匹配、删匹配模型与安装包缓存；体可选 `{"remove_ollama":true}` 同时卸载 Ollama 本体 |
| `PUT /v0/settings/tool-retrieval/paths` | 体 `{"models_dir":"..."}` 自定义模型目录（空串清除覆盖）；会尝试带 `OLLAMA_MODELS` 重启本机 Ollama |

未接线管理器时 `GET` 仍返回标准匹配快照；写接口可能 `503 tool_retrieval_unavailable`。国内 Windows 安装优先 ModelScope 同步源。

### 决策模型（System One）

| 路径 | 作用 |
|------|------|
| `GET /v0/settings/systemone` | 当前阶段、本机 / API 模式、模型（默认 `tev1`）、路径与下载进度 |
| `POST /v0/settings/systemone/enable` | 空体或 `{"provider":"local"}`：异步本机 Ollama（≥0.35）安装并拉 `tev1`；`{"provider":"api","base_url","model?","api_key?"}`：探测后写入 `decide_systemone_*` |
| `POST /v0/settings/systemone/disable` | 清除决策服务 knobs（不删本机文件） |
| `POST /v0/settings/systemone/cleanup` | 清理决策相关下载缓存等；体可选 `{"remove_ollama":true}` |

未接线时写接口可能 `503 systemone_unavailable`。**智能提速总开关仍在** `PATCH /v0/settings/runtime`（`decide_enabled` 及各功能开关）。

## 模型发现与批量导入

模型配置支持从 OpenAI 兼容端点拉取目录后批量创建，两个接口均需 **admin**：

| 路径 | 作用 |
|------|------|
| `POST /v0/settings/models/discover` | 仅查询不持久化：按 `base_url`（+ 可选 `api_key` / `api_key_env` / `profile_id`）GET 上游 `/models`，返回模型列表 |
| `POST /v0/settings/models/batch` | 按选中的模型批量创建 profile，共用同一 `base_url` 与凭证；返回 `created` 与 `skipped`（如重名）明细 |

编辑已有 profile 时可不传 `api_key` 而传 `profile_id`，由服务端复用已存密钥，明文密钥不会下发到浏览器。

## 工作区

| 路径 | 作用 |
|------|------|
| `GET /v0/workspaces` | 列出当前操作员可见的工作区（含默认 `default`） |
| `POST /v0/workspaces` | 新建工作区；随后创建的 Web 对话可带 `workspace_id` |
| `GET /v0/conversations?workspace_id=` | 只列出该工作区的会话 |
| `POST /v0/runs` 体字段 `workspace_id` | 新对话落入该工作区（不移动已有会话） |

Web 捕获的登录身份按工作区键共享（设置 → 账号）。渠道与 MCP 导出身份（设置 → 对外提供能力）仍用各自隔离键。托管登录页写入的目标由 session 决定：对话作用域进工作区账号池；MCP 作用域进对应导出身份 / `identity_slots`。

鉴权失败时 MCP 导出优先返回 URL elicitation；客户端不支持时，工具错误中立即包含 `hosted_login_url`（依赖可达的 `runtime.public_base_url`）。

集成时优先读代码注册表与现有集成测试，而不是假定未列出的路径存在。
