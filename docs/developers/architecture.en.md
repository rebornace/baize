[中文](./architecture.md) | **English**

# Architecture and plugin boundaries

Baize is a standalone **Agent Runtime** (sidecar / gateway): it turns LLM, tools, legacy HTTP APIs, and human checkpoints into auditable `Run`s. Integrate over REST / HTTP; there is no official language SDK (call the control-plane HTTP directly).

| Does | Does not |
|------|----------|
| OpenAPI → tools, HTTP plugins / MCP connectors, execution callbacks, human approval, optional linear workflows | Desktop app, official TS/Python SDK, heavy canvas / branching workflows, lock-in to one cloud or agent framework |
| SQLite by default; optional Postgres / Redis / S3; connector re-import **CatalogDiff**; **run.binding** execution-time dependency closure; optional OTLP (off by default); **Admin-authenticated** Prometheus **`GET /metrics`** (on by default); structured slog (`log_format` / `BAIZE_LOG_FORMAT`) | Require OTel/APM; per-tenant cgroup/egress sandbox; full IdP OBO / mTLS; client-side tool schema version pin; bit-identical reproducibility |

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
- **CatalogDiff:** After a successful connector re-import (OpenAPI / plugin / MCP), `PUT /v0/connectors/{id}` returns `catalog_diff` (added / removed / method·path·schema changes); Settings shows a short toast summary. The live catalog is source of truth — no client version pinning.
- **Execution-time dependency closure (R1):** Before the first main-model call, each Chat Run appends `run.binding` (model endpoint/profile, knobs, memory injection, narrowed tool contract fingerprints, connector `catalog_revision`); append failure fails the Run. Tool events carry `contract_hash` / `catalog_revision` for before/after fix comparison. See `GET /v0/runs/{id}/binding` and `GET /v0/connectors/{id}/catalog-revisions`.
- **Optional observability:** When `runtime.otel_otlp_endpoint` is set, the Runtime subscribes to the event bus and exports Run / `tool.result` (and MCP export calls) as OTLP spans; empty endpoint keeps the zero-cost path. Events may carry non-secret audit fields (e.g. `connector_id`); never tokens or passwords.
- **MCP export:** Runtime acts as an MCP server (`/v0/mcp/export`) and exposes a read-only subset of the tool catalog to MCP-capable Agent clients such as Cursor, Claude Desktop, or Doubao. Opposite direction from the bridge; does not go through Baize’s own chat model. Export Keys support **`identity_slots`** (bind identities per connector / optional role) — the permission boundary is the **downstream business account**, not end-user IdP OBO. On downstream auth failure, prefer URL elicitation; clients without elicitation get an immediate tool error with a public hosted login URL (`hosted_login_url`) — never paste passwords into the agent chat. `auth.mode: passthrough` only forwards whitelisted Chat Run headers downstream; also not OBO.
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
3. **Hosted login page** (`/v0/auth/hosted/...`): renders forms / OAuth from connector `hosted_login.methods`; submit invokes the login tool and capture. Target may be an MCP-export identity slot or the Chat workspace account pool (`conversation_id` + `workspace_id`). Credentials never go through the LLM. Configure methods and the public base URL under Settings → External login.
4. Logic the spec cannot cover → sidecar or execution callback.

Protocol major version is in the path and `v0` header; unknown major → `400 protocol_unsupported`.

## Tool catalog and Registry

