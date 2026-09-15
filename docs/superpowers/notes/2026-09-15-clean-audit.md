# CLEAN-AUDIT 盘点清单

> 日期：2026-09-15  
> 状态：**待产品确认**  
> 规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md)  
> 约束：本文件只读盘点结果；确认前不得开 CONTRACT 实现。

## 0. 产品确认栏

- [ ] 契约白名单已审阅
- [ ] 结构热点优先级已审阅
- [ ] 明确不做无异议
- [ ] 批准进入 CLEAN-CONTRACT

确认人 / 日期：

## 1. 契约白名单（HTTP）

| METHOD path | 处置 | 理由 |
|-------------|------|------|
| DELETE /v0/connectors/{id} | 保留 | |
| DELETE /v0/connectors/{id}/tools/{name} | 保留 | |
| DELETE /v0/conversations/{id} | 保留 | |
| DELETE /v0/conversations/{id}/identities | 保留 | |
| DELETE /v0/conversations/{id}/identities/{iid} | 保留 | |
| DELETE /v0/conversations/{id}/messages | 保留 | |
| DELETE /v0/settings/mcp-export/identities/{id} | 保留 | |
| DELETE /v0/settings/mcp-export/keys/{id} | 保留 | |
| DELETE /v0/settings/memory/{id} | 保留 | |
| DELETE /v0/settings/models/{id} | 保留 | |
| DELETE /v0/skills/{id} | 保留 | |
| GET /healthz | 保留 | |
| GET /v0/agents/{id} | 保留 | |
| GET /v0/artifacts/{id} | 保留 | |
| GET /v0/channels/media/{conv}/{object} | 保留 | |
| GET /v0/connectors | 保留 | |
| GET /v0/connectors/{id} | 保留 | |
| GET /v0/connectors/{id}/mcp/oauth/callback | 保留 | |
| GET /v0/connectors/{id}/mcp/oauth/status | 保留 | |
| GET /v0/conversations | 保留 | |
| GET /v0/conversations/{id}/identities | 保留 | |
| GET /v0/conversations/{id}/messages | 保留 | |
| GET /v0/me | 保留 | |
| GET /v0/runs/{id} | 保留 | |
| GET /v0/runs/{id}/events | 保留 | |
| GET /v0/runs/{id}/stream | 保留 | |
| GET /v0/settings/channels/{name} | 保留 | |
| GET /v0/settings/channels/{name}/login/status | 保留 | |
| GET /v0/settings/channels/{name}/outbound-deliveries | 保留 | |
| GET /v0/settings/credentials | 保留 | |
| GET /v0/settings/events-webhook | 保留 | |
| GET /v0/settings/events-webhook/deliveries | 保留 | |
| GET /v0/settings/inbox-channels | 保留 | |
| GET /v0/settings/mcp-export | 保留 | |
| GET /v0/settings/mcp-export/identities | 保留 | |
| GET /v0/settings/mcp-export/identities/{id} | 保留 | |
| GET /v0/settings/mcp-export/keys | 保留 | |
| GET /v0/settings/memory | 保留 | |
| GET /v0/settings/models | 保留 | |
| GET /v0/settings/runtime | 保留 | |
| GET /v0/settings/store | 保留 | |
| GET /v0/skills | 保留 | |
| GET /v0/skills/{id} | 保留 | |
| GET /v0/tools | 保留 | |
| GET /v0/ui-config | 保留 | |
| HANDLE /ui/ | 保留 | |
| HANDLE /v0/mcp/export | 保留 | |
| HANDLE /v0/mcp/export/ | 保留 | |
| PATCH /v0/settings/credentials | 保留 | |
| PATCH /v0/settings/mcp-export/identities/{id} | 保留 | |
| PATCH /v0/settings/memory/{id} | 保留 | |
| PATCH /v0/settings/models/{id} | 保留 | |
| PATCH /v0/settings/runtime | 保留 | |
| PATCH /v0/tools/{name} | 保留 | |
| POST /v0/channels/{name}/inbound | 保留 | |
| POST /v0/connectors/{id}/mcp/oauth/disconnect | 保留 | |
| POST /v0/connectors/{id}/mcp/oauth/start | 保留 | |
| POST /v0/connectors/{id}/tools | 保留 | |
| POST /v0/conversations/{id}/fork | 保留 | |
| POST /v0/conversations/{id}/identities | 保留 | |
| POST /v0/conversations/{id}/identities/{iid}/default | 保留 | |
| POST /v0/conversations/{id}/messages/{message_id}/rollback | 保留 | |
| POST /v0/inbox/{channel_id} | 保留 | |
| POST /v0/runs | 保留 | |
| POST /v0/runs/{id}/cancel | 保留 | |
| POST /v0/runs/{id}/plugin-callbacks | 保留 | |
| POST /v0/runs/{id}/resume | 保留 | |
| POST /v0/settings/channels/{name}/login/start | 保留 | |
| POST /v0/settings/channels/{name}/logout | 保留 | |
| POST /v0/settings/channels/{name}/outbound-deliveries/{id}/retry | 保留 | |
| POST /v0/settings/channels/{name}/process/restart | 保留 | |
| POST /v0/settings/channels/{name}/process/start | 保留 | |
| POST /v0/settings/channels/{name}/process/stop | 保留 | |
| POST /v0/settings/events-webhook/deliveries/{id}/retry | 保留 | |
| POST /v0/settings/events-webhook/test | 保留 | |
| POST /v0/settings/inbox-channels/{id}/rotate-secret | 保留 | |
| POST /v0/settings/inbox-channels/{id}/test | 保留 | |
| POST /v0/settings/mcp-export/identities | 保留 | |
| POST /v0/settings/mcp-export/keys | 保留 | |
| POST /v0/settings/memory | 保留 | |
| POST /v0/settings/models | 保留 | |
| POST /v0/settings/reload | 保留 | |
| POST /v0/settings/store/restart | 保留 | |
| POST /v0/skills | 保留 | |
| PUT /v0/agents/{id} | 保留 | |
| PUT /v0/connectors/{id} | 保留 | |
| PUT /v0/settings/channels/{name} | 保留 | |
| PUT /v0/settings/events-webhook | 保留 | |
| PUT /v0/settings/inbox-channels | 保留 | |
| PUT /v0/settings/store | 保留 | |

