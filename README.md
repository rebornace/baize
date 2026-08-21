# Baize

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**English** | [中文](README.zh-CN.md)

**Add AI next to your existing HTTP APIs — no rewrite, nothing left behind.**

Baize is an Agent Runtime that sits beside your existing HTTP APIs: no changes to your business process, no embedded SDK. With OpenAPI, operations become tools automatically; without it, add a small HTTP plugin. Writes can require human approval. When you are done, stop the process and leave nothing behind.

> Sidecar, not chatbot: the main path is **OpenAPI → Tools → HTTP**. `/ui` is an operator console (conversation list, tool cards, HITL), not a consumer chat product. Session credentials are optional.

---

## Why Baize

| Problem | Baize approach |
|--------|----------------|
| Your service is already running — no AI, and you do not want to touch the code | Sidecar Runtime; a Connector turns APIs into Tools |
| No Swagger / OpenAPI | HTTP plugin sidecar |
| Uncomfortable letting the LLM hit write APIs directly | `require_approval` |
| Hard to unplug cleanly / platform lock-in | Single process, config outside the app — stop it and you are gone |

### Core concepts

| Concept | Role |
|---------|------|
| **Runtime** | Process hosting agents, tools, store, and HTTP control plane |
| **Agent** | Prompt + LLM that drives a Run |
| **Tool** | One invokable capability (usually one OpenAPI operation) |
| **Connector** | Binding of a spec + `base_url` (+ auth) into the tool registry |
| **Run** | One execution: input → ReAct loop → output / waiting for approval / failure |

---

## Try it in 30 seconds

**Requirements:** Go 1.22+ (no C compiler; SQLite is pure Go)

```bash
git clone https://github.com/rebornace/baize.git
cd baize
go run ./cmd/baize start
```

The repo also starts a **bundled mock HTTP demo service** so you can click the UI or hit curl — it is not the product itself.

Default sample:

- Runtime: `http://127.0.0.1:8080` (`/ui`)
- Demo HTTP: `http://127.0.0.1:18080`
- LLM: built-in `mock` (no API key)

If ports are busy, stop the previous `baize` process and retry.

### Operator UI (`/ui`)

Open `http://127.0.0.1:8080/ui`. If a control-plane token is configured, opening `/ui` unlocks first; operators can only access Identities, while changing Tools requires an admin token.

- Left: conversation list + **New chat**; **Settings** at the bottom-left (operators see “Identities”)
- Center: transcript; mutating tools show a **card** (name + status). Expand it for arguments / result
- `waiting_human`: **Approve / Reject** on that card (no footer banner)
- Settings → Tools (admin-only): tools fold by Connector / path prefix, searchable; editable display name and description (human edits survive a re-PUT of the spec); add tools in a drawer; `extra` rows can be deleted; Identities page is available to operators; MCP / plugins are “coming soon” empty states (no fake forms)
- Settings → Skills (admin-only): list installed packs, upload `.md` / `.zip`, delete user packs, and tick default Agent skills
- Live runs use SSE (`GET /v0/runs/{id}/stream`); if the stream drops, the UI falls back to 700ms polling

With the bundled mock LLM, send something like “VPN is down, please file a record” and approve `create_ticket` on the card.

### Call a demo write API (curl)

`POST /v0/runs` is async: it returns `run_id` immediately. In the demo, the tool name `create_ticket` requires approval by default (`waiting_human`).

```bash
curl -s -X POST http://127.0.0.1:8080/v0/runs \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"ticket-agent\",\"input\":\"VPN is down, please file a record\",\"conversation_id\":\"conv_example_1\"}"
```

```bash
curl -s http://127.0.0.1:8080/v0/runs/<run_id>
```

### HITL approval

