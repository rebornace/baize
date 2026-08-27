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

Windows (from repo root — no global `baize` on PATH):

```powershell
.\demo.cmd          # trial: mock LLM + demo HTTP, no key
.\start.cmd         # production: needs BAIZE_API_KEY (see below)
.\baize.cmd demo    # same as demo.cmd
.\baize.cmd start   # same as start.cmd
```

POSIX:

```bash
./scripts/demo.sh
go run ./cmd/baize demo
```

`baize demo` / `demo.cmd` uses `configs/demo.yaml`: mock LLM + bundled demo HTTP — **no API key**. Not the production default.

> Until you `go install` or add a binary to PATH, do not run bare `baize start`; use `go run ./cmd/baize start`, `.\start.cmd`, or `.\baize.cmd start`.

Default sample:

- Runtime: `http://127.0.0.1:8080` (`/ui`)
- Demo HTTP: `http://127.0.0.1:18080`
- LLM: built-in `mock`

If ports are busy, stop the previous `baize` process and retry.

### Operator UI (`/ui`)

Open `http://127.0.0.1:8080/ui`. If a control-plane token is configured, opening `/ui` unlocks first; operators can only access Identities, while changing Tools requires an admin token.

- Left: conversation list + **New chat**; **Settings** at the bottom-left (operators see “Identities”)
- Center: transcript; mutating tools show a **card** (name + status). Expand it for arguments / result
- `waiting_human`: **Approve / Reject** on that card (no footer banner)
- Settings → OpenAPI (admin-only): upload API documents (OpenAPI 3, Swagger 2, Postman v2.1) to register Connectors; admins can delete an entire Connector from OpenAPI, Plugins, or MCP settings (with confirmation); Settings → Tools (admin-only): tools fold by Connector / path prefix, searchable; editable display name and description (human edits survive a re-PUT of the spec); add tools in a drawer; `extra` rows can be deleted; configure execution callback URL per OpenAPI / HTTP Connector; configure login capture (`auth.capture`) for OpenAPI / HTTP plugin Connectors; Identities page is available to operators; Settings → MCP (admin-only) registers MCP Servers; Settings → Plugins (admin-only) registers HTTP plugin sidecars
- Settings → Skills (admin-only): list installed packs, upload `.md` / `.zip`, delete user packs, and tick default Agent skills
- Settings → Webhook (admin-only): configure global run-event webhook URL and headers; send a test delivery
- Chat **Advanced** (collapsible): optional per-run `webhook_url` override (empty uses global settings)
- Live runs use SSE (`GET /v0/runs/{id}/stream`); outbound webhooks POST each event and a terminal `run.ended` payload; if the stream drops, the UI falls back to 700ms polling

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

Default `configs/demo.yaml` uses SQLite. After a Runtime restart, `waiting_human` runs can still be resumed.

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

Register your HTTP service as Tools by uploading an API document.

1. Open `/ui` → **Settings → OpenAPI** (admin).
2. **Add Connector**: set `id`, `base_url`, upload your API document (`.json`, `.yaml`, `.yml`), or paste a document URL (direct `.json`/`yaml` link or Swagger UI page; Runtime fetches server-side).
3. Save — the server detects the format, converts to OpenAPI 3, and discovers all operations.
4. Manage tools under **Settings → Tools**; run via `/ui` or `POST /v0/runs`.

| Format | Extensions | Notes |
|--------|------------|-------|
| OpenAPI 3 | `.json`, `.yaml`, `.yml` | OpenAPI 3.0 / 3.1 |
| Swagger 2 | `.json`, `.yaml`, `.yml` | Converted to OpenAPI 3 |
| Postman Collection v2.1 | `.json` | Converted to OpenAPI 3 |

**Not supported (v0):** PDF, Word, WSDL/SOAP.

If the document’s host is wrong, the form `base_url` wins.

