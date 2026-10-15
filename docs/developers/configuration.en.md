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

`id`, `system`, and default active `skills[]`. The sample defaults include built-in skill `baize-help` (product help; it cannot edit YAML or restart the process).

### `connector`

Sample default connector: `id`, `type` (default `openapi`), `spec`, `base_url`, `execution_callback_url`, `require_approval` / `require_approval_mutating`, `require_login`, plus `auth` (`mode`, `static` / `passthrough` / `vault_ref` / `capture`).

Optional **`tls`** (outbound mTLS, local file paths):

| Field | Meaning |
|-------|---------|
| `cert_file` | Client certificate PEM path |
| `key_file` | Client private key PEM path |
| `ca_file` | Optional CA PEM path appended to the system RootCAs |

`cert_file` and `key_file` must be a **pair** (only one set → Apply / PUT error). `ca_file` alone is valid (custom trust anchors only). All empty means no TLS config. Store keeps **path strings only** — never PEM bodies. Applies to **OpenAPI**, **HTTP plugin**, and **MCP HTTP** (discover + `tools/call`); orthogonal to `auth.mode`. **stdio** MCP does not use TLS.

### `run`

| Field | Default |
|------|------|
| `max_steps` | 16 |
| `tool_timeout_sec` | 60 (per tool call) |

> The decision layer (`internal/decide`) behaviors — memory pre-extraction judgment, tool-candidate narrowing, tool-choice enum constraint, bulky tool-result pruning, Auto-tier fallback — are **not YAML keys**. They are in-store hot-reloadable settings under Runtime → Smart speed-up (master `decide_enabled` plus per-feature toggles, all off by default) via `GET/PATCH /v0/settings/runtime`; YAML only supplies the startup baseline. Under Memory & compaction, “keep important facts in long chats” (`context_projection_enabled`, default off) is a model-facing projection only; saved messages are not rewritten.
>
> **Matching & decisions** (tool prefilter + System One) lives on its own settings page: enhanced matching via `/v0/settings/tool-retrieval*`; the decision model via `/v0/settings/systemone*` (local Ollama ≥0.35 pulls `tev1`, or an API). Enablement writes `decide_systemone_*` knobs; `decide_profile_id` is only a **standby model** when System One is unavailable (empty never bills the main model). Not in YAML.


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

Outbound events: `url`, `headers` (same global URL, retry, and dead-letter outbox as Run events).

| Payload `type` | When |
|----------------|------|
| Run trace events | Chat Run lifecycle (existing) |
| `catalog.diff` | After connector `PUT` / Apply **persists successfully** and `catalog_diff` is **non-empty** |

`catalog.diff` JSON includes `type`, `at` (RFC3339 UTC), `connector_id`, `catalog_revision`, and `diff` (added / removed / changed); **never** tokens, passwords, or full schema bodies. Empty URL → skip silently; enqueue or POST failure → **fail-open** (Apply/PUT still succeeds). A separate `catalog_diff_webhook_url` and Inbox/email alerting are **out of scope** for this version. Connector outbound mTLS is under `connector.tls` above.

### `runtime`

- `public_base_url`: publicly reachable Runtime base URL (no trailing slash). Used to inject sidecar `callback_urls` and to mint **hosted login** / MCP re-auth links (`/v0/auth/hosted/...`). Empty → no callback injection and no externally clickable hosted-login URL. Cloud agents / browsers must be able to open this address (do not use loopback-only `127.0.0.1` for public scenarios). Can also be set under Runtime settings or the External login wizard.
- `callback_hmac_secret`, `callback_token_ttl_sec` (default 3600): sidecar callback signing.
- `otel_otlp_endpoint`: optional OTLP HTTP traces endpoint (e.g. `http://localhost:4318`). **Empty = off** — no dial, default path unchanged. When set, Run lifecycle and `tool.result` (plus MCP export calls) are exported as spans; Chat `tool.result` spans add `baize.contract_hash` and `baize.catalog_revision` when present on event Data; export failures are fail-open.
- `otel_service_name`: OTel `service.name`, default `baize` (only when endpoint is set).
- `log_format`: `text` (default) or `json`; override with env **`BAIZE_LOG_FORMAT`** (invalid values fall back to `text`). Initializes `slog` to stderr at startup and tees into an in-process ring (~last 500 lines) for Settings and `GET /v0/settings/logs`. **No log database**; cleared on restart. Durable capture depends on how you redirect process stderr (systemd / Docker / etc.).
- **Prometheus scrape**: root **`GET /metrics`** is always mounted (no off switch); requires **Admin Bearer** (`Authorization: Bearer <admin_token>`). Operator tokens get `403`. Counters are in-process and reset on restart; scrape with an external Prometheus for long-term retention.
- **Admin UI**: Settings → **Runtime metrics** (admin only) shows metrics text and recent logs same-origin—no separate terminal required.

### Identity model (passthrough / export identity ≠ IdP OBO)

| Mechanism | Meaning | Not |
|-----------|---------|-----|
| `auth.mode: passthrough` | That Chat Run forwards whitelisted request headers (often `Authorization`) to downstream HTTP | End-user IdP on-behalf-of / token exchange |
| Export identity / `identity_slots` | Downstream **business account** credentials bound to an MCP export Key (Headers); permission boundary = what that account can do | Full OIDC OBO, replaying the end-user IdP token, or server-enforced peer mTLS (connector outbound client certs: see `connector.tls`) |
| Settings → Accounts (workspace) | Captured Chat logins, isolated per product workspace | The MCP export identity pool |

Per-tenant cgroup / egress sandboxing is **out of scope** for this version (sandboxing belongs to the orchestrator).

### `inbox`

`channels[]`: seed config for inbound Webhook Inbox.

### `mcp_export`

`enabled`: `*bool`; omitted defaults to enabling Streamable HTTP MCP export. Export Keys, export identities (Headers), and per-connector **`identity_slots`** are managed under Settings → Expose capabilities (HTTP: `/v0/settings/mcp-export*`), not in this YAML section.

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
