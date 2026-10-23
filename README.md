<p align="center">
  <img src="docs/images/baize-banner.jpg" alt="Baize" width="100%">
</p>

[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)

# Baize

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**English** | [中文](README.zh-CN.md)

**Website:** [rebornace.github.io/baize](https://rebornace.github.io/baize/)

[![baize - Lightweight AI Agent Framework | Product Hunt](https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1255168&theme=light&t=1789790934744)](https://www.producthunt.com/products/baize?embed=true&utm_source=badge-featured&utm_medium=badge&utm_campaign=badge-baize)

**An AI assistant for your team: it can chat, call the business systems you already run, and ask a person to confirm important changes — sit it beside your stack and go.**

In legend, Baize knows the names of all things. We hope the assistant can recognize and use the APIs and workflows in *your* world: understand the goal → call your systems → ask a teammate to confirm in the web console when needed → write results back to the chat or a messaging channel. You usually **do not change business code**; an API document is enough for the assistant to take real actions. The web console is for product, ops, and integrators.

---

## Why Baize

- **API docs become abilities**: upload or point to common API docs (OpenAPI / Swagger, etc.); the assistant can call those operations, and you can watch the steps in the console.
- **Approve important writes**: creates and status changes can wait for approve / reject on a card — with a clear trail.
- **Sit beside your stack, leave cleanly**: one service, config outside the app; stop it and leave almost nothing behind.
- **One assistant, many doors**: web console, alert/ticket inbox, and messaging channels share approvals, memory, and outbound replies.
- **Downstream login supported**: use “browser login” in chat, or configure a login page in Settings. Passwords never go through the model; then the assistant calls systems with the session.
- **Reuse wired abilities in everyday AI tools**: expose what you already connected to Cursor, Claude Desktop, Doubao, and similar clients (read-only). When login expires, open the returned link — never paste passwords into the chat.
- **Stays usable when you have many systems**: Settings → Matching & decisions and Smart speed-up help narrow what this turn needs before the main model runs — cheaper and faster (see below and the developer docs).
- **Group by business domain**: e.g. finance vs commerce. For stronger isolation, start a separate sub-task so approvals do not collide.
- **See API drift; replay what a turn used**: after re-import, compare what changed; beside each chat turn, open what model and abilities were bound.
- **Ops in the console**: runtime metrics and recent logs in Settings; common metrics scrape and optional tracing; tweak observability without editing config files. Outbound calls can use local certificates for encrypted connections.
- **Long chats stay usable**: automatic summaries; optional “keep important facts”; very large tool results or pastes can become workspace files with a short preview in chat; account-level memory and attachments.
- **Workspaces and accounts**: switch workspaces in the sidebar; logins are shared inside a workspace and isolated across workspaces. Identities for “expose to other AI tools” stay separate from chat accounts.
- **Can explain Baize itself**: a built-in help skill (English / Chinese) points you to the right Settings page — it does not edit config or restart the service.

---

## What the assistant can do

### Chat and act

- Default flow: think → pick an ability → run it → report; skill packs can add step-by-step flows (with optional approval points).
- The console can **follow along** as reasoning and calls happen.
- Switch among model setups; tune thinking options when the model supports them.
- When adding models, **fetch the server’s model list and import in batch**, or enter one model by hand.
- **Skill packs**: instructions + abilities. Checking skills in Settings only sets the default start for **new** chats; unchecked skills stay reachable with `@` or `/` in chat. Prefer `@` for login skills — do not pin them as defaults.
- **Workspaces**: switch or create in the sidebar; new chats land in the current workspace (moving old chats between workspaces is not supported yet).
- Thinking collapses to one line by default; usage appears on the reply when the turn finishes.

### Where abilities come from

- **API document → abilities**: each operation joins the list for the assistant to call.
- **Small companion services**: when docs are incomplete or logic lives outside the spec, run a small service that tells Baize what exists and how to run it — or have Baize call back to your URL so you execute on your side.
- **External tool ecosystems**: connect MCP-capable tool servers (including common sign-in flows).
- **Export to everyday AI clients**: use wired business abilities inside Cursor, Claude, Doubao, and similar tools without re-wiring. On auth failure, open the login link in a browser.

### People in the loop and safety

- Sensitive writes can pause until someone confirms in the console.
- When login is required: open the login page, skip that system for this turn, or end the turn.
- Abilities can be disabled anytime; full secrets stay out of conversation replay.
- Trial can use a built-in mock model (no cloud key yet); production uses a real model — paths stay separate.

### Memory, materials, long sessions

- **Memory**: store account-level facts the assistant can use across chats.
- **Schedules**: after approval, save a one-shot, daily, or weekly instruction; at that time Baize starts a new turn in the same chat. Writes still wait for approval. Manage your own schedules in Settings.
- **Attachments and file storage**: local or object storage.
- **Long chats**: automatic summaries; oversized content can become workspace files; optional “keep important facts.”
- Fork / roll back conversations for everyday ops.
- Web logins are shared **per workspace**; messaging channels and “expose abilities” identities stay separate.

### Messaging channels

Alerts, tickets, and instant messaging can share the **same** assistant, approvals, and outbound path.

- **Inbox**: external systems push alerts/tickets into the same flow.
- **Instant messaging**: this repo ships **personal WeChat DM** as the production adapter; Feishu and DingTalk **examples** are included — other IMs can follow the same pattern.
- Events can also go to your own notification URL.

### For product, ops, and integrators

- Browser console in **Chinese / English**: chats, call cards, settings, and runtime options.
- Matching & decisions, external login, expose abilities, runtime metrics, and eval corpus are all discoverable in Settings.
- Re-import shows what changed; chat offers “what this turn used”; exports to other AI tools are auditable.
- Full HTTP management surface for scripts and platforms — no mandatory language SDK.
- Some options hot-reload; a local database is enough to start, with larger databases when you need them.

---

## Use cases

**Assistant beside a legacy system**  
The service is live; you will not touch the code yet. Run Baize beside it, hand abilities over via the API doc, trial “query / create / approve” internally, then decide on deeper changes.

**Writes that need a human click**  
The assistant can read, draft, and suggest; status changes and commands wait for confirmation in the console, with a trail.

**One assistant for tools and channels**  
External tools, first-party plugins, inbox, and IM — one approval and memory story instead of a fleet of disconnected bots.

**Several business domains to keep apart**  
Group finance, commerce, and so on; start a separate sub-task only when you need real isolation — not a deep “sub-agent tree.”

**Reuse wired abilities inside Cursor and similar tools**  
Connect APIs and plugins in Baize, set up the login page, then export. When auth expires, open the link — never paste passwords into the agent chat. Make sure the login page is reachable from the client’s browser.

**APIs drift, runs need a trail, networks need encryption**  
Compare changes after re-import (optional notify); see what a turn bound; use certificates outbound and metrics/logs in the console — without bolting on another assistant stack.

---

## When you have many systems: cheaper and faster

If every turn dumps every ability into the main model, chats get slow and expensive. Baize can run a **light first pass**: which systems and abilities this turn likely needs, then send a shorter list to the main model. If that fails open, the full list is used so the turn still works.

In the console: **Settings → Matching & decisions** (optional stronger matching and a small local/cloud model to help judge) → **Runtime → Smart speed-up** (master switch and the sub-features you need). Configuring a judge model alone does not cut main-model usage until those switches are on.

Install steps, knobs, and fail-open behavior: [Architecture](docs/developers/architecture.en.md) and [Configuration](docs/developers/configuration.en.md).

### Benchmark (reproducible)

**37** real read-only business requests across **3** backends (**390** abilities), measuring main-model first-turn input size; stability over **5** rounds (**185** requests). Scripts: [`scripts/tool-routing-eval`](scripts/tool-routing-eval/README.md).

| Cap on abilities sent | Success | Avg. abilities sent | Avg. first-turn input | vs. cap 32 |
|---|---|---|---|---|
| 32 | 36/37 | 43.1 | 4,703 | — |
| 24 | 37/37 | 37.4 | 3,980 | −15% |
| **16 (default)** | **37/37** | 30.6 | **3,090** | **−34%** |
| 12 | 37/37 | 27.1 | 3,033 | −36% |
| 8 | 35/37 | 23.6 | 2,600 | −45% |

Sending all **390** abilities measured about **85k** first-turn input tokens; at cap 16, about **1.4k–3.8k**. Over 185 requests, success was about **99.5% (184/185)**. Using a small local model for judgments is often faster end-to-end than asking another large model to judge (about 1.5× on the same machine). Full field notes live in the developer docs and the script folder.

---

## Quick start

**Requirements:** Go 1.25+, Node.js 20+ (builds the web console), and an OpenAI-compatible API key.

```bash
cp .env.example .env        # then set BAIZE_API_KEY=sk-...
```

```powershell
.\serve.cmd                 # Windows
```

```bash
./scripts/serve.sh          # macOS / Linux
```

- Console: http://127.0.0.1:8080/ui
- Skip UI rebuild: `.\serve.cmd -SkipUI` / `BAIZE_SKIP_UI=1`
- Force UI rebuild: `.\serve.cmd -ForceUI` / `BAIZE_FORCE_UI=1`

Open the console and send a message. To change the model or wire a business system, copy `configs/config.yaml` to `configs/config.local.yaml` (git-ignored), edit it, and start with `-config`.

---

## Deploy with prebuilt binaries (no Go required)

Download the archive for your OS from [GitHub Releases](https://github.com/rebornace/baize/releases), unpack, then configure.

**1. Layout (example)**

```text
baize/
  baize                 # or baize.exe on Windows
  weixin-adapter        # only if you use the WeChat channel
  configs/config.yaml
  .env
  data/
```

**2. Create `.env` (keep secrets out of YAML)**

```bash
BAIZE_API_KEY=sk-your-model-key
BAIZE_SETTINGS_KEY=a-long-random-string
# optional: BAIZE_LISTEN=:8080
```

**3. Start**

```bash
./baize serve
# or: ./baize serve -config configs/config.local.yaml
```

Console: http://127.0.0.1:8080/ui  

Change listen port, model URL, etc., then **restart the process**. Most business wiring is done in the console. For systemd and WeChat adapter setup, see [deployment](docs/developers/deployment.en.md).

---

## Developer docs

Build, test, configuration, deployment, and HTTP surface (**Chinese and English** — these pages keep precise system names):

- [Developer docs (English)](docs/developers/README.en.md) · [中文](docs/developers/README.md)
- [Mobile](docs/developers/mobile.en.md): phone browser/shell to a remote assistant, plus on-device trimmed orchestration · [中文](docs/developers/mobile.md)

---

## License

[MIT](LICENSE)
