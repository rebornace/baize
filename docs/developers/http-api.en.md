[中文](./http-api.md) | **English**

# Control-plane HTTP overview

Baize Runtime exposes a REST-style control plane under **`/v0`** (plus `GET /healthz` and optional static `/ui/`).

The **authoritative route table** is the `HandleFunc` / `Handle` registration in `internal/api/server.go`; channel inbound routes are registered dynamically by bootstrap / channel. There is **no** standalone OpenAPI file for the whole control plane.

## Auth

Optional control-plane tokens (YAML `control_plane.operator_token` / `admin_token`, or named `operators[]`):

- When the gate is on, it blocks Runtime `/v0` (empty gate = open for local development).
- **Operator:** runs, human approval, conversation identity, etc.
- **Admin:** agents, connectors, tools, channels, and most settings writes; catalog writes (e.g. `PATCH /v0/tools/{name}`) need admin or `403`.
- With named operators, `/ui` conversations are isolated by `owner_id` (admin sees all).

This is not downstream business IAM or multi-tenant SSO.

## Main resource groups

| Group | Example paths | Notes |
|-------|---------------|--------|
| Agents | `PUT/GET /v0/agents/{id}` | Agent definitions |
| Connectors | `PUT/GET/DELETE /v0/connectors/{id}`, `GET /v0/connectors` | OpenAPI / HTTP plugins / MCP, etc. |
| Connector tools | `POST /v0/connectors/{id}/tools`, `DELETE .../tools/{name}` | Add / remove `extra` REST tools |
| MCP OAuth | `POST .../mcp/oauth/start`, etc. | MCP connector OAuth |
| Tools | `GET /v0/tools`, `PATCH /v0/tools/{name}` | Tool catalog (incl. disabled rows) |
| Skills | `GET/POST /v0/skills`, `GET/DELETE /v0/skills/{id}` | Skill packs |
| Runs | `POST /v0/runs`, `GET /v0/runs/{id}`, `.../events`, `.../stream` (SSE), `.../resume`, `.../cancel`, `.../plugin-callbacks` | Execution and trail |
| Inbox | `POST /v0/inbox/{channel_id}` | Signed webhook inbox ingress |
| Channels inbound | `POST /v0/channels/{name}/inbound` | Declarative channel inbound (current example: WeChat adapter; channel types are extensible) |
| Conversations | list / delete, messages, identities, fork, rollback, etc. | Conversations and identity; list accepts `?workspace_id=` |
| Workspaces | `GET/POST /v0/workspaces` | Web workspaces; logins shared within a workspace |
| Artifacts / media | `GET /v0/artifacts/{id}`, `GET /v0/channels/media/...` | Artifacts and channel media |
| Settings | `/v0/settings/*` | webhook, inbox, store, channels, runtime, credentials, models, memory, mcp-export, tool-retrieval… |
| MCP export | `HANDLE /v0/mcp/export` (and trailing slash) | MCP server exporting a read-only tool-catalog subset to Agent clients such as Cursor |
| Meta | `GET /v0/me`, `GET /v0/ui-config` | Current identity and UI config |

Minimal `Run` state machine: `queued` → `running` → (optional `waiting_human` ↔ `running`) → `succeeded` | `failed` | `cancelled`.

## Matching & decisions (tool prefilter + System One)

Settings → Matching & decisions covers both enhanced matching and the decision model. These endpoints require **admin** (reads also need an authenticated role when the gate is on).

### Tool matching (enhanced / standard)

| Path | Role |
|------|------|
| `GET /v0/settings/tool-retrieval` | Mode / phase, whether Ollama is installed/running, whether the embed model is present, paths, download progress |
| `POST /v0/settings/tool-retrieval/enable` | Empty body or `{"provider":"local"}`: async local Ollama install + pull embed model; `{"provider":"api","base_url","model","api_key?"}`: cloud / self-hosted embeddings |
| `POST /v0/settings/tool-retrieval/disable` | Back to standard matching (does not delete local files) |
| `POST /v0/settings/tool-retrieval/cleanup` | Disable enhanced matching, remove embed model and installer cache; optional body `{"remove_ollama":true}` also uninstalls Ollama |
| `PUT /v0/settings/tool-retrieval/paths` | Body `{"models_dir":"..."}` sets a custom models dir (empty string clears); may restart local Ollama with `OLLAMA_MODELS` |

If the manager is not wired, `GET` still returns a standard-matching snapshot; writes may return `503 tool_retrieval_unavailable`. On CN Windows, installer download prefers the ModelScope sync.

### Decision model (System One)

| Path | Role |
|------|------|
| `GET /v0/settings/systemone` | Phase, local / API mode, model (default `tev1`), paths, download progress |
| `POST /v0/settings/systemone/enable` | Empty body or `{"provider":"local"}`: async local Ollama (≥0.35) install + pull `tev1`; `{"provider":"api","base_url","model?","api_key?"}`: probe then write `decide_systemone_*` |
| `POST /v0/settings/systemone/disable` | Clear decision-service knobs (does not delete local files) |
| `POST /v0/settings/systemone/cleanup` | Clean decision-related download cache; optional body `{"remove_ollama":true}` |

If unwired, writes may return `503 systemone_unavailable`. The **smart speed-up master switch** remains on `PATCH /v0/settings/runtime` (`decide_enabled` and sub-features).

## Model discovery and batch import

Model profiles can be created in bulk by fetching the catalog from an OpenAI-compatible endpoint. Both endpoints require **admin**:

| Path | Role |
|------|------|
| `POST /v0/settings/models/discover` | Query only, nothing persisted: GETs the upstream `/models` using `base_url` (+ optional `api_key` / `api_key_env` / `profile_id`) and returns the model list |
| `POST /v0/settings/models/batch` | Creates one profile per selected model, all sharing one `base_url` and credential; returns `created` and `skipped` (e.g. name collisions) detail |

When editing an existing profile you can omit `api_key` and pass `profile_id`; the server then reuses the stored key, and the plaintext key is never sent to the browser.

## Workspaces

| Path | Role |
|------|------|
| `GET /v0/workspaces` | List workspaces visible to the operator (includes default `default`) |
| `POST /v0/workspaces` | Create a workspace; later Web chats may send `workspace_id` |
| `GET /v0/conversations?workspace_id=` | List conversations in that workspace |
| `POST /v0/runs` field `workspace_id` | New chats land in that workspace (existing chats are not moved) |

Captured Web logins are shared per workspace. Channel and MCP-export sessions keep their own isolation keys.

Prefer the code registry and existing integration tests over assuming unlisted paths exist.
