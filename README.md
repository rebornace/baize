# Baize

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**English** | [中文](README.zh-CN.md)

Lightweight **Agent Runtime** for enterprise legacy HTTP APIs. Import an OpenAPI spec, turn operations into tools, run auditable ReAct loops in-process, gate mutating calls with human-in-the-loop (HITL), and operate from an embedded Chat UI.

> Not a personal assistant. Baize sits beside your existing systems as a sidecar-style gateway: **OpenAPI → Tools → HTTP invoke**, with optional approval and session-scoped credentials.

---

## Why Baize

| Problem | Baize approach |
|--------|----------------|
| Legacy HTTP / OpenAPI systems with no agent layer | Register a connector; operations become tools automatically |
| Unsafe write paths for LLM agents | HITL approval for mutating tools before invoke |
| Opaque agent behavior | Every step is a `Run` with structured events |
| Multi-account / admin vs user tokens | Session identity store + pluggable auth resolver (OpenAPI security schemes by default) |

### Core concepts

| Concept | Role |
|---------|------|
| **Runtime** | Process hosting agents, tools, store, and HTTP control plane |
| **Agent** | System prompt + LLM binding that drives a Run |
| **Tool** | One invokable capability (usually one OpenAPI operation) |
| **Connector** | Binding of a spec + `base_url` (+ auth) into the tool registry |
| **Run** | One execution: input → ReAct loop → output / HITL / failure |

---

## Quick start

**Requirements:** Go 1.22+

```bash
git clone https://github.com/rebornace/baize.git
cd baize
go run ./cmd/baize start
```

**Windows (one-shot launcher):**

```powershell
.\start.cmd
```

This sets a usable `GOPROXY` when needed and runs `go run ./cmd/baize start`.

Default sample:

- Runtime: `http://127.0.0.1:8080`
- Mock ticket API: `http://127.0.0.1:18080`
- LLM: built-in `mock` (no API key)
- Chat UI: http://127.0.0.1:8080/ui

If ports are busy, stop the previous `baize` process and retry.

### Create a ticket (curl)

`POST /v0/runs` is async: it returns `run_id` immediately. `create_ticket` requires approval by default (`waiting_human`).

```bash
curl -s -X POST http://127.0.0.1:8080/v0/runs \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"ticket-agent\",\"input\":\"Create an urgent ticket: VPN is down\",\"conversation_id\":\"conv_example_1\"}"
```

```bash
curl -s http://127.0.0.1:8080/v0/runs/<run_id>
```

### HITL resume

```bash
# Approve
curl -s -X POST http://127.0.0.1:8080/v0/runs/<run_id>/resume \
  -H "Content-Type: application/json" \
  -d "{\"decision\":\"approve\",\"comment\":\"ok\"}"

# Reject
curl -s -X POST http://127.0.0.1:8080/v0/runs/<run_id>/resume \
  -H "Content-Type: application/json" \
  -d "{\"decision\":\"reject\",\"comment\":\"nope\"}"
```

Default `configs/default.yaml` uses SQLite. After a Runtime restart, `waiting_human` runs can still be resumed.

```bash
curl -s http://127.0.0.1:18080/tickets
curl -s http://127.0.0.1:8080/v0/runs/<run_id>/events
```

---

## Platform integration (OpenAPI)

Primary path: register a legacy OpenAPI service as tools.

1. Prepare an OpenAPI file and a reachable `base_url`.
2. Register / replace a connector:

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/ticket-api \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"openapi\",\"spec\":\"examples/mock-ticket/openapi.yaml\",\"base_url\":\"http://127.0.0.1:18080\",\"require_approval\":[\"create_ticket\"]}"
```

3. Inspect tools:

```bash
curl -s http://127.0.0.1:8080/v0/tools
```

You should see `method`, `path`, `operation_id`, and `connector_id`.

4. Run via `POST /v0/runs` or open `/ui`.

Re-`PUT` with the same connector `id` **replaces** that connector’s tools. Invalid specs return `400` without corrupting the existing registry.

> Note: `PUT /v0/connectors` does not yet attach session Identities / Resolver / Capture. The `baize start` path wires those for the configured connector. Hot-reload of auth session wiring is planned.

---

## Session identities

Successful login tools can **capture** tokens into a per-`conversation_id` identity store. Later runs in the same conversation automatically attach the resolved Bearer (default resolver follows OpenAPI `securitySchemes`).

- Chat UI sidebar: list accounts (redacted), set default, sign out
- New chat → new `conversation_id`
- `bearer_env` is a **startup fallback**, not the only identity source
- Default capture matches `*login*` and reads `accessToken` / `data.token` (override via `connector.auth.capture`; set `tool_name_glob: "__none__"` to disable)
- If local config only sets `bearer_env`, `baize start` applies capture defaults automatically
- Restart Runtime after changing auth/capture config

```bash
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/identities
```

---

## Conversation memory

With the default SQLite driver, Baize persists conversation messages and captured identities to `data/baize.db` (configurable via `store.sqlite_path`). Restarting the Runtime restores prior turns and signed-in accounts for each `conversation_id`.

- `conversation.max_messages` (default `40`) bounds the window of recent turns fed back to the LLM as context. Older turns stay in the database for audit but are dropped from the prompt. Values `<=0` are normalized to `40` on load.
- Clearing chat (`DELETE /v0/conversations/{id}/messages`) wipes the message history **only** — it does **not** sign the user out. Captured identities remain until removed via the identities API.
- `data/baize.db` is a runtime artifact; do not commit it to git (the default `.gitignore` already excludes `data/`).

```bash
# List persisted turns
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages

# Clear turns without signing out
curl -s -X DELETE http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages
```

---

## Real LLM & real backends

Do **not** put secrets in YAML.

> If you previously used `configs/demo.local.yaml`, rename it to `configs/default.local.yaml` and change the `demo.ticket_listen` key to `mock_ticket.listen`.

1. Copy `configs/default.yaml` → `configs/default.local.yaml` (gitignored; preferred by `baize start`)
2. Copy `.env.example` → `.env` and set `BAIZE_API_KEY` (and optionally `BAIZE_CONNECTOR_TOKEN`)
3. Example `default.local.yaml` fragment:

```yaml
llm:
  provider: openai_compatible
  base_url: https://api.deepseek.com
  model: deepseek-v4-flash
  disable_thinking: true
  api_key_env: BAIZE_API_KEY

connector:
  # point at your OpenAPI + base_url
  require_approval_mutating: true
  auth:
    bearer_env: BAIZE_CONNECTOR_TOKEN
    capture:
      tool_name_glob: "*login*"
      token_json_paths: ["accessToken", "data.accessToken", "data.token"]
      header_template: "Bearer {{token}}"
      default_scheme: "bearer"

mock_ticket:
  listen: "off"   # disable mock-ticket when using a real backend
```

```bash
go run ./cmd/baize start
# or: go run ./cmd/baize serve -config configs/default.local.yaml
```

---

## Chat UI build (optional)

Prebuilt assets live under `internal/ui/dist` (`//go:embed`). To rebuild after editing `web/chat` (Node 18+):

```bash
cd web/chat
npm ci
npm run build
```

---

## Commands

| Command | Description |
|---------|-------------|
| `baize start` | Load `default.local.yaml` if present, else `default.yaml`; start Runtime (+ mock ticket unless `mock_ticket.listen: off`) |
| `baize serve -config <path>` | Runtime only, explicit config |

---

## Documentation

- [Architecture & plugin protocol (draft)](docs/architecture-and-plugin-protocol.md)

---

## License

[MIT](LICENSE)
