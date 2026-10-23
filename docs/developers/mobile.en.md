**English** | [中文](./mobile.md)

# Mobile

Baize has two separate mobile-related paths:

| Path | Where it runs | Role |
|------|---------------|------|
| **Capacitor / browser shell** (this doc’s focus) | Phone WebView / browser | **Remote control** of a deployed sidecar `baize serve` |
| **MOBILE-RT** (on-device orchestration) | Trimmed Go agent embedded on the device | **Offload the server**: Run loop on-device; device talks to the LLM API by default |

**MOBILE-RT** design (real repo; not always in the public export): `docs/superpowers/specs/2026-10-23-mobile-rt-design.md`. **M0 done** (thin package + desktop demo)—not shipping full `baize serve` inside an app. Product path: proceed to **store listing** (spec phase M3) after feasibility.

## Boundaries

| Layer | Where it runs | Notes |
|-------|---------------|--------|
| Runtime (`baize serve`) | Linux / macOS / Windows | HTTP API, SSE, connectors, HITL, channels; sidecar mainline |
| Console UI (`web/chat`) | Browser or Capacitor WebView | Same-origin `/ui` + `/v0`, or `BAIZE_API_ORIGIN` in the shell |
| gomobile (MOBILE-RT) | Public `mobileagent/` → AAR | `Configure` / `ChatOnce`; AAR at `mobile/rt-shell/app/libs/`; **do not** bind `./cmd/baize` |

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

### Out of scope (Capacitor shell path)

- Treating the shell as an on-device Agent (the shell does not run a local Engine; that is MOBILE-RT)
- Embedding full `baize serve`, push notifications, or a React Native / Flutter console rewrite

### MOBILE-RT (separate path, summary)

- **API:** `mobileagent/` — `Configure` / `ChatOnce` / `Send` / `Approve` / `Deny`; impl in `internal/mobileagent`.  
- **M0:** one no-tools turn + AAR.  
- **M1:** `get_time` + `http_get`; `Send`/`Approve`/`Deny`.  
- **M2:** Android EncryptedSharedPreferences vault; `FetchRemoteConfig`; iOS Keychain (build Framework on Mac). Rebuild AAR: `scripts/build-mobileagent-aar.ps1`.  
- **Later:** M3 store listing; no public README promise until ready.

See also: [deployment](./deployment.en.md), [configuration](./configuration.en.md), [architecture](./architecture.en.md).