处置枚举：**保留** / **重命名** / **删除**。

## 2. 契约白名单（配置 / env / CLI）

| 键或命令 | 处置 | 理由 |
|----------|------|------|
| YAML `listen` | 保留 | 默认 `:8080`；可被 `BAIZE_LISTEN` 覆盖（`internal/config/config.go` `applyListenEnv`） |
| YAML `store.driver` / `store.sqlite_path` / `store.dsn` | 保留 | sqlite / postgres / memory 等驱动与连接 |
| YAML `ui.enabled` | 保留 | 是否挂载静态 UI |
| YAML `llm.*`（`provider` `base_url` `model` `api_key_env` `disable_thinking` `thinking_level` `thinking_dialect` `supports_vision`） | 保留 | 启动期默认 LLM；`api_key_env` 默认 `BAIZE_API_KEY` |
| YAML `skills.builtin_dir` / `skills.builtin_dirs` / `skills.user_dir` | 保留 | 技能扫描根；`builtin_dirs` 优先于 `builtin_dir` |
| YAML `agent.id` / `agent.system` / `agent.skills` | 保留 | 默认 agent 身份与技能列表 |
| YAML `connector.*`（`id` `type` `spec` `base_url` `execution_callback_url` `require_approval*` `require_login` `auth.*`） | 保留 | 样板 OpenAPI/MCP 连接器与鉴权块（`auth.mode` / static / passthrough / vault_ref / capture） |
| YAML `run.max_steps` / `run.tool_timeout_sec` | 保留 | 默认 16 / 60s |
| YAML `conversation.*`（`max_messages` `persist_identities` `compact_enabled` `compact_threshold` `compact_reserve_output` `compact_recent_messages`） | 保留 | 会话上限与滚动摘要压缩 |
| YAML `middleware.*`（`driver` `worker_concurrency` `lease_ttl_sec` `reconcile_interval_sec` `redis.*`） | 保留 | memory（默认）或 redis 队列；`redis.password_env` 等可指向自定义 env |
| YAML `storage.*`（`driver` `file.root_dir` `s3.endpoint` `region` `bucket` `prefix` `access_key_env` `secret_key_env` `use_ssl` `path_style` `auto_create_bucket`） | 保留 | file / s3 / memory；S3 凭证 env 默认 `S3_ACCESS_KEY` / `S3_SECRET_KEY` |
| YAML `mock_ticket.listen` | 保留 | 演示 mock-ticket 侧车监听（默认 `:18080`） |
| YAML `control_plane.operator_token` / `admin_token` / `operators[]` | 保留 | 支持 `env:VAR` / `file:/path` / 明文；operators 为 `id` + `token` |
| YAML `events.webhook.url` / `events.webhook.headers` | 保留 | 出站事件 webhook |
| YAML `runtime.public_base_url` / `callback_hmac_secret` / `callback_token_ttl_sec` | 保留 | 侧车 callback URL 注入与 HMAC token（默认 TTL 3600s） |
| YAML `inbox.channels[]` | 保留 | 入站 webhook 渠道种子（`inbox.Channel`） |
| YAML `mcp_export.enabled` | 保留 | 省略时默认 true（`*bool` 区分未设与 false） |
| YAML `channels[]`（`name` `type` `enabled` `config`） | 保留 | 声明式渠道实例；**省略整段**时 legacy 路径按 type 名 auto-wire **非** `DeclarativeOnly` 的注册渠道（webhook 等须在本节显式 `enabled: true`，见 §4 / `wireChannels`） |
| env `BAIZE_LISTEN` | 保留 | 非空时覆盖 YAML `listen` |
| env `BAIZE_API_KEY` | 保留 | 默认 LLM API key（`llm.api_key_env` 可改名） |
| env `BAIZE_SETTINGS_KEY` | 保留 | 密封设置/收件箱/MCP OAuth 等；`baize demo` 未设时用临时 demo key |
| env `BAIZE_OPERATOR_TOKEN` / `BAIZE_ADMIN_TOKEN` / `BAIZE_OP_*` | 保留 | 经 `control_plane.*: env:…` 引用（非硬编码名，样板见 `configs/demo.yaml`） |
| env `S3_ACCESS_KEY` / `S3_SECRET_KEY` | 保留 | `storage.s3` 默认凭证 env（可被 yaml 改名） |
| env `BAIZE_TEST_PG_DSN` | 保留 | **仅测试/CI** Postgres 集成（非运行时产品契约） |
| env `BAIZE_CONNECTOR_TOKEN` 等 | 保留 | 连接器样板中的 `${VAR}` / `env:VAR` 占位示例（`configs/default.local.yaml`） |
| env 自定义（`llm.api_key_env`、`middleware.redis.password_env`、`storage.s3.access_key_env` 等） | 保留 | 由 YAML 字段名声明，非固定 `BAIZE_*` 前缀 |
| `.env` 加载 | 保留 | `baize` 入口 `config.LoadDotEnv(".env")`；已存在进程 env 不被覆盖 |
| CLI `baize start` | 保留 | `configs/minimal.yaml`（或 `minimal.local.yaml`）；须 `ValidateStart`（含 API key） |
| CLI `baize demo` | 保留 | 分层 `demo.yaml` + `default.local.yaml` + `demo.local.yaml`；`bootstrap.Run` |
| CLI `baize serve -config <path>` | 保留 | 显式单文件配置启动 |
| CLI `baize reset-credentials -config <path>` | 保留 | 清空 store 中 control-plane 热更新 creds，回退 YAML break-glass |
| CLI `weixin-adapter` flags：`-baize` `-secret` `-addr` `-creds` `-ilink-base` `-port-file` | 保留 | 独立进程；`-secret` / `-baize` 必填；HTTP 面见 §1 脚注（非 core mux） |

