**English** | [中文](./mobile.md)

# Mobile (client shell)

Baize is a **sidecar Agent Runtime on a server**. This doc covers the Capacitor / browser path as an **operator console client** (talks to a deployed instance).

**On-device trimmed Go Agent (MOBILE-RT)** is captured in planning and **deferred** (large effort; a remote-only shell is weak for “Agent on the phone”). Requirement note lives on `baize_real`: `docs/superpowers/notes/2026-10-22-mobile-on-device-runtime.md` (not exported to the public tree).

## Boundaries

| Layer | Where it runs | Notes |
|-------|---------------|--------|
| Runtime (`baize serve`) | Linux / macOS / Windows | HTTP API, SSE, connectors, HITL, channels |
| Console UI (`web/chat`) | Browser or Capacitor WebView | Same-origin `/ui` + `/v0`, or `BAIZE_API_ORIGIN` in the shell |
| gomobile | **Not used** | Fine for tiny Go bindings; not a substitute for a long-lived Runtime + React console |

## Phone-browser baseline (before the shell)

On a device (or DevTools device mode) open `https://<host>/ui/` (reachable from the phone; often via `runtime.public_base_url`):

| Capability | Expectation | Code |
|------------|-------------|------|
| Auth | Bearer in `localStorage` when the gate is on | `controlAuth.ts`, `GateRoot` |
| SSE streaming | Incremental assistant output | `openRunStream` (`fetch` + stream, not `EventSource`) |
| HITL | Approve/reject on cards; drawer on narrow screens | Chat list + ≤768 drawer |
| Hosted login | Opens the system browser; resume after login | `openExternal` → Capacitor Browser / `window.open` |
| Uploads | Composer / skill upload via `<input type="file">` | OS picker; add store permissions as needed |

If this fails, fix **reachability / mixed content** first (`http://127.0.0.1` is not reachable from a phone).

## Capacitor shell (`mobile/`)

```powershell
cd mobile
copy .env.example .env
# Recommended: BAIZE_SERVER_URL=http://192.168.x.x:8080  (phone-reachable; same-origin /ui + SSE)
npm install
npm run sync
npm run add:android   # or add:ios on macOS
npm run open:android
```

| Variable | Role |
|----------|------|
| `BAIZE_SERVER_URL` | **Recommended**; WebView loads remote `/ui/` same-origin (SSE/HITL without CORS) |
| `BAIZE_API_ORIGIN` | Baked `VITE_API_ORIGIN` when shipping bundled www (no `BAIZE_SERVER_URL`) |
| Runtime `BAIZE_CORS_ORIGINS` | Comma-separated Origins on the **baize process** for bundled cross-origin API (e.g. `https://localhost,capacitor://localhost`); `*` for dev only |

Default server embeds are unchanged (`BAIZE_MOBILE` unset → `internal/ui/dist`, `base=/ui/`).

### Out of scope (this phase)

- Compiling `./cmd/baize` with gomobile for the stores
- On-device full Runtime, offline agent, push notifications
- React Native / Flutter rewrite of the console

See also: [deployment](./deployment.en.md), [configuration](./configuration.en.md), [architecture](./architecture.en.md).
