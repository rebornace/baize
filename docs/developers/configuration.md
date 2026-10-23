**中文** | [English](./configuration.en.md)

# 配置：YAML、环境变量与 CLI

权威结构见 `internal/config/config.go`；下列为现行常用键摘要。样板见 `configs/`（`config.yaml`、`config.local.yaml.example`、`docker-*.yaml` 等）。

入口会 `LoadDotEnv(".env")`：已存在的进程环境变量**不会**被 `.env` 覆盖。

## CLI

| 命令 | 作用 |
|------|------|
| `baize serve` | 以默认 `configs/config.yaml` 启动 Runtime；须通过 `Validate`（含 API Key） |
| `baize serve -config <path>` | 显式单文件配置启动（如本地用 `configs/config.local.yaml`） |
| `baize reset-credentials -config <path>` | 清空 store 中控制面热更新口令，回退 YAML break-glass |

旁路进程：`weixin-adapter`（见 [部署](./deployment.md)，[English](./deployment.en.md)）。

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

`id`、`system`、默认激活 `skills[]`。样板默认含内置技能 `baize-help`（产品说明，不能改 YAML / 重启进程）。

### `connector`

样板默认连接器：`id`、`type`（默认 `openapi`）、`spec`、`base_url`、`execution_callback_url`、`require_approval` / `require_approval_mutating`、`require_login`，以及 `auth`（`mode`、`static` / `passthrough` / `vault_ref` / `capture`）。

可选 **`tls`**（出站 mTLS，本机文件路径）：

| 字段 | 含义 |
|------|------|
| `cert_file` | 客户端证书 PEM 路径 |
| `key_file` | 客户端私钥 PEM 路径 |
| `ca_file` | 可选；追加到系统 RootCAs 的 CA PEM 路径 |

`cert_file` 与 `key_file` **必须成对**（仅一侧非空 → Apply / PUT 错误）。仅 `ca_file` 合法（只定制信任锚）。三者皆空等同未配置。Store **只存路径字符串**，不读入/不持久化 PEM 正文。覆盖 **OpenAPI**、**HTTP 插件** 与 **MCP HTTP**（discover + `tools/call`）；与 `auth.mode` **正交**。**stdio** MCP 不适用。

**设置页可配**：连接器编辑器（业务系统 / 外部工具服务 / 插件）高级区「出站 TLS / mTLS」——填写 **Runtime 本机路径**（不是浏览器上传 PEM）。省略 `tls` 字段保留服务端原值；三者清空并保存可清除配置。

### `connector_groups`

可选。命名连接器集合，供工具收窄时把同域其它连接器仍留在候选内。示例见样板 `configs/config.yaml` 注释：

```yaml
connector_groups:
  - id: finance
    connectors: [expense, invoice]
```

省略则行为与未分组时相同。分组草稿也可在设置「匹配与决策 → 连接器分组」热保存（KV），不必改 YAML 重启。「允许分组内子任务」打开后才可 `spawn_group_run`；边界见 [架构](./architecture.md)。

### `run`

| 字段 | 默认 |
|------|------|
| `max_steps` | 16 |
| `tool_timeout_sec` | 60（单次工具调用） |

> 决策层（`internal/decide`）相关行为——记忆预抽取判断、工具候选收敛、工具枚举约束、大体积工具结果裁剪、Auto 档位回退——**不在 YAML**，而是库内的热更新参数，在 UI「运行参数 → 智能提速」调节（总开关 `decide_enabled` + 各功能开关，默认均关），经 `GET/PATCH /v0/settings/runtime` 读写。YAML 只提供启动基线。同页「记忆与压缩」含「长对话保留重点」（`context_projection_enabled`，默认关）：只整理给模型看的投影，不改写已保存消息。
>
> **匹配与决策**（工具预筛 + 决策模型）在独立设置页配置：增强匹配经 `/v0/settings/tool-retrieval*`；决策模型经 `/v0/settings/systemone*`（本机 Ollama ≥0.35 拉默认模型 `tev1`，或 API）。`decide_systemone_*` 由启用流程写入 knobs；`decide_profile_id` 仅为决策模型不可用时的**备用模型**（留空不会改用主模型）。不在 YAML。


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

### `control_plane`

| 字段 | 说明 |
|------|------|
| `operator_token` / `admin_token` | 空=未配；支持 `env:VAR`、`file:/path` 或明文 |
| `operators[]` | `{ id, token }` 具名操作员 |

Gate 全空为开发态（无多运营隔离）。不是下游业务 IAM。

### `events.webhook`

出站事件：`url`、`headers`（与 Run 事件共用同一全局 URL、重试与死信 outbox）。

| 载荷 `type` | 何时投递 |
|-------------|----------|
| Run 轨迹事件 | Chat Run 生命周期（既有行为） |
| `catalog.diff` | 连接器 `PUT` / Apply **成功持久化后**，且 `catalog_diff` **非空** |

`catalog.diff` JSON 含 `type`、`at`（RFC3339 UTC）、`connector_id`、`catalog_revision` 与 `diff`（added / removed / changed）；**不含** token、密码或完整 schema 正文。URL 为空 → 静默跳过；入队或 POST 失败 → **fail-open**（Apply/PUT 仍成功）。独立 `catalog_diff_webhook_url`、Inbox/邮件告警 **不在**本版本范围。连接器出站 mTLS 见上方 `connector.tls`。