## 3. 契约白名单（Web 客户端 / 路由）

路由定义：`web/chat/src/main.tsx`（`BrowserRouter` `basename="/ui"`；无 `createBrowserRouter`）。对外静态面：`HANDLE /ui/`（§1）。

### 3.1 UI 路由

| 表面（api.ts 函数或 UI 路由） | 处置 | 理由 |
|------------------------------|------|------|
| UI `/` → `ChatPage` | 保留 | 聊天主壳 |
| UI `/settings` → `SettingsLayout`（子路由见下） | 保留 | 设置 IA |
| UI `/settings` index → `SettingsHome` | 保留 | 设置首页 |
| UI `/settings/tools` → `ToolsSettings` | 保留 | 工具目录 |
| UI `/settings/openapi` → `OpenApiSettings`（`AdminOnly`） | 保留 | OpenAPI 连接器 |
| UI `/settings/skills` → `SkillsSettings` | 保留 | 技能 |
| UI `/settings/memory` → `MemorySettings` | 保留 | 记忆 |
| UI `/settings/identities` → `IdentitiesSettings` | 保留 | 会话身份 |
| UI `/settings/mcp` → `McpSettings`（`AdminOnly`） | 保留 | MCP 连接器 |
| UI `/settings/mcp-export` → `McpExportSettings`（`AdminOnly`） | 保留 | MCP 导出 |
| UI `/settings/plugins` → `PluginSettings`（`AdminOnly`） | 保留 | HTTP 插件连接器 |
| UI `/settings/webhooks` → `WebhookSettings`（`AdminOnly`） | 保留 | 事件 webhook |
| UI `/settings/inbox` → `InboxSettings`（`AdminOnly`） | 保留 | 外部来信 |
| UI `/settings/channels/weixin` → `WeixinChannelSettings` | 保留 | 微信渠道 |
| UI `/settings/models` → `ModelSettings` | 保留 | 模型配置 |
| UI `/settings/storage` → `StorageSettings`（`AdminOnly`） | 保留 | 存储驱动 |
| UI `/settings/runtime` → `RuntimeSettings` | 保留 | 运行时旋钮 + 控制面凭证 |
| UI `*` → `Navigate` `/` | 保留 | 未知路径回聊天 |
| 组件 `GateRoot` / `UnlockPage`（非独立路由） | 保留 | 门禁解锁；`getMe` + `getUIConfig` |