This demo write API is marked for approval.

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
# Live trajectory (SSE). Ctrl+C to stop.
curl -N http://127.0.0.1:8080/v0/runs/<run_id>/stream
```

If `control_plane` has a token configured, the `/v0` requests above need:

`Authorization: Bearer <operator or admin token>`

Changing Connector / Tools requires the admin token. This is not the same key as the downstream login in the conversation.

---

## Point it at your APIs

### With OpenAPI

Register your HTTP service's OpenAPI as Tools.

1. Prepare an OpenAPI file and a Runtime-reachable `base_url`.
2. Register / replace a Connector:

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/ticket-api \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"openapi\",\"spec\":\"examples/mock-ticket/openapi.yaml\",\"base_url\":\"http://127.0.0.1:18080\",\"require_approval\":[\"create_ticket\"]}"
```

3. Inspect Tools:

```bash
curl -s http://127.0.0.1:8080/v0/tools
```

You should see `method`, `path`, `operation_id`, and `connector_id`.

4. Run via `POST /v0/runs` or open `/ui`.

Re-`PUT` with the same `id` **merges** that Connector’s Tools: existing `spec`/`plugin` rows keep their `enabled` and `require_login` state, disappeared operations are removed, new operations default to enabled, and `extra` rows survive unless explicitly deleted. Omit the catalog fields (`require_login` / `require_approval` / `tools`) to keep the on-disk catalog untouched. Invalid specs return `400` without corrupting the existing Registry or catalog.

### No OpenAPI: HTTP plugin

When your HTTP service has no usable OpenAPI spec, run a sidecar that implements
`GET /healthz`, `GET /v0/tools`, and `POST /v0/tools/{name}/invoke`
(`X-Baize-Protocol: v0`).

```bash
go run ./examples/http-plugin/cmd/http-plugin
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/legacy-sidecar \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"http\",\"base_url\":\"http://127.0.0.1:19090\",\"require_approval\":[\"create_ticket\"]}"
```

HITL still uses `require_approval`. Default `baize start` still uses the repo’s demo OpenAPI Connector.

### Tool catalog (enable / disable / add REST)

Each Connector owns a **tool catalog** persisted in the store (SQLite by default). `GET /v0/tools` returns catalog rows — including disabled ones — so the Settings page can list and re-enable them; the in-memory Registry only registers `enabled = true` rows, so a disabled tool is invisible to the model and to invoke.

- **Display name / description** — catalog rows may carry a `title` (Settings UI only; never sent in the model tool list) and a `description`. `PATCH /v0/tools/{name}` can update `title` and/or `description`. A human-edited `description` is marked `description_custom`; a later re-`PUT` of the Connector spec does not overwrite it. `title` is always kept across merge.
- **Enable / disable** — `PATCH /v0/tools/{name}` with `{"enabled": false}` (or `true`) unregisters (or re-registers) the tool immediately and persists the flag to the catalog. With SQLite, disabled rows and manually added rows survive a Runtime restart. The same `PATCH` can also flip `require_login` on the row. Group enable / disable on the Settings page is repeated per-tool `PATCH enabled` calls, not a separate API.
- **Settings tree / search** — the Tools page groups by Connector and path prefix (collapsible) and supports search; it does not introduce a new catalog HTTP surface.
- **Add a REST tool (OpenAPI only)** — `POST /v0/connectors/{id}/tools` adds an `extra` row on an `openapi` Connector, reusing that Connector’s `base_url`, auth, session identity, and HITL. Use it when the spec is missing an endpoint. Conflicting names return `409`.
- **Plugin tools** — sidecar-discovered (`plugin`) rows can be enabled / disabled but **cannot** be added or deleted; the sidecar is the source of truth. Adding an `extra` row on an `http` Connector returns `400`.
- **Delete** — `DELETE /v0/connectors/{id}/tools/{name}` only removes `source = extra` rows; `spec` / `plugin` rows return `400`.
- **Re-PUT / restart** — re-`PUT`-ing a Connector merges with the existing catalog (see above); a Runtime restart reloads the on-disk catalog and only re-registers enabled rows. YAML / PUT may omit catalog fields to keep the on-disk catalog untouched.