Re-`PUT` with the same `id` **merges** that Connector’s Tools: existing `spec`/`plugin` rows keep their `enabled` and `require_login` state, disappeared operations are removed, new operations default to enabled, and `extra` rows survive unless explicitly deleted. Omit the catalog fields (`require_login` / `require_approval` / `tools`) to keep the on-disk catalog untouched. Invalid specs return `400` without corrupting the existing Registry or catalog.

#### Advanced: curl / YAML bootstrap

For automation or bootstrap defaults, register via `PUT /v0/connectors/{id}` with a server-side `spec` path or `spec_content`:

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/ticket-api \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"openapi\",\"spec\":\"examples/mock-ticket/openapi.yaml\",\"base_url\":\"http://127.0.0.1:18080\",\"require_approval\":[\"create_ticket\"]}"
```

Inspect Tools:

```bash
curl -s http://127.0.0.1:8080/v0/tools
```

You should see `method`, `path`, `operation_id`, and `connector_id`.

### No OpenAPI: HTTP plugin

When your HTTP service has no usable OpenAPI spec, run a sidecar that implements
`GET /healthz`, `GET /v0/tools`, and `POST /v0/tools/{name}/invoke`
(`X-Baize-Protocol: v0`).

```bash
go run ./examples/http-plugin/cmd/http-plugin
```

Register with **Settings → Plugins** (admin) or `PUT /v0/connectors/{id}` (`type: http`):

HITL still uses `require_approval`. Use `baize demo` for the repo’s demo OpenAPI Connector.

Set `runtime.public_base_url` (Runtime root URL reachable by the sidecar) to inject a short-lived signed `callback_urls.event` on HTTP plugin invoke; the sidecar may POST notes/progress into the Run event stream (`plugin.callback`). MCP `tools/call` and enterprise execution callbacks receive the same `callback_urls.event` at invoke time for async Run event posts. If unset, nothing is injected. See architecture doc §4.2.

### Enterprise execution callback (§4.3)

When the legacy system exposes a single execution endpoint instead of per-operation HTTP, set `execution_callback_url` on the Connector. Tool discovery still comes from OpenAPI or the HTTP sidecar; invoke POSTs to your URL with `tool`, `arguments`, `run_id`, and `idempotency_key`. Edit per Connector under **Settings → Tools**, or via `PUT /v0/connectors/{id}` / YAML `connector.execution_callback_url`.

Reference server:

```bash
go run ./examples/enterprise-callback
```

### MCP connectors (optional)

[MCP](https://modelcontextprotocol.io/) Servers expose tools over **stdio** (local subprocess) or **Streamable HTTP** (remote URL). Register one with `PUT /v0/connectors/{id}` (`type: mcp`) or **Settings → MCP** (admin). Discovered tools enter the catalog with `source: mcp`; enable/disable, HITL `require_approval`, and Run invoke behave like OpenAPI / HTTP plugin tools. The `auth` block on the Connector is **ignored** — pass secrets via `mcp.env` (stdio) or `mcp.headers` (HTTP). Production `baize start` does **not** pre-register any MCP Server.

**stdio — local subprocess** (needs Node.js on the **same host as Baize** for `npx` commands):

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/analytics-db \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"stdio\",\"command\":\"npx\",\"args\":[\"-y\",\"@bytebase/dbhub\",\"--transport\",\"stdio\",\"--dsn\",\"postgres://baize:baize@127.0.0.1:5432/demo?sslmode=disable\"]},\"require_approval\":[\"execute_sql\"]}"
```

**HTTP — Streamable HTTP endpoint** (remote MCP or a Server you run on the host while Baize is in Docker):

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/tavily \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"http\",\"url\":\"https://mcp.tavily.com/mcp/?tavilyApiKey=YOUR_KEY\"}}"
```

Bad config or unreachable Server → `400 invalid_mcp` (Registry unchanged). Global tool name conflicts → `409 tool_conflict`.

#### MCP + Postgres trial (`docker-compose.mcp-demo.yml`)

Postgres demo DB + Baize Runtime only — **no** Node/DBHub container and **no** pre-registered MCP connector:

```bash
export BAIZE_API_KEY=sk-...
docker compose -f docker-compose.mcp-demo.yml up --build
```

- Postgres: `postgres://baize:baize@127.0.0.1:5432/demo` (sample `tickets` table from `examples/mcp-demo/init.sql`)
- Runtime / UI: http://127.0.0.1:8080 (`/ui`)

