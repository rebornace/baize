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
| Conversations | list / delete, messages, identities, fork, rollback, etc. | Conversations and identity |
| Artifacts / media | `GET /v0/artifacts/{id}`, `GET /v0/channels/media/...` | Artifacts and channel media |
| Settings | `/v0/settings/*` | webhook, inbox, store, channels, runtime, credentials, models, memory, mcp-export… |
| MCP export | `HANDLE /v0/mcp/export` (and trailing slash) | MCP server exporting a read-only tool-catalog subset to Agent clients such as Cursor |
| Meta | `GET /v0/me`, `GET /v0/ui-config` | Current identity and UI config |

Minimal `Run` state machine: `queued` → `running` → (optional `waiting_human` ↔ `running`) → `succeeded` | `failed` | `cancelled`.

Prefer the code registry and existing integration tests over assuming unlisted paths exist.