### 3.2 `api.ts` 导出（类型 / 错误 / 本地辅助）

| 表面 | 处置 | 理由 |
|------|------|------|
| `api.ts` 导出类型（`Run`/`Event`/`ModelProfile`/`ConnectorInfo`/`ChatMessage`/…） | 保留 | TS 契约；随 UI 与测试引用 |
| `ApiError` | 保留 | `strings*.ts` 错误映射 |
| `setGateEnabled` | 保留 | 门禁开关；无独立 HTTP |
| `inferMediaType` / `fileToAttachment` / `isImageAttachment` | 保留 | 附件本地处理；`createRun` 入参 |
| `isTerminal` | 保留 | `findLiveRun.ts` |
| `openRunStream` | 保留 | `GET /v0/runs/{id}/stream`（SSE） |

### 3.3 `api.ts` HTTP 客户端函数

| 表面 | 处置 | 理由 |
|------|------|------|
| `getUIConfig` | 保留 | `GET /v0/ui-config` |
| `getMe` | 保留 | `GET /v0/me` |
| `startWeixinLogin` | 保留 | `POST /v0/settings/channels/weixin/login/start` |
| `getWeixinLoginStatus` | 保留 | `GET /v0/settings/channels/weixin/login/status` |
| `logoutWeixin` | 保留 | `POST /v0/settings/channels/weixin/logout` |
| `getWeixinSettings` / `putWeixinSettings` | 保留 | `GET`/`PUT /v0/settings/channels/weixin` |
| `startWeixinProcess` / `stopWeixinProcess` / `restartWeixinProcess` | 保留 | `POST …/channels/weixin/process/{start\|stop\|restart}` |
| `getChannelOutboundDeliveries` / `retryChannelOutboundDelivery` | 保留 | 微信出站投递；`GET`/`POST …/outbound-deliveries` |
| `listMemory` / `createMemory` / `patchMemory` / `deleteMemory` | 保留 | `GET`/`POST`/`PATCH`/`DELETE /v0/settings/memory` |
| `getRuntimeSettings` / `patchRuntimeSettings` | 保留 | `GET`/`PATCH /v0/settings/runtime` |
| `getCredentials` / `patchCredentials` | 保留 | `GET`/`PATCH /v0/settings/credentials`；`RuntimeSettings` |
| `createRun` | 保留 | `POST /v0/runs` |
| `getStoreSettings` / `putStoreSettings` / `restartAfterStoreChange` | 保留 | `GET`/`PUT /v0/settings/store`；`POST …/store/restart` |
| `getEventsWebhook` / `putEventsWebhook` / `testEventsWebhook` | 保留 | events-webhook CRUD + test |
| `getEventsWebhookDeliveries` / `retryEventsWebhookDelivery` | 保留 | 投递列表与重试 |
| `getInboxChannels` / `putInboxChannels` / `rotateInboxSecret` / `testInboxChannel` | 保留 | inbox-channels 全套 |
| `getMCPExportSettings` | 保留 | `GET /v0/settings/mcp-export` |
| `listMCPExportIdentities` / `createMCPExportIdentity` / `patchMCPExportIdentity` / `deleteMCPExportIdentity` | 保留 | mcp-export identities |
| `listMCPExportKeys` / `createMCPExportKey` / `revokeMCPExportKey` | 保留 | mcp-export keys |
| `listModelProfiles` / `createModelProfile` / `updateModelProfile` / `deleteModelProfile` | 保留 | `GET`/`POST`/`PATCH`/`DELETE /v0/settings/models` |
| `getRun` / `listEvents` / `resumeRun` / `cancelRun` | 保留 | runs 读/事件/恢复/取消 |
| `listTools` / `patchTool` | 保留 | `GET /v0/tools`；`PATCH /v0/tools/{name}` |
| `patchToolRequireLogin` | **删除** | **无 UI 调用**（全 `web/chat/src` 无引用；`ToolsSettings` 直调 `patchTool`） |
| `createConnectorTool` / `deleteConnectorTool` | 保留 | connector tools POST/DELETE |
| `listConnectors` / `getConnector` / `putConnector` / `deleteConnector` | 保留 | connectors CRUD |
| `startMcpOAuth` / `disconnectMcpOAuth` | 保留 | MCP OAuth start/disconnect |
| `listSkills` / `uploadSkill` / `deleteSkill` | 保留 | skills（无 `GET /v0/skills/{id}` 封装） |
| `getAgent` / `putAgent` | 保留 | `GET`/`PUT /v0/agents/{id}` |
| `listIdentities` / `createIdentity` / `setDefaultIdentity` / `deleteIdentity` / `clearIdentities` | 保留 | conversation identities |
| `listMessages` | 保留 | `GET …/messages` |
| `clearMessages` | **删除** | **无 UI 调用**（全 `web/chat/src` 无引用）；后端 `DELETE …/messages` 仍存在 |
| `deleteConversation` | 保留 | `DELETE /v0/conversations/{id}` |
| `rollbackMessages` / `forkConversation` | 保留 | rollback / fork |
| `listConversations` | 保留 | `GET /v0/conversations` |

