**English** | [中文](./mobile.md)

# Mobile

Baize has two separate mobile-related paths:

| Path | Where it runs | Role |
|------|---------------|------|
| **Capacitor / browser shell** (this doc’s focus) | Phone WebView / browser | **Remote control** of a deployed sidecar `baize serve` |
| **On-device trimmed orchestration** | Trimmed Go package on the device | Run loop on-device; device talks to the LLM API by default (eases server load) |

The on-device path ships as public package `mobileagent/`, with sample shells under `mobile/rt-shell` (Android) and `mobile/rt-shell-ios` (iOS). It does **not** embed full `baize serve`. Store listing is a later product decision — this page does not promise a date.

## Boundaries

| Layer | Where it runs | Notes |
|-------|---------------|--------|
| Sidecar (`baize serve`) | Linux / macOS / Windows | HTTP API, SSE, connectors, approvals, channels |
| Console UI (`web/chat`) | Browser or Capacitor WebView | Same-origin `/ui` + `/v0`, or `BAIZE_API_ORIGIN` in the shell |
| On-device bind (gomobile) | `mobileagent/` → AAR / XCFramework | `Configure` / `Send` / approve; AAR at `mobile/rt-shell/app/libs/`, iOS at `mobile/rt-shell-ios/Frameworks/`; **do not** bind `./cmd/baize` |

## Phone-browser baseline (before the shell)

On a device (or DevTools device mode) open `https://<host>/ui/` (reachable from the phone; often via `runtime.public_base_url`):

| Capability | Expectation | Code |
|------------|-------------|------|
| Auth | Bearer in `localStorage` when the gate is on | `controlAuth.ts`, `GateRoot` |
| SSE streaming | Incremental assistant output | `openRunStream` (`fetch` + stream, not `EventSource`) |
| Human approval | Approve/reject on cards; drawer on narrow screens | Chat list + ≤768 drawer |
| Browser login | Opens the system browser; resume after login | `openExternal` → Capacitor Browser / `window.open` |
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
| `BAIZE_SERVER_URL` | **Recommended**; WebView loads remote `/ui/` same-origin (SSE/approvals without CORS) |
| `BAIZE_API_ORIGIN` | Baked `VITE_API_ORIGIN` when shipping bundled www (no `BAIZE_SERVER_URL`) |
| Runtime `BAIZE_CORS_ORIGINS` | Comma-separated Origins on the **baize process** for bundled cross-origin API (e.g. `https://localhost,capacitor://localhost`); `*` for dev only |

Default server embeds are unchanged (`BAIZE_MOBILE` unset → `internal/ui/dist`, `base=/ui/`).

### Out of scope (Capacitor shell path)

- Treating the shell as on-device orchestration (the shell does not run a local engine; that is the `mobileagent` path)
- Embedding full `baize serve`, push notifications, or a React Native / Flutter console rewrite

## On-device trimmed orchestration (summary)

- **Package / API:** `mobileagent/` — `Configure` / `ChatOnce` / `Send` / `Approve` / `Deny`; implementation in `internal/mobileagent`.
- **Available today:** no-tools single turn; `get_time` / `http_get`; send and approve/deny; Android encrypted credential store and remote config fetch; iOS Keychain parity.
- **Rebuild:** Android AAR — `scripts/build-mobileagent-aar.ps1`; iOS XCFramework — `./scripts/build-mobileagent-ios.sh`. Shells: `mobile/rt-shell/`, `mobile/rt-shell-ios/`.

See also: [deployment](./deployment.en.md), [configuration](./configuration.en.md), [architecture](./architecture.en.md).
