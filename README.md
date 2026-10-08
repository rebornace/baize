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

**An AI assistant runtime for your team: it can chat, call your business systems, and ask a person to confirm important writes — beside what you already run.**

In legend, Baize knows the names of all things; we use that name hoping the assistant can recognize and use the APIs, plugins, and workflows in your business world. Baize helps teams attach a capable assistant to existing services: understand the goal → call APIs or plugins → ask an operator to confirm in the console when needed → write results back to the conversation or a messaging channel. You usually **do not change business code**; an API document is enough for the assistant to take real actions. The web **`/ui`** is an operator console for product, ops, and integrators.

---

## Product advantages

- **API docs become capabilities**: upload or point to OpenAPI / Swagger / Postman-style docs; operations become tools the assistant can call, visible in the console.
- **Human confirmation for important writes**: creates and status changes can wait for approve / reject on a card before they run — with a clear trail.
- **Sidecar on, clean off**: one process, config outside the app; stop it and leave almost nothing behind.
- **One assistant, many entry points**: console, signed alert/ticket ingress, and IM channels share approvals, memory, and outbound delivery.
- **Downstream login supported**: the assistant can guide a login flow, then call systems with session credentials.
- **Long conversations stay usable**: automatic context compaction; optional “keep important facts in long chats” (a model-facing projection, not a rewrite of saved messages); multiple model setups and thinking options; account-level memory, attachments, and workspace files.
- **Workspaces and logins**: switch workspaces in the console; business logins are shared among chats in the same workspace and isolated across workspaces.
- **Can explain Baize itself**: the built-in `baize-help` skill answers (in English or Chinese) how Baize is set up and where to click in the console; it does not edit `config.yaml` or restart the process.
- **Stays usable with many tools**: when backends pile up, turn on enhanced matching under Settings → Tool matching — one-click local Ollama + embedding model, or a cloud / self-hosted OpenAI-compatible embedding API. Standard matching needs no extra components and remains the fail-open default.

---

## What the assistant can do

### Chat and act

- Default flow: think → pick a tool → run it → report; skill packs can add step-by-step flows (with optional approval points).
- The console can **follow along** as reasoning and tool steps happen.
- Switch among multiple model setups; tune thinking-related options when the model supports them.
- To add models, you can **fetch the endpoint's model list and batch-import**: enter the base URL (API key optional — add it if you get a 401), pull the models advertised by that OpenAI-compatible endpoint, select the ones you want, and create profiles in one go; you can also enter a single model manually when the endpoint has no list.
- **Skill packs**: instructions + tools. Checking skills in Settings and saving the **default configuration** only sets the **pre-activation** starting set for new chats; unchecked skills stay discoverable via `@`/`/` (including `/reload` in the completion popup) or `activate_skill`. Pins are stored on the user message so history bubbles keep the tags. Skill `tools:` is a **floor** by default (kept available), not an allowlist that hides other connector tools; agents may opt into **strict tool binding** (`tool_binding: exclusive`, still expandable via `activate_skill`). Catalog changes realign on the next turn (`skills_reload` / `/reload`, or automatically when the conversation is behind). Prefer `@login-…` for login skills—do not check them as defaults.
- **Workspaces**: switch or create a workspace in the sidebar; new chats land in the current workspace. v1 does not move existing chats between workspaces.
- Thinking is a one-line row by default (expand to read); token usage is written onto the reply when the run finishes.

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
- **Context compaction**: long threads get rolling structured checkpoints (goal / progress / decisions / facts to keep) so space goes to what still matters.
- **Keep important facts in long chats** (optional Runtime knob, default off): when a thread gets long, the assistant may keep a model-facing projection of facts it still needs; on failure it falls back to compaction. This does not promise lower cloud API token use.
- Conversation fork / rollback for everyday ops (console and API).
- Web login identities are shared **per workspace**; channel sessions (e.g. WeChat) and MCP-export identities stay on their own isolation keys.

### Messaging channels

Channels are a **general, extensible** capability: alerts, tickets, and instant messaging can share the same assistant, approvals, and outbound path.

- **Signed inbox**: external systems push alerts/tickets into the same assistant and approval flow.
- **Instant messaging**: the architecture supports IM via adapters. **The adapter shipped in this repository today is personal WeChat DM** (a small companion process plus an outbound queue with retry in settings). Other IMs can follow the same pattern — Baize is not limited to WeChat.
- Events can also be pushed to your own notification endpoints.

### For product, ops, and integrators

- Browser console in **Chinese / English**: conversations, tool cards, settings, and runtime knobs. Runtime settings are tabbed (Basics / Smart speed-up / Memory & compaction / Security); tool matching has its own page.
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

## Architecture: decision layer and tool matching

As more connectors are added, a turn may carry a large number of tool schemas, and the prefill cost grows with that number. Baize addresses this with a **decision layer** that moves high-frequency routing decisions out of generative calls into a cheap, deterministic step. The idea is inspired by the **Jev** approach to calibrated decisions; because Baize uses OpenAI-compatible APIs and cannot read logits, it does not use probability scores. The layer is local, pluggable, and always fails open.