### 3.4 §1 HTTP 无 Web 封装（非删除候选）

下列 §1 路由**无**对应 `api.ts` 函数，属探针、OAuth 重定向、MCP 协议、渠道入站或插件侧车；**保留** HTTP 契约，不在 Web 表标删：

`GET /healthz`；`GET /v0/artifacts/{id}`；`GET /v0/channels/media/{conv}/{object}`；`GET /v0/connectors/{id}/mcp/oauth/callback`；`GET /v0/connectors/{id}/mcp/oauth/status`；`GET /v0/settings/mcp-export/identities/{id}`；`GET /v0/skills/{id}`；`POST /v0/channels/{name}/inbound`；`POST /v0/inbox/{channel_id}`；`POST /v0/runs/{id}/plugin-callbacks`；`POST /v0/settings/reload`；`HANDLE /v0/mcp/export`（及尾斜杠变体）。

媒体 URL 由 `createRun` 附件与消息渲染间接使用 channel media 路径，无独立 fetch 封装。

## 4. 废弃 / 兼容 / 双写信号

| 位置 | 信号摘要 | 建议处置 | 理由 |
|------|----------|----------|------|
| `web/chat/src/pages/runtimeSettingsHelpers.ts` | `@deprecated`：`MAIN_KNOB_FIELDS` / `COMPACT_ADV_FIELDS` / `KNOB_FIELDS` | 保留（CONTRACT 前勿删） | UI 内部别名；产品行为以 `mainKnobFields()` / `compactAdvFields()` 为准 |
| `web/chat/src/historyBlocks.ts` | `@deprecated` 类型别名 `ToolOrWorkflowBlock` | 保留 | 调用点迁移期兼容；对外无独立 API |
| `internal/store/sqlite.go` | `type SQLite = SQLStore` 向后兼容别名 | 内部保留 | **非 HTTP 契约**；STRUCT 时可保留或改名 |
| `internal/store/store.go` + `internal/api/server_models.go` + `web/chat/src/api.ts` | `disable_thinking` 与 `thinking_level` / `thinking_dialect` 双写；`SyncProfileThinking` / `applyThinkingPayload` | 保留 | **对外**：`GET/PATCH/POST /v0/settings/models` JSON 仍含 `disable_thinking`；PATCH 时 `thinking_level` 优先；YAML `llm.disable_thinking` 仍为启动配置字段 |
| `internal/store/sqlite.go` / `postgres.go` | DB 迁移：`disable_thinking=1` → `thinking_level=off` | 保留 | 存量数据兼容；不改变 HTTP 字段名 |
| `internal/config/config.go` + `internal/bootstrap/bootstrap.go` `wireChannels` | 省略 YAML `channels:`（legacy）：遍历注册 descriptor 按 type 名接线一次；**跳过** `DeclarativeOnly`（含 webhook）；当前仓库无其它非 DeclarativeOnly 入站 IM 时 legacy 可不挂任何 webhook | 保留 | **部署契约**：旧单文件仍 auto-wire 非 DeclarativeOnly 渠道；webhook/多实例须 `channels:` 显式声明（与动态 `POST /v0/channels/{name}/inbound` 一致） |
| `internal/channel/registry.go` | `RegisterChannel` 无 metadata 注册；声明式配置 partial 列表 back-compat | 内部保留 | 影响渠道接线语义；非 REST 路径 |
| `web/chat/src/api.ts` | 错误文案保留 `"HTTP <status>:"` 前缀 | 保留 | `GateRoot` 等 UI 用 `startsWith('HTTP 401:')` |
| `web/chat/src/components/Composer.submit.test.tsx` | `onSend` 返回 `undefined` 的 void 契约 | 保留 | 旧组件回调约定 |
| `web/chat/src/pages/ChatPage.tsx` | 非安全上下文剪贴板 legacy fallback | 保留 | HTTP/LAN 环境 UX |
| `internal/api/server.go` 等 | `legacy single-instance` goroutine 执行路径注释 | 内部保留 | 运行时实现细节；**非对外契约**（grep 见 `_tmp-debt-grep` 同类项可忽略） |

## 5. 结构热点

统计日期：2026-09-15。复现命令见 §10 任务 4。

**Go：** 在 `internal`、`cmd` 下递归 `*.go`，排除 `vendor`；按**目录（包）**聚合行数，取 Top 15。

**TypeScript：** 在 `web/chat/src` 下递归 `*.ts` / `*.tsx` 单文件行数；**排除** `*.test.ts(x)`、`*.spec.ts(x)`（不把测试计入 STRUCT 热点）；取生产源码 Top 15。若含测试文件，`ModelSettings.test.tsx`（672）等会挤占榜单，与「页面实现体量」目标不一致。

