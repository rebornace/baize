# 配置：YAML、环境变量与 CLI

权威结构见 `internal/config/config.go`；下列为现行常用键摘要。样板见 `configs/`（`minimal.yaml`、`demo.yaml`、`docker-*.yaml` 等）。

入口会 `LoadDotEnv(".env")`：已存在的进程环境变量**不会**被 `.env` 覆盖。

## CLI

| 命令 | 作用 |
|------|------|
| `baize start` | 生产默认：`configs/minimal.yaml`（若存在则优先 `minimal.local.yaml`）；须通过 `ValidateStart`（含 API Key） |
| `baize demo` | 试用栈：分层合并 `demo.yaml` + 可选 `default.local.yaml` / `demo.local.yaml` |
| `baize serve -config <path>` | 显式单文件配置启动 Runtime |
| `baize reset-credentials -config <path>` | 清空 store 中控制面热更新口令，回退 YAML break-glass |

旁路进程：`weixin-adapter`（见 [deployment](./deployment.md)）。

## YAML 顶层键

### `listen`

HTTP 监听地址，默认 `:8080`。非空环境变量 `BAIZE_LISTEN` 在加载后覆盖本字段（改端口需重启进程）。

### `store`

| 字段 | 含义 |
|------|------|
| `driver` | `sqlite` / `postgres` / `memory` 等已注册驱动 |
| `sqlite_path` | SQLite 路径；`driver: sqlite` 且空时默认 `./data/baize.db` |
| `dsn` | Postgres 等连接串 |

### `ui`

`enabled`：是否挂载静态 Chat UI。

### `llm`

启动期默认模型（首次可种子到库内 model profile；之后以库 / 设置为准）：

| 字段 | 默认 / 说明 |
|------|-------------|
| `provider` / `base_url` / `model` | 供应商与模型 |
| `api_key_env` | 默认 `BAIZE_API_KEY` |
| `disable_thinking` / `thinking_level` / `thinking_dialect` | 思考链相关 |
| `supports_vision` | 默认 `false`；视觉模型再开 |

### `skills`

| 字段 | 说明 |
|------|------|
| `builtin_dir` / `builtin_dirs` | 内置技能扫描根；`builtin_dirs` 非空时优先于 `builtin_dir` |
| `user_dir` | 默认 `./data/skills` |

### `agent`

`id`、`system`、默认激活 `skills[]`。

### `connector`

样板默认连接器：`id`、`type`（默认 `openapi`）、`spec`、`base_url`、`execution_callback_url`、`require_approval` / `require_approval_mutating`、`require_login`，以及 `auth`（`mode`、`static` / `passthrough` / `vault_ref` / `capture`）。

### `run`

| 字段 | 默认 |
|------|------|
| `max_steps` | 16 |
| `tool_timeout_sec` | 60（单次工具调用） |

### `conversation`

| 字段 | 默认 / 说明 |
|------|-------------|
| `max_messages` | 40 |
| `persist_identities` | `*bool`；省略默认持久化 |
| `compact_enabled` | 省略默认 `true` |
| `compact_threshold` | 0.8（越界回退） |
| `compact_reserve_output` | 8000 |
| `compact_recent_messages` | 8 |

### `middleware`

队列：`driver` 默认 `memory`，或 `redis`（`addr` / `db` / `username` / `password_env` / `stream` / `consumer_group` / `events_channel`）。另有 `worker_concurrency`、`lease_ttl_sec`、`reconcile_interval_sec`。

### `storage`

Blob：`driver` 为 `file` / `s3` / `memory`（空则 bootstrap 默认 memory）。`file.root_dir`；`s3.*`（`endpoint`、`region`、`bucket`、`prefix` 默认 `baize`、`access_key_env` / `secret_key_env` 默认 `S3_ACCESS_KEY` / `S3_SECRET_KEY`、`use_ssl`、`path_style`、`auto_create_bucket`）。

### `mock_ticket`

`listen`：演示 mock-ticket 侧车，默认 `:18080`；`off` 可关。

### `control_plane`

| 字段 | 说明 |
|------|------|
| `operator_token` / `admin_token` | 空=未配；支持 `env:VAR`、`file:/path` 或明文 |
| `operators[]` | `{ id, token }` 具名操作员 |

Gate 全空为开发态（无多运营隔离）。不是下游业务 IAM。

### `events.webhook`

出站 Run 事件：`url`、`headers`。

### `runtime`

侧车 callback 注入：`public_base_url`（空则不注入）、`callback_hmac_secret`、`callback_token_ttl_sec`（默认 3600）。

### `inbox`

`channels[]`：入站 Webhook Inbox 种子配置。

### `mcp_export`

`enabled`：`*bool`；省略默认开启 Streamable HTTP MCP 导出。

### `channels`

声明式渠道实例列表：

```yaml
channels:
  - name: weixin          # 实例名；同 type 多实例时必填且唯一
    type: webhook         # 注册表中的渠道类型键
    enabled: true         # 声明式模式下仅 true 才接线
    config:               # 不透明覆盖（source、secret、outbound_url…）
      source: weixin
```

**省略整段 `channels`** 时走 legacy：只 auto-wire 注册表中**非** `DeclarativeOnly` 的类型。像 `webhook`（微信等）属 DeclarativeOnly，必须在本节显式 `enabled: true`。

## 重要环境变量

| 变量 | 作用 |
|------|------|
| `BAIZE_LISTEN` | 非空时覆盖 YAML `listen` |
| `BAIZE_API_KEY` | 默认 LLM Key（可由 `llm.api_key_env` 改名） |
| `BAIZE_SETTINGS_KEY` | 设置 / 收件箱 / MCP OAuth 等密封；`demo` 未设时用临时 demo key |
| `BAIZE_OPERATOR_TOKEN` / `BAIZE_ADMIN_TOKEN` 等 | 经 YAML `env:…` 引用，非硬编码名 |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `storage.s3` 默认凭证 env |
| `BAIZE_TEST_PG_DSN` | **仅测试 / CI** Postgres |
| `WEIXIN_ADAPTER_SECRET` | Compose 独立微信适配器共享 HMAC（见 deployment） |

其它自定义 env 名由 YAML 字段声明（如 `middleware.redis.password_env`）。
