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

Baize is a team AI assistant that runs beside your existing services. It reads API docs, calls systems you already have, waits for a person in the web console when a write needs approval, and writes results back to the chat or a messaging channel. You usually do not change business code. On a phone you can remote the same console, or embed a trimmed chat engine in your own app.

The name comes from the legendary Baize that knows the names of all things. The web console is for product, ops, and integrators.

---

## What it does

**Works on phone.** Two paths: (1) open remote `/ui` in a browser or a Capacitor shell (same chat, settings, and approvals as desktop); (2) ship the Go-built `mobileagent` (AAR / XCFramework) for on-device trimmed orchestration and **draw your own UI**. Full guide: [Mobile](docs/developers/mobile.en.md).

**Connect systems.** Upload or point to API docs (OpenAPI / Swagger, etc.); each operation becomes something the assistant can call, visible in the console. When docs are incomplete, run a small service that declares abilities and how to invoke them, or have Baize call your URL so you execute on your side. You can also attach MCP-capable tool servers.

**Approve writes.** Creates and status changes can pause until a teammate approves or rejects on a card, with a trail. Abilities can be disabled anytime; full secrets stay out of conversation replay.

**One assistant, several doors.** Console, phone shell, alert/ticket inbox, and messaging share approvals, memory, and outbound replies (the on-device SDK is a separate trimmed path). This repo ships personal WeChat DM; Feishu and DingTalk examples are included. Events can also go to your own notification URL.

**Downstream login.** Use “browser login” in chat, or configure a login page in Settings. Passwords never go through the model; then the assistant calls systems with the session. Wired abilities can be exposed read-only to Cursor, Claude Desktop, Doubao, and similar clients; when login expires, open the returned link—do not paste passwords into the chat. The login page must be reachable from the client’s browser.

**Narrow when you have many systems.** With Matching & decisions and Runtime → Smart speed-up, Baize can judge which systems and abilities this turn needs, then send a shorter list to the main model. On failure it uses the full list. Configuring a judge model alone does not cut main-model usage until those switches are on. Group domains such as finance and commerce; start a separate sub-task when approvals must not collide.

**See API changes; inspect a turn.** After re-import, compare what changed (optional notify). Beside each chat turn, open which model and abilities were used. Settings shows runtime metrics and recent logs; common metrics scrape and optional tracing; some observability options apply without editing config files. Outbound calls can use local certificates.

**Long chats and schedules.** Long threads get summaries; optional “keep important facts.” Oversized tool results or pastes can become workspace files with a short preview in chat. After approval, save a one-shot, daily, or weekly instruction; at fire time Baize starts a new turn in the same chat (writes still wait for approval). Account-level memory and attachments. Fork or roll back conversations.

**Workspaces.** Switch or create workspaces in the sidebar; new chats land in the current workspace (moving old chats is not supported yet). Logins are shared inside a workspace; workspaces, messaging channels, and “expose abilities” identities stay separate. A built-in help skill (English / Chinese) points to the right Settings page; it does not edit config or restart the service.

**Console.** Chinese and English; chats, call cards, settings, and runtime options. Matching & decisions, external login, expose abilities, runtime metrics, and eval corpus live under Settings. Full HTTP management surface; no mandatory language SDK. A local database is enough to start.

Trial can use a built-in mock model (no cloud key yet); production uses a real model. One process, config outside the app; stop it and leave almost nothing on the business side.

---

## On phone: remote console, or your UI + on-device SDK

| You want… | Do this |
|-----------|---------|
| Full console on a phone | Open reachable `/ui/` in a browser, or use the [`mobile/`](mobile/) Capacitor shell |
| Your own app UI | Build the [`mobileagent/`](mobileagent/) bind package; call `Configure` / `Send` / approve from Kotlin or Swift |

The on-device path is **not** full `baize serve` in the app: no connector farm or full HITL product surface—in-memory sessions, OpenAI-compatible chat, and a small tool set. Guide and sample shells: [Mobile](docs/developers/mobile.en.md).

---

## Where it fits

A live service you will not change yet: run Baize beside it, hand abilities over via the API doc, trial query / create / approve internally.

The assistant can read, draft, and suggest; status changes and commands wait for confirmation in the console.

External tools, first-party plugins, inbox, and IM share one approval and memory path.

Several business domains: group them; start a sub-task only when you need isolation that does not collide.

Reuse wired abilities in Cursor and similar clients: connect and export in Baize; open the login link when auth expires.

APIs drift, turns need a trail, outbound needs encryption: compare re-import diffs, inspect bindings, use certificates and metrics—without another assistant stack.

---

## Many systems: narrow first

Sending every ability every turn is slow and expensive. With Smart speed-up on, Baize judges first, then sends a shorter list; on failure it uses the full list. Details: [Architecture](docs/developers/architecture.en.md), [Configuration](docs/developers/configuration.en.md).

### Benchmark

37 real read-only requests across 3 backends (390 abilities), measuring main-model first-turn input size; 5 rounds, 185 requests. Scripts: [`scripts/tool-routing-eval`](scripts/tool-routing-eval/README.md).

| Cap on abilities sent | Success | Avg. abilities sent | Avg. first-turn input | vs. cap 32 |
|---|---|---|---|---|
| 32 | 36/37 | 43.1 | 4,703 | — |
| 24 | 37/37 | 37.4 | 3,980 | −15% |
| **16 (default)** | **37/37** | 30.6 | **3,090** | **−34%** |
| 12 | 37/37 | 27.1 | 3,033 | −36% |
| 8 | 35/37 | 23.6 | 2,600 | −45% |

All 390 abilities in the first turn measured about 85k input tokens; at cap 16, about 1.4k–3.8k. Success over 185 requests was about 99.5% (184/185). On the same machine, judging with a small local model was about 1.5× faster end-to-end than judging with another large model. Field notes: developer docs and the script folder.

---

## Quick start

Go 1.25+, Node.js 20+ (builds the web console), and an OpenAI-compatible API key.

```bash
cp .env.example .env        # set BAIZE_API_KEY=sk-...
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

Open the console and send a message. To change the model or wire a system, copy `configs/config.yaml` to `configs/config.local.yaml` (git-ignored) and start with `-config`.

---

## Prebuilt binaries (no Go)

Download from [GitHub Releases](https://github.com/rebornace/baize/releases).

```text
baize/
  baize                 # or baize.exe on Windows
  weixin-adapter        # only for the WeChat channel
  configs/config.yaml
  .env
  data/
```

```bash
BAIZE_API_KEY=sk-your-model-key
BAIZE_SETTINGS_KEY=a-long-random-string
# optional: BAIZE_LISTEN=:8080
```

```bash
./baize serve
# or: ./baize serve -config configs/config.local.yaml
```

Console: http://127.0.0.1:8080/ui  

Restart after changing listen port or model URL. Most wiring is done in the console. systemd and WeChat: [deployment](docs/developers/deployment.en.md).

---

## Developer docs

Build, test, configuration, deployment, and HTTP (system names kept; Chinese and English):

- [Developer docs (English)](docs/developers/README.en.md) · [中文](docs/developers/README.md)
- [Mobile](docs/developers/mobile.en.md) (detail for the section above) · [中文](docs/developers/mobile.md)

---

## License

[MIT](LICENSE)
