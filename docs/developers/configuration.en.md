[中文](./configuration.md) | **English**

# Configuration: YAML, environment variables, and CLI

Authoritative shapes live in `internal/config/config.go`; the tables below summarize commonly used keys. Samples are under `configs/` (`config.yaml`, `config.local.yaml.example`, `docker-*.yaml`, and so on).

The entrypoint calls `LoadDotEnv(".env")`: process environment variables that already exist are **not** overwritten by `.env`.

## CLI

| Command | Role |
|------|------|
| `baize serve` | Start Runtime with the default `configs/config.yaml`; must pass `Validate` (including API Key) |
| `baize serve -config <path>` | Start with an explicit single config file (e.g. `configs/config.local.yaml` locally) |
| `baize reset-credentials -config <path>` | Clear control-plane hot-update credentials in the store; fall back to YAML break-glass |

Sidecar process: `weixin-adapter` (see [Deployment](./deployment.md), [English](./deployment.en.md)).

## Top-level YAML keys

### `listen`

HTTP listen address; default `:8080`. A non-empty `BAIZE_LISTEN` environment variable overrides this field after load (changing the port requires a process restart).

### `store`

| Field | Meaning |
|------|------|
| `driver` | Registered drivers such as `sqlite` / `postgres` / `memory` |
| `sqlite_path` | SQLite path; when `driver: sqlite` and empty, defaults to `./data/baize.db` |
| `dsn` | Connection string for Postgres and similar |

### `ui`

`enabled`: whether to mount the static Chat UI.

### `llm`

Startup default model (may seed a model profile in the store on first run; afterward the store / settings win):

| Field | Default / notes |
|------|-------------|
| `provider` / `base_url` / `model` | Provider and model |
| `api_key_env` | Default `BAIZE_API_KEY` |
| `disable_thinking` / `thinking_level` / `thinking_dialect` | Thinking-chain related |
| `supports_vision` | Default `false`; enable for vision models |

### `skills`

| Field | Notes |
|------|------|
| `builtin_dir` / `builtin_dirs` | Built-in skill scan roots; when `builtin_dirs` is non-empty it takes precedence over `builtin_dir` |
| `user_dir` | Default `./data/skills` |

### `agent`

`id`, `system`, and default active `skills[]`.

### `connector`

Sample default connector: `id`, `type` (default `openapi`), `spec`, `base_url`, `execution_callback_url`, `require_approval` / `require_approval_mutating`, `require_login`, plus `auth` (`mode`, `static` / `passthrough` / `vault_ref` / `capture`).

### `run`

| Field | Default |
|------|------|
| `max_steps` | 16 |
| `tool_timeout_sec` | 60 (per tool call) |

> The decision layer (`internal/decide`) behaviors — DP-1 memory pre-extraction judgment, DP-2a tool-candidate narrowing, DP-2b tool-choice enum constraint, DP-3 bulky tool-result pruning, DP-4 Auto-tier fallback, and the decision-model selection — are **not YAML keys**. They are in-store hot-reloadable settings tuned on the Runtime page and read/written via `GET/PATCH /v0/settings/runtime`; YAML only supplies the startup baseline.

### `conversation`

| Field | Default / notes |
|------|-------------|
| `max_messages` | 40 |
| `persist_identities` | `*bool`; omitted means persist by default |
| `compact_enabled` | omitted defaults to `true` |
| `compact_threshold` | 0.8 (falls back when out of range) |
| `compact_reserve_output` | 8000 |
| `compact_recent_messages` | 8 |

### `middleware`

Queue: `driver` defaults to `memory`, or `redis` (`addr` / `db` / `username` / `password_env` / `stream` / `consumer_group` / `events_channel`). Also `worker_concurrency`, `lease_ttl_sec`, `reconcile_interval_sec`.

### `storage`

Blob: `driver` is `file` / `s3` / `memory` (empty → bootstrap defaults to memory). `file.root_dir`; `s3.*` (`endpoint`, `region`, `bucket`, `prefix` default `baize`, `access_key_env` / `secret_key_env` default `S3_ACCESS_KEY` / `S3_SECRET_KEY`, `use_ssl`, `path_style`, `auto_create_bucket`).

### `control_plane`

| Field | Notes |
|------|------|
| `operator_token` / `admin_token` | Empty = unset; supports `env:VAR`, `file:/path`, or plaintext |
| `operators[]` | Named operators `{ id, token }` |

All gates empty means development mode (no multi-operator isolation). This is not downstream business IAM.

### `events.webhook`

Outbound Run events: `url`, `headers`.

### `runtime`

Sidecar callback injection: `public_base_url` (empty → no injection), `callback_hmac_secret`, `callback_token_ttl_sec` (default 3600).

### `inbox`

`channels[]`: seed config for inbound Webhook Inbox.

### `mcp_export`

`enabled`: `*bool`; omitted defaults to enabling Streamable HTTP MCP export.

### `channels`

Declarative channel instance list:

```yaml
channels:
  - name: weixin          # instance name; required and unique when multiple instances share a type
    type: webhook         # channel type key in the registry
    enabled: true         # in declarative mode, only true is wired
    config:               # opaque overrides (source, secret, outbound_url…)
      source: weixin
```

**Omitting the entire `channels` section** uses legacy behavior: only auto-wire registry types that are **not** `DeclarativeOnly`. Types such as `webhook` (WeChat and similar) are DeclarativeOnly and must be explicitly `enabled: true` in this section.

## Important environment variables

| Variable | Role |
|------|------|
| `BAIZE_LISTEN` | When non-empty, overrides YAML `listen` |
| `BAIZE_API_KEY` | Default LLM key (rename via `llm.api_key_env`) |
| `BAIZE_SETTINGS_KEY` | Sealing for settings / inbox / MCP OAuth, etc.; when unset, secrets cannot be sealed or opened (the related write endpoints return 400) — required in production |
| `BAIZE_OPERATOR_TOKEN` / `BAIZE_ADMIN_TOKEN`, etc. | Referenced via YAML `env:…`, not hard-coded names |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | Default credential env names for `storage.s3` |
| `BAIZE_TEST_PG_DSN` | Postgres for **tests / CI only** |
| `WEIXIN_ADAPTER_SECRET` | Shared HMAC for the Compose WeChat adapter (see deployment) |

Other custom env names are declared by YAML fields (for example `middleware.redis.password_env`).
