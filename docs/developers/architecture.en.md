[中文](./architecture.md) | **English**

# Architecture and plugin boundaries

Baize is a standalone **Agent Runtime** (sidecar / gateway): it turns LLM, tools, legacy HTTP APIs, and human checkpoints into auditable `Run`s. Integrate over REST / HTTP; there is no official language SDK (call the control-plane HTTP directly).

| Does | Does not |
|------|----------|
| OpenAPI → tools, HTTP plugins / MCP connectors, execution callbacks, human approval, optional linear workflows | Desktop app, official TS/Python SDK, heavy canvas / branching workflows, lock-in to one cloud or agent framework |
| SQLite by default; optional Postgres / Redis / S3 | Require OTel or a specific APM as a runtime must-have |

**Five abstractions:** `Runtime` · `Agent` · `Tool` · `Connector` · `Run`  
Skill / Memory / Channel / linear workflow are configuration or plugin shapes — not a sixth abstraction.

## Logical architecture

```
Enterprise / integrator
  REST control plane · SSE / Webhook events · Chat UI
          │
          ▼
    Baize Runtime (Go)
  Agent · LLM (OpenAI-compatible / in-store model profiles)
  Tool catalog · Run engine (default ReAct · optional linear workflow) · human approval
  Connector clients
     │           │            │
     ▼           ▼            ▼
 OpenAPI        HTTP plugin v0   MCP bridge
 legacy REST    sidecar          MCP server
```

- **MCP bridge (client):** `PUT /v0/connectors/{id}` with `type: mcp`; `tools/list` → catalog `source=mcp`; `tools/call` inside a Run.
- **MCP export:** Runtime acts as an MCP server (`/v0/mcp/export`) and exposes a read-only subset of the tool catalog to MCP-capable Agent clients such as Cursor or Claude Desktop. Opposite direction from the bridge; does not go through Baize’s own chat model.
- **Storage:** SQLite by default; `postgres` / `memory` supported. Blob: `file` / `s3` / `memory`.
- **Control-plane auth:** optional operator / admin tokens; see [HTTP API](./http-api.en.md) and [Configuration](./configuration.en.md).

## Plugin protocol v0 (HTTP + JSON)

Sidecars talk to the Runtime over **same-network HTTP**. The built-in OpenAPI connector does not use this protocol. MCP maps to internal tools and does not replace this protocol.

### Conventions

- Base URL is declared when registering the connector.
- Headers: `Authorization` (passthrough or injected), `X-Baize-Run-Id`, `X-Baize-Tenant-Id` (optional), `X-Baize-Protocol: v0`.
- Error body: `{ "error": { "code", "message", "retryable" } }`.

### Sidecar must implement

```http
GET  /healthz                     → 200 { "status": "ok" }
GET  /v0/tools                    → { "tools": [ ToolDesc, ... ] }
POST /v0/tools/{tool_name}/invoke → ToolResult
```

Invoke includes `arguments` and `context` (`run_id`, `agent_id`, `tenant_id`, optional `callback_urls`). When `runtime.public_base_url` (and HMAC) is set and `run_id` is present, the Runtime injects short-lived signed `callback_urls.event`; the sidecar may POST Run events (`plugin.callback`). No injection if `public_base_url` is empty.

### Enterprise execution callback

Connector `execution_callback_url`: Runtime POSTs tool name, arguments, `run_id`, `idempotency_key`, and optional `callback_urls`. The enterprise side does not need to implement `/v0/tools` discovery.

### Built-in OpenAPI connector

1. Import OpenAPI 3.x → each operation → one Tool (name prefers `operationId`).
2. Credential priority (summary): forced `identity_id` → unexpired conversation Identity → connector default headers **only when** there is no `conversation_id` → empty. With a conversation, `require_login` tools without credentials do not call downstream HTTP. Full secrets never appear in events / GET run.
3. Logic the spec cannot cover → sidecar or execution callback.

Protocol major version is in the path and `v0` header; unknown major → `400 protocol_unsupported`.

## Tool catalog and Registry

- Catalog rows live in the store (`source`: `spec` / `plugin` / `mcp` / `extra`). `spec`/`plugin`/`mcp` can be disabled but not deleted; `extra` can be deleted.
- Registry mounts **only** `enabled=true` rows; the engine and approval path read the Registry; `GET /v0/tools` reads the catalog (disabled rows visible).
- Agents are **not** bound to a connector subset: each Run gives the model all enabled tools (tool names unique across connectors).

## Agent run shapes

| Mode | Behavior |
|------|----------|
| Default | Single-agent ReAct: pick tool → execute → write trail → finish |
| Optional | Linear `workflow.yaml` inside a Skill (ordered steps + optional approval; no branch / loop) |
| Skill | Config shape: `SKILL.md` + tools; `agent.skills` defaults; `activate_skill` widens the current Run only |
| Channel | Signed inbox; IM via out-of-process adapters + declarative `channels` (see [Deployment](./deployment.en.md)). **This repo ships a personal WeChat adapter today**; the channel capability is not limited to WeChat |

## Human approval

Writes can set `require_approval`: Run enters `waiting_human`; ops / API `POST /v0/runs/{id}/resume` continues. Chat UI (`/ui`) is the operator entry.