**设置页说明**：设置 → Webhook 会提示同一 `events.webhook` 也会接收 `type=catalog.diff`；不改投递模型。

### `runtime`

- `public_base_url`：公网可达的 Runtime 基址（无尾斜杠）。用于向侧车注入 `callback_urls`，以及签发**托管登录页** / MCP 重登链接（`/v0/auth/hosted/...`）。空则不注入 callback，也无法生成外网可点的托管登录 URL。云端 Agent / 浏览器须能打开该地址（勿对公网场景使用仅本机可达的 `127.0.0.1`）。也可在设置「运行参数」或「对外登录」向导中填写。
- `callback_hmac_secret`、`callback_token_ttl_sec`（默认 3600）：侧车 callback 签名。
- `otel_otlp_endpoint`：可选 OTLP HTTP traces 端点（如 `http://localhost:4318`）。**空=关闭**，不拨号、不影响默认路径。打开后把 Run 生命周期与 `tool.result`（及 MCP 导出调用）导出为 span；Chat `tool.result` span 在事件 Data 有时写入 `baize.contract_hash`、`baize.catalog_revision`；失败 fail-open。
- `otel_service_name`：OTel `service.name`，默认 `baize`（仅在 endpoint 非空时生效）。
- `log_format`：`text`（默认）或 `json`；也可用环境变量 **`BAIZE_LOG_FORMAT`**（非法值回退 `text`）。启动时初始化 `slog` 输出到 stderr，并 tee 到进程内环形缓冲（约最近 500 行），供管理后台与 `GET /v0/settings/logs` 查看；**不写日志库**，重启清空。完整落盘取决于进程 stderr 的重定向（systemd / Docker 等）。
- **热更新**：`GET/PATCH /v0/settings/runtime` 可读写有效 `log_format` / `otel_otlp_endpoint` / `otel_service_name`（设置 → 运行参数 → **观测**）。PATCH 写入运行时 KV 覆盖并立即生效（换 slog Handler / 热换 OTLP exporter）；**不写回 YAML**。空 endpoint = 强制关闭 OTel。`observability_reset: true` 清除覆盖并恢复 YAML（及启动时 `BAIZE_LOG_FORMAT` 写入的基线）。`BAIZE_LOG_FORMAT` 仅影响启动基线，不热改环境变量本身。
- **Prometheus 刮取**：根路径 **`GET /metrics`** 默认挂载（无关闭开关）；需 **Admin Bearer**（`Authorization: Bearer <admin_token>`）。Operator 口令返回 `403`。计数器在进程内，重启清零；长期留存请外部 Prometheus 等定期 scrape。
- **管理后台**：设置 → **运行指标**（仅管理员）可同源查看指标文本与近期日志，无需另开终端；页头链到运行参数观测说明。

### 身份模型（透传 / 导出身份 ≠ IdP OBO）

| 机制 | 含义 | 不是 |
|------|------|------|
| `auth.mode: passthrough` | 该次 Chat Run 把白名单请求头（常为 `Authorization`）带给下游 HTTP | 终端用户 IdP 的 on-behalf-of / 委托令牌交换 |
| 导出身份 / `identity_slots` | MCP 导出 Key 绑定的**下游业务账号**凭证（Headers）；权限边界=该业务账号能做什么 | 完整 OIDC OBO、按终端用户重放 IdP token、服务端强制对方 mTLS（连接器出站客户端证书见 `connector.tls`） |
| 设置 → 账号（工作区） | Chat 捕获的业务登录，按产品工作区隔离 | MCP 导出身份池 |

Per-tenant cgroup / egress 沙箱 **不在**本版本范围（沙箱交给编排）。

### `inbox`

`channels[]`：入站 Webhook Inbox 种子配置。

### `mcp_export`

`enabled`：`*bool`；省略默认开启 Streamable HTTP MCP 导出。导出 Key、导出身份（Headers）与按连接器的 **`identity_slots`** 在设置「对外提供能力」中管理（HTTP：`/v0/settings/mcp-export*`），不写在本 YAML 段。近期调用审计：`GET /v0/settings/mcp-export/invocations`（设置页「调用审计」列表；仅管理员）。

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
| `BAIZE_CORS_ORIGINS` | 逗号分隔的浏览器 Origin；非空时为控制面 API 开 CORS（Capacitor 捆绑 www 跨域调用）。手机壳优先用同源 `/ui`，见 [移动端](./mobile.md) |
| `BAIZE_SETTINGS_KEY` | 设置 / 收件箱 / MCP OAuth 等密封；未设置时无法密封或打开密钥（相关写接口返回 400），生产必须配置 |
| `BAIZE_OPERATOR_TOKEN` / `BAIZE_ADMIN_TOKEN` 等 | 经 YAML `env:…` 引用，非硬编码名 |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | `storage.s3` 默认凭证 env |
| `BAIZE_TEST_PG_DSN` | **仅测试 / CI** Postgres |
| `WEIXIN_ADAPTER_SECRET` | Compose 独立微信适配器共享 HMAC（见 deployment） |

其它自定义 env 名由 YAML 字段声明（如 `middleware.redis.password_env`）。