### 5.1 Go 包 Top 15

| 路径 | 行数（约） | 建议拆法（一句话） | 本版优先级 |
|------|------------|--------------------|------------|
| `internal/api` | 16962 | 按域拆 handler：`server_settings_*` / `server_runs` / `server_conversations` 等，共享 `parse`/`ACL` 小模块 | P0 |
| `internal/store` | 7039 | 驱动与迁移分目录；SQL 方法按实体（models/conversations/runs）切文件 | P1 |
| `internal/run` | 5600 | 引擎步进、流式事件、插件回调与取消分模块 | P1 |
| `internal/channel/webhook` | 5137 | 入站/出站/重试与配置解析分模块 | P2 |
| `internal/connector` | 3788 | MCP/invoke/registry 与 OpenAPI 子包边界收紧（父包不含 openapi 子目录行数） | P1 |
| `internal/bootstrap` | 3723 | `wire*` 按子系统（store/channel/connector）分段 | P2 |
| `internal/channel` | 2451 | registry 与各渠道适配边界 | P2 |
| `internal/llm` | 2311 | provider 实现与 thinking/profile 适配分文件 | P1 |
| `internal/identity` | 2146 | 会话身份解析、默认身份与 store 映射分层 | P1 |
| `cmd/weixin-adapter/internal/weixinlink` | 1798 | 独立进程 iLink 客户端；保持与 core 边界 | 保留 |
| `internal/connector/openapi` | 1674 | spec 解析、路由展开与 invoker 生成拆文件 | P1 |
| `internal/conversation` | 1662 | 持久化、压缩、fork/rollback 服务分层 | P1 |
| `internal/memory` | 1597 | store 后端与 settings API 映射 | P2 |
| `internal/config` | 1500 | 校验与 env 覆盖分文件 | P2 |
| `cmd/weixin-adapter` | 1381 | 独立进程入口与 HTTP 面；不并入 baize core | 保留 |

### 5.2 Web 生产源码 Top 15（排除 `*.test.*` / `*.spec.*`）

| 路径 | 行数（约） | 建议拆法（一句话） | 本版优先级 |
|------|------------|--------------------|------------|
| `web/chat/src/api.ts` | 1409 | 按资源拆 `api/runs.ts`、`api/settings/memory.ts`、`api/connectors.ts` 等，保留 `authHeaders`/`parseJSON` 内核 | P0 |
| `web/chat/src/pages/ChatPage.tsx` | 1312 | 拆会话侧栏、消息区（列表+折叠块）、Composer 区、顶栏与 run 生命周期 hook | P0 |
| `web/chat/src/locales/en.ts` | 868 | **保留（文案包）** | 保留 |
| `web/chat/src/locales/zh.ts` | 859 | **保留（文案包）** | 保留 |
| `web/chat/src/pages/ToolsSettings.tsx` | 808 | 列表/批量操作/连接器内嵌表单拆子组件 + `useToolsSettings` | P1 |
| `web/chat/src/pages/McpExportSettings.tsx` | 744 | 身份列表、密钥列表、工具 export 列拆段 | P1 |
| `web/chat/src/pages/ModelSettings.tsx` | 621 | 列表与编辑 Modal/表单拆文件 | P1 |
| `web/chat/src/pages/InboxSettings.tsx` | 533 | 渠道卡片与密钥旋转 Modal 拆组件 | P2 |
| `web/chat/src/pages/RuntimeSettings.tsx` | 531 | 旋钮表单与 operators 卡片拆分 | P2 |
| `web/chat/src/pages/WeixinChannelSettings.tsx` | 516 | 登录流与进程控制拆 hook | P2 |
| `web/chat/src/components/settings/ConnectorEditorModal.tsx` | 371 | OpenAPI/MCP 两步表单按 `connectorForms/*` 与 shell 拆段 | P2 |
| `web/chat/src/pages/SkillsSettings.tsx` | 355 | 列表、上传与 agent 技能勾选拆 hook + 子列表 | P2 |
| `web/chat/src/components/Composer.tsx` | 314 | 输入区、附件、技能选择与提交拆子组件 | P2 |
| `web/chat/src/foldEvents.ts` | 294 | 纯函数块类型与 fold 规则可按 event kind 分文件 | P2 |
| `web/chat/src/pages/MemorySettings.tsx` | 280 | CRUD 表格与编辑表单拆组件 | P2 |

## 6. 门禁基线

统计日期：2026-09-15。环境：Go 1.25.0、`golangci-lint` v2.4.0（与 CI `golangci-lint-action` `version: v2.4.0` 对齐）、`web/chat` Node lint。复现命令见 §10 任务 5。

