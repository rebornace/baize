[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)

# Baize

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**English** | [中文](README.zh-CN.md)

**An enterprise AI Agent Runtime: it chats, calls tools, and waits for approval on writes — beside your existing systems, with almost nothing left when you stop it.**

Baize is not another chatbot skin. It is an auditable Agent: the model reasons, invokes tools, pauses for a human when needed, then continues — with the same Run trail across console and channels. Your business HTTP stack does not need a rewrite; an OpenAPI document is enough to give the Agent hands. `/ui` is an **operator console**, not a consumer entertainment chat app.

---

## Why Baize instead of “wrap ChatGPT around our API”

| Common approach | Baize |
|-----------------|--------|
| Stuff docs into the prompt and hope the model invents calls | **OpenAPI / Swagger / Postman → real executable Tools** in a catalog and Run trail |
| Let the model mutate production freely | **HITL on writes**: approve / reject on tool cards; pause and resume the Run |
| Embed an SDK and touch business code | **Zero-intrusion sidecar**: one Runtime process; stop it and leave almost nothing behind |
| Web chat only | **One Agent** across the console, signed Inbox, and personal WeChat DM — same Runs, approvals, and outbound |
| Scattered tools and ad-hoc auth | **Connectors + conversation identity + login Skills** for `require_login` downstream APIs |
| Long threads blow the context window | **Rolling compaction**, multi-model profiles, thinking level, Memory, and workspace files |

In short: **Baize sells an Agent that works, stays under control, and uninstalls cleanly — not another chat UI.**

---

## What the Agent can do (product highlights)

### Reason and act

- **ReAct Agent**: pick a tool → run it → record events → finish; optional linear Skill workflows (ordered steps + HITL gates).
- **Streaming Runs**: the console follows reasoning and tool steps over SSE — not a black-box wait.
- **Multi-model profiles**: switch LLM configs; tune **thinking level** where the provider dialect supports it.
- **Skills**: `SKILL.md` + tool bindings; default skills plus per-run `activate_skill` to widen capability.

### Where tools come from

- **OpenAPI → tools**: import a spec; each operation lands in the tool catalog for the Agent to call.
- **HTTP plugins & execution callbacks**: when the doc is incomplete, sidecar or enterprise callback covers legacy logic.
- **MCP both ways**: act as an **MCP client** (including OAuth login flows) and **export** a read-only tool subset to other hosts.
- **Login & identity**: for `require_login` APIs, managed login Skills / conversation identities — not secrets pasted into the prompt.

### Human-in-the-loop and boundaries

- Sensitive writes enter `waiting_human`; operators decide on the card, then `resume`.
- Tools can be disabled; full credentials stay out of event replay.
- Trial uses a mock LLM (no key); production uses a real model — paths stay separate.

### Memory, materials, long sessions

- **Memory**: account-scoped retrievable facts the Agent can use beyond the current window.
- **Workspace files / Blob**: attachments and objects via local or S3-style drivers.
- **Context compaction**: rolling summaries so tokens go to history that still matters.
- Conversation **fork / rollback** for operator-friendly recovery (console + API).

### Channels: meet people where they are

- **Signed Inbox**: alerts / tickets enter the same Agent and approval path.
- **Personal WeChat DM**: out-of-process adapter + outbound outbox (retry from settings) — not a demo-only webhook toy.
- Webhook outbound for enterprise notification buses.

### For integrators and operators

- Full **HTTP control plane** (runs, conversations, connectors, settings) — no mandatory language SDK.
- Bilingual **zh / en** `/ui`: conversations, tool cards, settings IA, humanized runtime knobs.
- Partial runtime hot-reload; SQLite by default, Postgres when you need it.

---

## Use cases

**Sidecar Agent for a legacy HTTP system**  
Service is live; you will not touch the code yet. Run Baize beside it, turn the API doc into tools, trial “query / create / approve” on the internal network, then decide on deeper changes.

**Ops-gated assistant for writes**  
The Agent can read, draft, and suggest; status changes and commands only run after a human clicks approve in `/ui`, with an auditable Run trail.

**One Agent for tools and channels**  
MCP tools, first-party HTTP, WeChat DM, Inbox alerts — one Runtime, one approval and memory story, instead of a Bot per entry point.

---

## Feel the Agent in 30 seconds

**Requirements:** Go 1.25+ (matches CI; no C compiler)

```powershell
# Windows
.\demo.cmd
```

```bash
# POSIX
./scripts/demo.sh
# or: go run ./cmd/baize demo
```

Trial stack = mock LLM + bundled demo HTTP — **no API key**.

- Console: http://127.0.0.1:8080/ui  
- Demo business HTTP: http://127.0.0.1:18080  

Open `/ui`, send “VPN is down, please file a record” — watch the Agent pick a tool, then **approve the write** on the card. If ports are busy, stop the old `baize` process or set `BAIZE_LISTEN` (e.g. `:9080`) and restart.

Production start (real LLM, requires `BAIZE_API_KEY`): Windows `.\start.cmd`, POSIX `./scripts/start.sh`.

---

## Developer docs

Build, test, config, deploy, and HTTP surface:

- [Developer docs index](docs/developers/README.md)
- [Getting started](docs/developers/getting-started.md) · [Architecture](docs/developers/architecture.md) · [Deployment](docs/developers/deployment.md) · [Configuration](docs/developers/configuration.md) · [HTTP API](docs/developers/http-api.md) · [Local probes](docs/developers/performance.md)

---

## License

[MIT](LICENSE)
