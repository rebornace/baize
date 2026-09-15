<p align="center">
  <img src="docs/images/baize-banner.jpg" alt="Baize" width="100%">
</p>

[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)

# Baize

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**English** | [中文](README.zh-CN.md)

**An AI assistant runtime for your team: it can chat, call your business systems, and ask a person to confirm important writes — beside what you already run.**

In legend, Baize knows the names of all things; we use that name hoping the assistant can recognize and use the APIs, plugins, and workflows in your business world. Baize helps teams attach a capable assistant to existing services: understand the goal → call APIs or plugins → ask an operator to confirm in the console when needed → write results back to the conversation or a messaging channel. You usually **do not change business code**; an API document is enough for the assistant to take real actions. The web **`/ui`** is an operator console for product, ops, and integrators.

---

## Product advantages

- **API docs become capabilities**: upload or point to OpenAPI / Swagger / Postman-style docs; operations become tools the assistant can call, visible in the console.
- **Human confirmation for important writes**: creates and status changes can wait for approve / reject on a card before they run — with a clear trail.
- **Sidecar on, clean off**: one process, config outside the app; stop it and leave almost nothing behind.
- **One assistant, many entry points**: console, signed alert/ticket ingress, and IM channels share approvals, memory, and outbound delivery.
- **Downstream login supported**: the assistant can guide a login flow, then call systems with session credentials.
- **Long conversations stay usable**: automatic context compaction; multiple model setups and thinking options; account-level memory, attachments, and workspace files.

---

## What the assistant can do

### Chat and act

- Default flow: think → pick a tool → run it → report; skill packs can add step-by-step flows (with optional approval points).
- The console can **follow along** as reasoning and tool steps happen.
- Switch among multiple model setups; tune thinking-related options when the model supports them.
- **Skill packs**: instructions + tools; set defaults, or enable more skills for the current conversation.

### Where tools come from

- **API document → tools**: each operation joins the tool list for the assistant to call.
- **HTTP plugins (small companion services)**: when docs are incomplete or logic lives outside the spec, run a small service that tells Baize which tools exist and how to invoke them — ideal for legacy systems, internal scripts, or custom logic. You can also have Baize call back to your own URL with the tool name and arguments so your side executes them.
- **Connect external tool ecosystems (MCP)**: Baize can act as a client to MCP-capable tool servers (including common OAuth login flows).
- **Export tools to everyday Agent clients**: Baize can also act as an MCP server and expose a **read-only** slice of your tool catalog to clients such as Cursor or Claude Desktop — reuse the business capabilities you already wired in Baize inside the assistants your team already uses. Export does not run through Baize’s own chat model; it is for sharing the tool catalog.

### People in the loop and safety

- Sensitive writes can pause until an operator confirms in the console.
- Tools can be disabled anytime; full secrets stay out of conversation event replay.
- Trial can use a built-in mock model (no cloud API key); production uses a real model — paths stay separate.

### Memory, materials, long sessions

- **Memory**: store account-level facts the assistant can retrieve across chats.
- **Attachments and object storage**: keep materials locally or in object storage.
- **Context compaction**: long threads get rolling summaries so space goes to what still matters.
- Conversation fork / rollback for everyday ops (console and API).

### Messaging channels

Channels are a **general, extensible** capability: alerts, tickets, and instant messaging can share the same assistant, approvals, and outbound path.

- **Signed inbox**: external systems push alerts/tickets into the same assistant and approval flow.
- **Instant messaging**: the architecture supports IM via adapters. **The adapter shipped in this repository today is personal WeChat DM** (a small companion process plus an outbound queue with retry in settings). Other IMs can follow the same pattern — Baize is not limited to WeChat.
- Events can also be pushed to your own notification endpoints.

### For product, ops, and integrators

- Browser console in **Chinese / English**: conversations, tool cards, settings, and runtime knobs.
- Full HTTP management surface for scripts and platforms — no mandatory language SDK.
- Some runtime knobs hot-reload; a local database is enough to start, with larger databases when you need them.

---

## Use cases

**Assistant beside a legacy system**  
Service is live; you will not touch the code yet. Run Baize beside it, hand capabilities over via the API doc, trial “query / create / approve” internally, then decide on deeper changes.

**Writes that need a human click**  
The assistant can read, draft, and suggest; status changes and commands wait for confirmation in the console, with a trail.

**One assistant for tools and channels**  
External tools, first-party plugins, inbox ingress, and IM — one approval and memory story.

**Reuse wired tools inside everyday Agent clients**  
After APIs and plugins are connected in Baize, export them over MCP to Cursor and similar clients so the team works in familiar apps.

---

## Try it in 30 seconds

**Requirements:** Go 1.25+ (matches CI; no C compiler)

```powershell
# Windows
.\demo.cmd
```

```bash
# macOS / Linux
./scripts/demo.sh
# or: go run ./cmd/baize demo
```

The trial stack uses a mock model and bundled demo business HTTP — **no cloud API key**.

- Console: http://127.0.0.1:8080/ui  
- Demo business HTTP: http://127.0.0.1:18080  

Open the console, send “VPN is down, please file a record” — watch the assistant pick a tool, then **approve the write**. If ports are busy, stop the old Baize process or change the listen port and restart.

Production (real model, needs `BAIZE_API_KEY`): Windows `.\start.cmd`, macOS / Linux `./scripts/start.sh`.

---

## Deploy with prebuilt binaries (no Go required)

For production on a server or laptop: download the archive for your OS/arch from [GitHub Releases](https://github.com/rebornace/baize/releases) (`baize` plus optional `weixin-adapter`), unpack, then configure as below.

**1. Layout (example)**

```text
baize/
  baize                 # or baize.exe on Windows
  weixin-adapter        # only if you use the Weixin channel
  configs/minimal.yaml  # copy the sample from this repo
  .env
  data/
```

**2. Create `.env` (secrets stay out of YAML)**

```bash
BAIZE_API_KEY=sk-your-model-key
BAIZE_SETTINGS_KEY=a-long-random-string
# optional: BAIZE_LISTEN=:8080
```

**3. Start**

```bash
# from the unpack directory; prefers configs/minimal.local.yaml when present
./baize start
# or pin the config file:
./baize serve -config configs/minimal.yaml
```

Console: http://127.0.0.1:8080/ui  

Change listen port, model URL, DB path, etc. via YAML / env and **restart the process** — no rebuild. Connectors and accounts are mostly configured in the UI and stored under `data/`.

For systemd and standalone Weixin adapter setup, see [deployment](docs/developers/deployment.md). To cross-compile locally from this repo:

```bash
# Windows PowerShell
.\scripts\build-release.ps1 -Version v0.1.0
# macOS / Linux
./scripts/build-release.sh v0.1.0
```

---

## Developer docs

Build, test, configuration, deployment, and HTTP surface (**Chinese and English**):

- [Developer docs (English)](docs/developers/README.en.md) · [中文](docs/developers/README.md)

---

## License

[MIT](LICENSE)