| 工具 | 命令 | 问题数或退出码 | 备注 |
|------|------|----------------|------|
| golangci 全量 | `golangci-lint run ./... --timeout=5m` | **46 issues**；退出码 **1** | 官方摘要：`errcheck` 20、`staticcheck` 22、`ineffassign` 2、`unused` 2。Top 规则（按子码/linter 粗排）：`errcheck`、`staticcheck`（常见 `S1016`/`QF1006`/`ST1019`/`SA4000`/`QF1002` 等）、`unused`、`ineffassign`。CI 现为 `only-new-issues: true`，故本地全量是「首次存量债」基线；**AUDIT 不修**，留给 GATES |
| eslint | `npm ci` + `npm run lint`（`web/chat`） | 退出码 **0**；无 error/warning 摘要 | `eslint src` 干净 |
| gofmt -l | `gofmt -l ./cmd ./internal`；另扫全仓 `*.go`（排除 `.git`/`vendor`/`.superpowers`/`node_modules`） | **0** dirty；退出码 **0** | 裸 `gofmt -l .` 会因 `.superpowers/engine_old.go`（非法 UTF-16）报错；产品源码已齐。**不**把 `.superpowers` 噪声计入门禁 |

## 7. 性能候选（只标注，不测）

本阶段**不**跑重型压测、**不**新增 `Benchmark*`、**不**写 README 数字。仓库当前无既有 `Benchmark` 函数。

| 路径 | 为何值得测 | PERF-HOT 建议探针 |
|------|------------|-------------------|
| 聊天流式 `GET /v0/runs/{id}/stream` | 主路径：UI `openRunStream` / SSE 轮询；长 run、多 tool 事件时延迟与缓冲直接影响体感；已有 `server_sse_poll_test` 证明可测 | 本地起 `baize demo` → `POST /v0/runs` → 计时首个 `run.started` / 终端事件到齐；或扩展现有 SSE 测试为可脚本化耗时；可选 `go test` 包内轻量计时（非压测） |
| 会话消息读写 | `GET/DELETE …/messages`、fork/rollback、rolling summary 与 compact 同路径；大会话时 list + compact 易成瓶颈 | 构造 N 条消息会话后测 `handleListMessages` / store `ListMessages` 耗时；`go test` 对 `internal/conversation` / `internal/store` 加可控 N 的计时断言或后续 `Benchmark` |
| blob / artifacts | 附件上传、`GET /v0/artifacts/{id}`、channel media；S3/file 驱动切换后 IO 差异大，易误判「慢在 API」 | 固定小/中附件：`createRun` 带 attachment → 读 artifact URL；对 `blob.Store` Put/Get 做本地 `go test` 计时；对比 memory vs file 驱动 |
| 渠道出站 | 微信等 `outbound-deliveries` 列表/重试、webhook dispatcher；失败重试队列积压时影响渠道 UX | 种子若干 delivery 行 → `GET …/outbound-deliveries` 与 `POST …/retry` 延迟；对 `internal/channel/webhook` 出站路径加可复现 fixture + 计时（勿打真实外部网） |

## 8. 公开文档处置（CONTRACT 执行）

| 路径 | 建议 | 理由 |
|------|------|------|
| `README.md` / `README.zh-CN.md` | 重写为产品向 | 规格已定 |
| 新建开发者文档 | 新建 | 规格已定 |
| `docs/architecture-and-plugin-protocol.md` | 删除 | 规格已定 |
| `docs/deployment.md` | 删除 | 规格已定 |

## 9. 明确不做（本轮 CLEAN）

- 插件公共 Go SDK、OTel、Playwright、新功能史诗
- 强制覆盖率挡合并
- 无证据的性能改动、伪造竞品对比
- AUDIT 阶段不修 golangci 全量 **46** 条存量（留给 GATES；CI 仍 only-new）
- 不把 `cmd/weixin-adapter` 侧 HTTP 并入 baize core mux 契约清理范围（独立进程；§1 脚注已说明）
- 不把 `web/chat/src/locales/*` 文案包纳入 STRUCT 拆分目标（§5.2 已标「保留」）
- 不将 `internal/store` 的 `SQLite` 类型别名等**非 HTTP** 内部兼容当作对外删除项
- 不清理 `.superpowers` 下非产品源码噪声（如非法编码 `engine_old.go`）；不计入 gofmt 门禁
- 本 AUDIT **不**跑重型压测 / **不**编造 Benchmark 或 README 性能数字（§7 只标注）

## 10. 复现命令备忘

### 任务 2：HTTP 路由全表