- Catalog rows live in the store (`source`: `spec` / `plugin` / `mcp` / `extra`). `spec`/`plugin`/`mcp` can be disabled but not deleted; `extra` can be deleted.
- Registry mounts **only** `enabled=true` rows; the engine and approval path read the Registry; `GET /v0/tools` reads the catalog (disabled rows visible).
- Agents are **not** bound to a connector subset: by default each Run gives the model all enabled tools (tool names unique across connectors). When tool narrowing is on and the tool count exceeds the threshold, a tool prefilter (standard matching: BM25 by default; optional enhanced matching: BM25 + embedding hybrid via Settings → Matching & decisions and `internal/toolindex`) plus the decision layer first narrow the set, and only the candidate subset is sent to the main model (master switch, sub-features, and TopK live under Runtime → Smart speed-up; a decision backend must be configured before those toggles save main-model tokens). Skill `tools:` is a **floor** unless the agent sets `tool_binding: exclusive`: declared tools are preserved through narrowing; undeclared enabled connector tools remain candidates and are not hidden by the skill list alone. Exclusive mode exposes only tools on currently activated skills (plus `activate_skill`). Each user message is a new Run with fresh narrowing; once the conversation has session credentials, login-establish tools (and managed `connector_login` skill tool floors) are not force-injected into Top-K so a successful login stops crowding later turns. The system prompt includes the current wall-clock time so the model can interpret timestamps in tool results.
- **Tool matching (enhanced):** `internal/toolretrieval` manages one-click local Ollama install / model pull / paths and cleanup, or an OpenAI-compatible embedding API; see [HTTP API](./http-api.en.md) (`/v0/settings/tool-retrieval*`). On CN Windows, the installer prefers [ModelScope `Lixiang/ollama-release`](https://www.modelscope.cn/models/Lixiang/ollama-release/files) (resolves `OllamaSetup.exe` from the latest stable tag), then ghproxy / official; size-mismatched stale mirrors are skipped. When disabled or probing fails, standard matching remains the fallback path.
- **Decision service (System One):** `internal/systemoneenable` symmetrically manages local Ollama `tev1` pull (Ollama ≥0.35; shared installer path) or a System One-compatible API; enablement writes `decide_systemone_*` knobs (`/v0/settings/systemone*`). Judgment chain: System One → optional `decide_profile_id` standby model (empty never bills the main model) → Rules.
- **Workspaces:** Web chats carry `workspace_id` (default `default`). Captured logins are shared inside a workspace (Settings → Accounts); other workspaces, channel sessions (`source:account:peer`), and MCP-export identities (Settings → Expose capabilities) stay isolated. v1 does not move existing chats between workspaces.
- **Model-facing projection** (Runtime “keep important facts in long chats”, default off): edits only the projection fed to the model (pin / summary / drop bulky tool results); it does **not** rewrite `conversation.Messages`. Fail-open to the Compactor. Does not promise lower cloud API tokens.
- **Rolling summary (Compactor):** when over budget, older turns become a structured checkpoint (goal, progress, decisions, facts to keep). Raw messages stay; the prompt must not encode a global “do this before exploring” policy.

## Agent run shapes

| Mode | Behavior |
|------|----------|
| Default | Single-agent ReAct: pick tool → execute → write trail → finish |
| Optional | Linear `workflow.yaml` inside a Skill (ordered steps + optional approval; no branch / loop) |
| Skill | Config shape: `SKILL.md` (optional `SKILL.en.md`) + tools; `agent.skills` pre-activates new chats (sample includes `baize-help`) while unchecked skills stay reachable via `@`/`activate_skill`; `@id`/`/id` pins are stored on the user message as `pinned_skills` (visible in history); Composer completes `/reload`; catalog reload via `skills_reload`/`/reload` or auto dirty; optional `tool_binding: exclusive` |
| Channel | Signed inbox; IM via out-of-process adapters + declarative `channels` (see [Deployment](./deployment.en.md)). **This repo ships a personal WeChat adapter today**; the channel capability is not limited to WeChat |

## Human approval

Writes can set `require_approval`: Run enters `waiting_human`; ops / API `POST /v0/runs/{id}/resume` continues. Chat UI (`/ui`) is the operator entry.

When downstream login is required (`login_required`), chat can pause with a hosted-login entry: open the browser hosted page, skip that connector for this turn, or end the turn. For MCP export, see auth-failure → elicitation / `hosted_login_url` above.