This catalog switch is **not** the same as:

- **Session login** — `require_login` on a tool row gates whether a Run with a `conversation_id` may call it without a captured identity; it does not store credentials.
- **Control-plane token** — `control_plane.operator_token` / `admin_token` gates who may call `/v0`. Catalog writes (`PATCH /v0/tools/{name}`, `POST/DELETE /v0/connectors/{id}/tools`) require the admin token; operators get `403`.

### Agent Skills (optional)

Skills are an optional **configuration** layer (not a sixth Runtime abstract): a `SKILL.md` with Markdown process guidance plus a list of tool names. They narrow (or stack) which enabled tools the model sees and inject flow text into the Run system prompt.

| Topic | Behavior |
|-------|----------|
| Disk layout | Builtin `./skills` (`skills.builtin_dir`) and user `./data/skills` (`skills.user_dir`); each subfolder is a pack id with `SKILL.md` |
| Install / remove | Admin upload `.md` or `.zip` via `POST /v0/skills` or Settings → Skills; `DELETE /v0/skills/{id}` removes **user** packs only (builtin → `400`). Same id: user overrides builtin |
| Default activation | `agent.skills` (YAML / `PUT /v0/agents/{id}`) lists packs active at Run start |
| Progressive activation | When any pack is installed, the model gets built-in `activate_skill` to expand the active set **for that Run** |
| Empty `agent.skills` | Visible tools = all catalog-**enabled** tools (same as before Skills) |
| Intersection | Visible tools = ∪(active Skill `tools`) ∩ catalog `enabled`; Skills cannot turn on a disabled catalog row |

The sample stack ships `skills/ticket-triage` and default YAML sets `agent.skills: [ticket-triage]` so mock-ticket create still works out of the box.

This is **not** Cursor’s personal coding Skill marketplace, and Baize does **not** guarantee drop-in compatibility with upstream packs such as `grill-me` / `superpowers` — only the familiar `SKILL.md` frontmatter + body shape is intentionally similar.

### Real LLM

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
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
    capture:
      tool_name_glob: "*login*"
      token_json_paths: ["accessToken", "data.accessToken", "data.token"]
      header_template: "Bearer {{token}}"
      default_scheme: "bearer"

mock_ticket:
  listen: "off"   # turn off demo HTTP when pointing at your service