Because the Baize image does not include Node, run [DBHub](https://github.com/bytebase/dbhub) on your **host** (Node 18+), then register MCP:

```bash
# Host terminal — stdio DBHub against compose Postgres (Baize must also run on the host)
npx -y @bytebase/dbhub --transport stdio --dsn "postgres://baize:baize@127.0.0.1:5432/demo?sslmode=disable"
```

Use the stdio `PUT` snippet above (`command` / `args` instead of a one-off shell). When Baize runs **inside** Docker, start DBHub on the host with HTTP transport and register `transport: http` — e.g. `url: http://host.docker.internal:9090/mcp` (port/path per DBHub docs).

Prefer a read-only DSN in production; the compose credentials are for local trials only.

#### Search / web MCP (documentation only)

Baize does not proxy the public internet — the MCP Server calls search APIs. There is **no** search compose stack; add connectors yourself after `baize start` or via Settings → MCP.

**Tavily (HTTP)** — remote Streamable HTTP, your API key in the URL:

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/tavily \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"http\",\"url\":\"https://mcp.tavily.com/mcp/?tavilyApiKey=YOUR_KEY\"}}"
```

**Brave Search (stdio)** — host `npx`, your `BRAVE_API_KEY`:

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/brave-search \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"stdio\",\"command\":\"npx\",\"args\":[\"-y\",\"@modelcontextprotocol/server-brave-search\"],\"env\":{\"BRAVE_API_KEY\":\"env:BRAVE_API_KEY\"}}}"
```

Tool names come from each Server; they must be globally unique across all Connectors.

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

**Chat composer — Skill symbols (`@` / `/`):** In `/ui`, type `@skill-id` or `/skill-id` (equivalent) to activate packs for **that Run only**. Multiple mentions are deduplicated in order and merged with the optional `skills` field on `POST /v0/runs`; together they **override** the Agent default list (not append). Unknown ids → `400 unknown_skill`. Symbols are stripped from the stored user text. `@` / `/` also open an autocomplete list (`GET /v0/skills` is readable by operators).

**Chat attachments:** Use the paperclip in `/ui` or send `attachments[]` on `POST /v0/runs` (base64 JSON). Supported types:

| Extension | Handling |
|-----------|----------|
| `.txt` `.md` `.csv` | UTF-8 text injected into the user context |
| `.docx` `.xlsx` `.pdf` | Text extracted (PDF text layer only; no page rendering) |
| `.png` `.jpg` `.jpeg` `.webp` `.gif` | Multimodal image parts when vision is enabled |

Limits: up to 5 files, 8 MiB total decoded, 64 KiB extracted text per file (truncated with `…[truncated]`). Other extensions → `400 unsupported_attachment`. PDF with no extractable text → `400 empty_pdf_text`.

**Vision hard failure:** `llm.supports_vision` defaults to `false` (see sample YAML comments). If a Run includes image attachments while `supports_vision=false`, the server returns **`400 vision_unsupported`** and **does not create a Run** — images are never silently downgraded to filenames only. Set `supports_vision: true` when using a vision-capable model. `/ui` reads `GET /v0/ui-config` and blocks image sends locally; the server remains authoritative.

Built-in Skills use two tiers: `skills/` for core packs (`minimal` scans only this; default `data-analytics`); `examples/skills/` for demo trials (`demo` / `docker-demo` add it via `builtin_dirs`). Clean deployments list only core packs under Settings → Skills.

This is **not** Cursor’s personal coding Skill marketplace, and Baize does **not** guarantee drop-in compatibility with upstream packs such as `grill-me` / `superpowers` — only the familiar `SKILL.md` frontmatter + body shape is intentionally similar.

### Data analytics & reports

For multi-source statistics and interactive dashboards, Baize ships a built-in tool **`create_analysis_page`** (always registered; no HITL). Built-in Skill **`data-analytics`** (`skills/data-analytics`, default in `minimal.yaml`): toggle in Settings → Skills or via `activate_skill`. Works with only `create_analysis_page` before any Connector; `list_tickets` / `get_ticket` appear once those tools are registered.

**Recommended:** `create_analysis_page` → a self-contained analysis page with **filters**, chart **drilldown**, and in-browser **PDF export**. The tool returns `{ artifact_id, artifact_url, kind: "analysis_page" }`; `/ui` embeds the page in an iframe.

**Token strategy:**

| Approach | When to use |
|----------|-------------|
| `format: "sections"` + `binding` | **Default** — aggregate Connector JSON into `datasets` in the model, bind charts / KPIs / tables; smallest token footprint |
| `echarts.option` in a section | Complex charts that `binding` cannot express |
| `format: "html"` | Full layout freedom; larger payload; Runtime wraps and validates the HTML |

Pull JSON from your Connector tools and reshape into `datasets` in the model — do not dump raw large JSON into a single section.

**Optional — AntV MCP for static PNG:**

Baize does **not** pre-register AntV. Admins may add MCP Server `@antv/mcp-server-chart` (stdio `npx`) under **Settings → MCP** for single-chart PNG URLs — useful for Office / slides, not a full analysis site. MCP results still appear as JSON tool cards in chat (no dedicated image embed).

| | `create_analysis_page` | AntV MCP |
|--|------------------------|----------|
| Output | **Full analysis page** (multi-block + filters + drilldown) | Usually **one chart** + image URL |
| Narrative | markdown / KPI / mixed tables & charts | None |
| Hosting | Baize artifact + chat iframe | External image URL |
| Flexibility | sections + full ECharts option; or whole-page HTML | Fixed `generate_*` chart types |

### Production one-shot (native Go)

`baize start` uses `configs/minimal.yaml`: **no demo Connector, no mock-ticket, real LLM**. Set an API key first:

```bash
cp .env.example .env   # set BAIZE_API_KEY
export BAIZE_API_KEY=sk-...   # or source .env
go run ./cmd/baize start
```

Optional: copy `configs/minimal.yaml` → `configs/minimal.local.yaml` (gitignored) to override `llm.base_url` / `model`.

The tool catalog is empty until you register a Connector (`PUT /v0/connectors/{id}`) or an MCP Server (`type: mcp`). SQLite still stores runs and conversations.

### Real LLM and your APIs

Do **not** put secrets in YAML.

1. Production default `configs/minimal.yaml` already uses `openai_compatible`; `baize start` fails fast if `BAIZE_API_KEY` is missing.
2. Copy `.env.example` → `.env` and set `BAIZE_API_KEY` (and optional control-plane / connector tokens).
3. Add a `connector` block in `configs/minimal.local.yaml`, or register after startup:

```yaml
connector:
  id: my-api
  type: openapi
  spec: path/to/your/openapi.yaml
  base_url: https://your-api.example.com
  require_approval_mutating: true
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
    capture:
      tool_name_glob: "*login*"
      token_json_paths: ["accessToken", "data.accessToken", "data.token"]
      header_template: "Bearer {{token}"
      default_scheme: "bearer"
```

```bash
go run ./cmd/baize start
# or: go run ./cmd/baize serve -config configs/minimal.local.yaml
```

For the trial stack, use `go run ./cmd/baize demo` (mock LLM, no key).

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
- Default capture matches `*login*` and reads `accessToken` / `data.token` (override via `connector.auth.capture`; set `tool_name_glob: "__none__"` to disable). Plugin tools whose names match `*login*` are captured the same way.
- `baize start` and `PUT /v0/connectors` both wire Identities / Resolver / Capture (OpenAPI and HTTP plugin Connectors)
- Restart Runtime after changing auth/capture YAML; `PATCH /v0/tools/{name}` persists `enabled` / `require_login` to the catalog (SQLite keeps them across restart) and immediately registers/unregisters the tool with the in-memory Registry. This is a different switch from the conversation login (session identity) and from the control-plane token.

```bash
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/identities
```

### Conversation memory

With the default SQLite driver, Baize persists conversation messages and captured identities to `data/baize.db` (configurable via `store.sqlite_path`). Restarting the Runtime restores prior turns and signed-in accounts for each `conversation_id`.

- `conversation.max_messages` (default `40`) bounds the window of recent turns fed back to the LLM as context. Older turns stay in the database for audit but are dropped from the prompt. Values `<=0` are normalized to `40` on load.
- Clearing chat (`DELETE /v0/conversations/{id}/messages`) wipes the message history **only** — it does **not** sign the user out. Captured identities remain until removed via the identities API. An empty conversation also **disappears from the left list**.
- **Rollback / Fork (`/ui`)**: user bubbles — edit & rollback (truncate from that message); assistant — regenerate; any message — fork prefix into a new conversation (identities not copied). Blocked while a run is active.
- `GET /v0/conversations` lists summaries; title is the first user message, truncated to 40 runes (no LLM titles).
- `data/baize.db` is a runtime artifact; do not commit it to git (the default `.gitignore` already excludes `data/`).

```bash
# Conversation list (left sidebar)
curl -s http://127.0.0.1:8080/v0/conversations

# List persisted turns
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages

# Clear turns without signing out
curl -s -X DELETE http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages

# Rollback from a message (deletes that message and everything after)
curl -s -X POST http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages/<message_id>/rollback

# Fork: copy prefix through a message into a new conversation
curl -s -X POST http://127.0.0.1:8080/v0/conversations/<conversation_id>/fork \
  -H 'Content-Type: application/json' \
  -d '{"through_message_id":"<message_id>"}'
```

---

## Run and deploy

Baize is a single static binary. Optional launchers (set a China module proxy when `GOPROXY` is unset):

- Production Windows: `.\start.cmd` or `.\baize.cmd start` (loads `BAIZE_API_KEY` from `.env`)
- Trial Windows: `.\demo.cmd` or `.\baize.cmd demo`
- POSIX production: `./scripts/start.sh`; trial: `./scripts/demo.sh`

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

**Production** (Runtime only, no demo HTTP):

```bash
export BAIZE_API_KEY=sk-...
docker compose up --build
```

- Runtime / UI: http://127.0.0.1:8080 (`/ui`)
- Uses `configs/docker-minimal.yaml`; requires `BAIZE_API_KEY`

**Try the sample stack** (Runtime + demo HTTP, mock LLM):

```bash
docker compose -f docker-compose.demo.yml up --build
```

- Runtime / UI: http://127.0.0.1:8080
- Demo HTTP: http://127.0.0.1:18080
- Uses `configs/docker-demo.yaml`; `mock-ticket` runs as a separate container

Trial compose may set `BAIZE_CONNECTOR_TOKEN` to `dev` for machine-path scripts; `/ui` does not use it. Do not put secrets in YAML.

**MCP + Postgres trial** (Runtime + demo DB, no bundled MCP Server):

```bash
export BAIZE_API_KEY=sk-...
docker compose -f docker-compose.mcp-demo.yml up --build
```

See [MCP connectors](#mcp-connectors-optional) for DBHub on the host and `PUT` examples.

**Custom YAML mount**:

```bash
docker build -t baize:local .
docker run --rm -p 8080:8080 \
  -v /path/to/your.yaml:/app/configs/docker-minimal.yaml \
  -v baize-data:/app/data \
  -e BAIZE_API_KEY \
  baize:local
```

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
| `baize start` | Production default: `minimal.yaml`; Runtime only; **requires** `BAIZE_API_KEY`; no demo Connector |
| `baize demo` | Trial: `demo.yaml`; Runtime + bundled demo HTTP; mock LLM, no key |
| `baize serve -config <path>` | Runtime only with explicit config (no API key check) |

---

## Documentation

- [Architecture & plugin protocol (draft)](docs/architecture-and-plugin-protocol.md)

---

## License

[MIT](LICENSE)