```powershell
$routes = Select-String -Path internal\api\server.go -Pattern 'HandleFunc\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"' |
  ForEach-Object { if ($_.Line -match 'HandleFunc\("((?:GET|POST|PUT|PATCH|DELETE) [^"]+)"') { $Matches[1] } } |
  Sort-Object -Unique
$extra = @(
  'POST /v0/channels/{name}/inbound',
  'HANDLE /v0/mcp/export',
  'HANDLE /v0/mcp/export/',
  'HANDLE /ui/'
)
$all = @($routes + $extra | Sort-Object -Unique)
$path = Join-Path (Get-Location) 'docs\superpowers\notes\2026-09-15-clean-audit-routes.txt'
[IO.File]::WriteAllLines($path, $all, [Text.UTF8Encoding]::new($false))
```

```powershell
Select-String -Path internal\**\*.go,cmd\**\*.go -Pattern 'RegisterRoute|POST /v0/channels/' |
  Where-Object { $_.Path -notmatch '_test\.go$' } |
  Select-Object -First 30 Path,LineNumber,Line
```

（任务 2 核对：`internal/api/server.go` 为 `HandleFunc`/`mux.Handle` 主表；**动态渠道入站** `POST /v0/channels/{name}/inbound` 由 `internal/channel/webhook/channel.go` 经 `api.Server.RegisterRoute` 按渠道名挂载（契约路径模板见上）。）

（**非本表：** 独立进程 `cmd/weixin-adapter` 另暴露适配器侧 HTTP（如 `/outbound`、`/admin/*`），不属于 baize core mux 白名单。）

### 任务 3：配置 / env / CLI + 废弃信号

```powershell
Select-String -Path internal\**\*.go,cmd\**\*.go,configs\**\* -Pattern 'BAIZE_|os\.Getenv|APIKeyEnv' |
  Where-Object { $_.Path -notmatch '_test\.go$' } |
  Select-Object -First 60 Path,LineNumber,Line
```

（核对：`internal/config/config.go` `type Config` + `cmd/baize/main.go` / `settings_reset.go` / `cmd/weixin-adapter/config.go`。）

```powershell
Select-String -Path internal\**\*.go,web\chat\src\**\*.{ts,tsx} -Pattern 'deprecated|Deprecated|legacy|back-compat|backward compatible|兼容|废弃|@deprecated' |
  Select-Object Path,LineNumber,Line
```

（盘点后删临时文件：`Remove-Item docs\superpowers\notes\_tmp-debt-grep.txt -ErrorAction SilentlyContinue`。）

### 任务 4：Web 表面 + 结构热点

```powershell
Select-String -Path web\chat\src\api.ts -Pattern '^export (async )?function |^export const ' |
  ForEach-Object { $_.Line.Trim() }
```

```powershell
# UI 路由：web\chat\src\main.tsx（BrowserRouter basename=/ui）
```

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"

Get-ChildItem -Recurse -Filter '*.go' internal,cmd |
  Where-Object { $_.FullName -notmatch '\\vendor\\' } |
  Group-Object { $_.DirectoryName } |
  ForEach-Object {
    $lines = 0
    $_.Group | ForEach-Object { $lines += @(Get-Content $_.FullName).Count }
    [PSCustomObject]@{ Lines=$lines; Dir=$_.Name.Replace((Get-Location).Path + '\','') }
  } |
  Sort-Object Lines -Descending |
  Select-Object -First 15

Get-ChildItem -Recurse -Include '*.ts','*.tsx' web\chat\src |
  Where-Object { $_.Name -notmatch '\.(test|spec)\.(ts|tsx)$' } |
  Sort-Object { @(Get-Content $_.FullName).Count } -Descending |
  Select-Object -First 15 |
  ForEach-Object { "{0,5}  {1}" -f @(Get-Content $_.FullName).Count, $_.FullName.Replace((Get-Location).Path+'\','') }
```

（TS 榜单排除 `*.test.*` / `*.spec.*`；Go 榜单为包目录 Top 15，见 §5.1–5.2。删除候选核对：`Select-String -Path web\chat\src -Pattern 'patchToolRequireLogin|clearMessages' -Recurse` 应仅命中 `api.ts` 定义行。）

### 任务 5：门禁基线 + 性能候选 + 收口

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"

# gofmt（产品源码；勿用裸 gofmt -l . —— .superpowers 有非法 UTF-16）
gofmt -l ./cmd ./internal
# 可选全仓扫描（排除噪声目录）见实现时脚本：Get-ChildItem *.go 过滤 .git/vendor/.superpowers/node_modules

Push-Location web\chat
npm ci
npm run lint
Pop-Location

# golangci v2.4.0（与 CI 一致；若代理失败可 GOPROXY=https://goproxy.cn,direct）
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0
$env:PATH = "$(go env GOPATH)\bin;$env:PATH"
golangci-lint run ./... --timeout=5m
# 摘要：46 issues（errcheck 20 / staticcheck 22 / ineffassign 2 / unused 2）；勿提交巨型原始日志
```

（§7 性能：只标注；仓库无既有 `Benchmark*`。探针建议见表，**不**在 AUDIT 执行重型压测。）