```

```bash
go run ./cmd/baize start
# or: go run ./cmd/baize serve -config configs/default.local.yaml
```

---

## Approvals and identities

Mutating tools can require human approval via `require_approval` before the real invoke (see the demo flow under “Try it in 30 seconds” above).

### Session identities

Successful login tools can **capture** tokens into a per-`conversation_id` identity store. Later calls in the same conversation attach the resolved Bearer when a session identity is available (default selection follows OpenAPI `securitySchemes`).

Runs **with** a `conversation_id` (including `/ui`) use **only** session identities — never the connector’s configured default Token. Configured Tokens are optional and only apply to machine / curl Runs that omit `conversation_id`.

- `/ui` → **Settings → Identities**: list accounts (redacted), set default, sign out
- `/ui` → **Settings → Tools**: mark a tool as「需要登录」(default: public; not inferred from OpenAPI `security`)
- Sidebar **New chat** → new `conversation_id` (`localStorage` key `baize.conversation_id`)
- For Runs **without** `conversation_id`, default HTTP headers come from `connector.auth.mode`:
  - `static` — `${ENV}` expanded at registration (e.g. `Bearer ${BAIZE_CONNECTOR_TOKEN}`)
  - `passthrough` — per-Run allowlisted request headers from `POST /v0/runs`
  - `vault_ref` — `env:` / `file:` references resolved at registration
- Default capture matches `*login*` and reads `accessToken` / `data.token` (override via `connector.auth.capture`; set `tool_name_glob: "__none__"` to disable)
- `baize start` and `PUT /v0/connectors` both wire Identities / Resolver / Capture (OpenAPI)
- Restart Runtime after changing auth/capture YAML; `PATCH /v0/tools/{name}` persists `enabled` / `require_login` to the catalog (SQLite keeps them across restart) and immediately registers/unregisters the tool with the in-memory Registry. This is a different switch from the conversation login (session identity) and from the control-plane token.

```bash
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/identities
```

### Conversation memory

With the default SQLite driver, Baize persists conversation messages and captured identities to `data/baize.db` (configurable via `store.sqlite_path`). Restarting the Runtime restores prior turns and signed-in accounts for each `conversation_id`.

- `conversation.max_messages` (default `40`) bounds the window of recent turns fed back to the LLM as context. Older turns stay in the database for audit but are dropped from the prompt. Values `<=0` are normalized to `40` on load.
- Clearing chat (`DELETE /v0/conversations/{id}/messages`) wipes the message history **only** — it does **not** sign the user out. Captured identities remain until removed via the identities API. An empty conversation also **disappears from the left list**.
- `GET /v0/conversations` lists summaries; title is the first user message, truncated to 40 runes (no LLM titles).
- `data/baize.db` is a runtime artifact; do not commit it to git (the default `.gitignore` already excludes `data/`).

```bash
# Conversation list (left sidebar)
curl -s http://127.0.0.1:8080/v0/conversations

# List persisted turns
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages

# Clear turns without signing out
curl -s -X DELETE http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages
```

---

## Run and deploy

Baize is a single static binary. Optional launchers (set a China module proxy when `GOPROXY` is unset):

- POSIX: `./scripts/start.sh`
- Windows: `.\start.cmd`

### Native binary

```bash
# Linux server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o baize ./cmd/baize
# macOS
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o baize ./cmd/baize
# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o baize.exe ./cmd/baize
```

Copy the binary to the target host; no runtime dependencies beyond the OS.

### Docker

**Try the sample stack** (Runtime + demo HTTP, mock LLM):

```bash
docker compose up --build
```

- Runtime / UI: http://127.0.0.1:8080 (`/ui`)
- Demo HTTP: http://127.0.0.1:18080

Start **both** services. `configs/docker.yaml` points `base_url` at hostname `mock-ticket`; if you start only the Runtime, tools that call the demo HTTP will fail to connect.

Trial compose still sets `BAIZE_CONNECTOR_TOKEN` to `dev` for optional machine-path scripts; `/ui` does not use it. Do not put secrets in YAML.

**Production sidecar** (your HTTP service, no demo HTTP):

```bash
docker build -t baize:local .
docker run --rm -p 8080:8080 \
  -v /path/to/your.yaml:/app/configs/docker.yaml \
  -v baize-data:/app/data \
  -e BAIZE_API_KEY \
  baize:local
```

In `your.yaml` set `mock_ticket.listen: off` and point `connector.base_url` at your HTTP service. Do not use this repo's compose file as production orchestration.

Override module proxy at build time if needed: `docker build --build-arg GOPROXY=https://proxy.golang.org,direct .`

---

## Chat UI build (optional)

`/ui` is a React + Vite SPA, prebuilt under `internal/ui/dist` (`//go:embed`). Rebuild after editing `web/chat` (Node 18+):

```bash
cd web/chat
npm ci
npm run build
```

---

## Commands

| Command | Description |
|---------|-------------|
| `baize start` | Prefer `default.local.yaml` if present, else `default.yaml`; start Runtime (also starts the demo HTTP service unless `mock_ticket.listen: off`) |
| `baize serve -config <path>` | Runtime only, explicit config |

---

## Documentation

- [Architecture & plugin protocol (draft)](docs/architecture-and-plugin-protocol.md)

---

## License

[MIT](LICENSE)
