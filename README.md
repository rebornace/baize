[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)

# Baize

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

**English** | [中文](README.zh-CN.md)

**Add AI next to your existing HTTP APIs — no rewrite, nothing left behind.**

Baize is an Agent Runtime that sits beside your services: turn OpenAPI into callable tools, require human approval for writes, and stop the process when you are done. `/ui` is an operator console, not a consumer chat product.

---

## Highlights

- **Zero intrusion:** No changes to the business process, no embedded SDK — sidecar on, sidecar off.
- **OpenAPI → tools:** Upload or point at an API document; operations become assistant capabilities.
- **Approval gate for writes:** Mutating calls can wait for a human before hitting your systems.
- **Clean uninstall:** Single process, config outside the app — stop Runtime and leave almost nothing behind.
- **Chinese / English UI:** `/ui` supports both; language preference stays in this browser only.

---

## Core capabilities

- Connect existing HTTP services so operators can chat and invoke them (OpenAPI / Swagger / Postman when you have a doc; a small HTTP plugin when you do not).
- Human-in-the-loop (HITL): approve or reject sensitive writes on tool cards.
- Operator console: conversations, tool cards, settings, and approvals in `/ui`.
- Optional MCP tools, signed Inbox ingress for alerts/tickets, personal WeChat DM channel, and more — see developer docs and the settings UI.
- Separate trial vs production paths: demo uses a mock LLM (no API key); production needs a real LLM key.

---

## Use cases

**Sidecar AI for a legacy HTTP system**  
The service is already live and you do not want to touch the code. Run Baize beside it, turn the API doc into tools, trial on the internal network, then decide on deeper changes.

**Ops-approved write actions**  
The assistant can read and suggest, but creates, status changes, and commands must be approved in `/ui` before they run — reducing mistakes and overreach.

---

## Try it in 30 seconds

**Requirements:** Go 1.25+ (matches CI; no C compiler)

Windows (from repo root):

```powershell
.\demo.cmd
```

POSIX:

```bash
./scripts/demo.sh
# or: go run ./cmd/baize demo
```

The trial stack uses a mock LLM and bundled demo HTTP — **no API key**.

- Console: http://127.0.0.1:8080/ui
- Demo HTTP: http://127.0.0.1:18080

In `/ui`, try “VPN is down, please file a record” and approve the write on the tool card. If ports are busy, stop the previous `baize` process, or set `BAIZE_LISTEN` (e.g. `:9080`) and restart.

Production start (real LLM, requires `BAIZE_API_KEY`): Windows `.\start.cmd`, POSIX `./scripts/start.sh`. Config, deploy, and channels live in the developer docs.

---

## Performance

Benchmark numbers will be filled in a later **PERF** pass. This section intentionally has **no** competitor comparison figures; we only publish reproducible measurements when they exist.

---

## Next steps (technical readers)

Build, test, configuration, HTTP surface, and deployment:

- [Developer docs index](docs/developers/README.md)
- [Getting started](docs/developers/getting-started.md)
- [Architecture](docs/developers/architecture.md) · [Deployment](docs/developers/deployment.md) · [Configuration](docs/developers/configuration.md) · [HTTP API](docs/developers/http-api.md)

---

## License

[MIT](LICENSE)