When the tool count is above a threshold and DP-2a tool narrowing is on, candidates are narrowed before the main model:

1. **System routing** (optional) — which connectors the turn needs. Discriminative query terms force a connector; the model can only add connectors, not remove forced ones.
2. **Tool prefilter** — **standard matching** by default (BM25 over Latin terms and Chinese bigrams; no extra components). For many backends or mixed Chinese/English phrasing, open Settings → **Tool matching** and enable **enhanced matching**: install Ollama locally and pull an embedding model (e.g. `bge-m3`), or point at a cloud / self-hosted OpenAI-compatible embedding API. Enhanced mode uses BM25 + dense retrieval (RRF) and falls back to standard matching on failure so chat keeps working.

The console shows install progress, app/model paths, optional custom model directory, and cleanup (including optional Ollama uninstall).

The narrowing affects only **which schemas are in the prompt**, not tool availability: a registered tool still runs if requested. If nothing matches, it fails open and sends the full set; system and login tools are always kept.

### Benchmark (reproducible)

Setup: **37** real read-only business requests spanning **3** connected backends (**390** tools total), run against DeepSeek-Flash; stability measured over **5 rounds (185 requests)**. Corpus and scripts live in [`scripts/tool-routing-eval`](scripts/tool-routing-eval/README.md).

| Prefilter width | Run success | Avg. tools sent | Avg. turn-0 prompt | Prompt vs. width 32 |
|---|---|---|---|---|
| 32 | 36/37 | 43.1 | 4,703 | — |
| 24 | 37/37 | 37.4 | 3,980 | −15% |
| **16 (default)** | **37/37** | 30.6 | **3,090** | **−34%** |
| 12 | 37/37 | 27.1 | 3,033 | −36% |
| 8 | 35/37 | 23.6 | 2,600 | −45% |

Sending the **full 390-tool** catalog measured roughly **85k prompt tokens** on turn 0; at width 16 it measured about **1.4k–3.8k**. Over **5 repeated rounds (185 requests)** width 16 had a **99.5% run-success rate (184/185)**; the one failure came from an unrelated workflow, not from a tool being unavailable. At width 8, two multi-step requests failed. The default width is **16**.

The decision layer is **opt-in** and hot-reloadable via runtime settings (it is off by default); the benchmark reflects behavior once tool routing is enabled.

---

## Quick start

**Requirements:** Go 1.25+ (matches CI; no C compiler), Node.js 20+ (npm; Chat UI is embedded via `go:embed`), and an OpenAI-compatible API key.

```bash
cp .env.example .env        # then set BAIZE_API_KEY=sk-...
```

```powershell
.\serve.cmd                 # Windows — rebuilds web/chat → internal/ui/dist when sources change, then go build
```

```bash
./scripts/serve.sh          # macOS / Linux — same UI freshness check
```

- Console: http://127.0.0.1:8080/ui
- UI-only skip (reuse existing `internal/ui/dist`): `.\serve.cmd -SkipUI` / `BAIZE_SKIP_UI=1`
- Force UI rebuild: `.\serve.cmd -ForceUI` / `BAIZE_FORCE_UI=1`

Open the console and send a message. To change the model, base URL, or wire a business system, copy `configs/config.yaml` to `configs/config.local.yaml` (git-ignored), edit it, then start with `.\serve.cmd -config configs\config.local.yaml` (or `./scripts/serve.sh -config configs/config.local.yaml`).

---

## Deploy with prebuilt binaries (no Go required)

For production on a server or laptop: download the archive for your OS/arch from [GitHub Releases](https://github.com/rebornace/baize/releases) (`baize` plus optional `weixin-adapter`), unpack, then configure as below.

**1. Layout (example)**

```text
baize/
  baize                 # or baize.exe on Windows
  weixin-adapter        # only if you use the Weixin channel
  configs/config.yaml   # copy the sample from this repo
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
# from the unpack directory (uses configs/config.yaml)
./baize serve
# or pin another config file:
./baize serve -config configs/config.local.yaml
```

Console: http://127.0.0.1:8080/ui  

Change listen port, model URL, DB path, etc. via YAML / env and **restart the process** — no rebuild. Connectors and accounts are mostly configured in the UI and stored under `data/`.

For systemd and standalone Weixin adapter setup, see [deployment](docs/developers/deployment.md). To cross-compile locally from this repo:

```bash
# Windows PowerShell
.\scripts\build-release.ps1 -Version v0.3.0
# macOS / Linux
./scripts/build-release.sh v0.3.0
```

---

## Developer docs

Build, test, configuration, deployment, and HTTP surface (**Chinese and English**):

- [Developer docs (English)](docs/developers/README.en.md) · [中文](docs/developers/README.md)

---

## License

[MIT](LICENSE)
