**English** | [中文](./mobile.md)

# Mobile: two ways to ship

| You want… | Use | Who owns the UI |
|-----------|-----|-----------------|
| Full operator console on a phone (chat, settings, approvals) | **A. Remote sidecar** | Baize’s built-in `/ui` (browser or Capacitor shell) |
| Your own app UI, with the chat loop on-device | **B. On-device SDK** | **You**; call the Go-built `mobileagent` library |

The paths do not replace each other. B does **not** embed full `baize serve`: no connector farm, MCP, channels, or full product HITL. You get an in-memory session, OpenAI-compatible chat, and a small tool set (`get_time` / approval-gated `http_get`).

---

## A. Remote sidecar (fastest)

Open a deployed Baize console on the phone, or wrap it in Capacitor.

**First prove the phone browser can open** `https://<reachable-host>/ui/` (`runtime.public_base_url` or LAN IP—not `127.0.0.1` from the phone). Auth, streaming, approval cards, and browser login should work.

Then the Capacitor shell ([`mobile/`](../../mobile/)):

```powershell
cd mobile
copy .env.example .env
# Recommended: BAIZE_SERVER_URL=http://192.168.x.x:8080
npm install
npm run sync
npm run add:android   # or add:ios on macOS
npm run open:android
```

| Variable | Role |
|----------|------|
| `BAIZE_SERVER_URL` | WebView loads remote `/ui/` same-origin (simplest for SSE/approvals) |
| `BAIZE_API_ORIGIN` | Only when shipping bundled www and calling the API cross-origin |
| Sidecar `BAIZE_CORS_ORIGINS` | Set on the **baize** process for cross-origin (e.g. `https://localhost,capacitor://localhost`) |

The shell handles safe areas, keyboard, and system-browser login. It does **not** run the on-device engine.

Server install and public base URL: [Deployment](./deployment.en.md), [Configuration](./configuration.en.md).

---

## B. On-device SDK: custom UI

Compile orchestration into the app: you own Kotlin/Swift (or other native) UI; the Go package owns the chat/tool loop. Reference shells (minimal apps you can copy calls from):

- Android: [`mobile/rt-shell/`](../../mobile/rt-shell/) (uses `app/libs/mobileagent.aar`)
- iOS: [`mobile/rt-shell-ios/`](../../mobile/rt-shell-ios/) (uses `Frameworks/Mobileagent.xcframework`)

### 1. Build the bind artifacts

Public bind entry: [`mobileagent/`](../../mobileagent/) (gomobile **cannot** bind `internal/`).

```powershell
# Android AAR (JDK 17+, ANDROID_HOME, NDK, gomobile)
.\scripts\build-mobileagent-aar.ps1
# Output: mobile/rt-shell/app/libs/mobileagent.aar
```

```bash
# iOS XCFramework (macOS, Xcode, gomobile)
./scripts/build-mobileagent-ios.sh
# Output: mobile/rt-shell-ios/Frameworks/Mobileagent.xcframework
```

Link the AAR / XCFramework into **your** project (Gradle `implementation files(...)` / Xcode Embed & Sign). You do not have to ship the sample shell apps.

### 2. Call sequence (you paint the UI)

**Configure once at startup** (OpenAI-compatible base + key + model; key stays in memory—**do not** expect Go to persist it):

```kotlin
// Android (package mobileagent)
mobileagent.Mobileagent.configure(
  "https://api.example.com/v1",
  apiKeyFromYourVault,  // prefer EncryptedSharedPreferences (or similar) in the app
  "your-model"
)
```

```swift
// iOS
var err: NSError?
MobileagentConfigure("https://api.example.com/v1", apiKeyFromKeychain, "your-model", &err)
```

Optional remote config (JSON with `api_base` / `model`, optional `api_key`):

- `FetchRemoteConfig(url)` → JSON status string  
- or `ApplyRemoteConfigJSON(raw)`  

Remote config may also carry an **OpenAPI connector**. GET is parsed by default; POST/PUT/PATCH need `"allow_writes": true` or an explicit `"methods": ["GET","POST"]` list (**DELETE is unsupported**). A non-empty `methods` list overrides `allow_writes`:

```json
{
  "api_base": "https://api.deepseek.com",
  "model": "deepseek-chat",
  "api_key": "...",
  "readonly_api": {
    "base_url": "https://tickets.example.com",
    "auth_header": "Authorization",
    "auth_value": "Bearer ...",
    "spec": "openapi: 3.0.3 ...",
    "methods": ["GET", "POST", "PUT"]
  }
}
```

Use `"spec_url": "https://.../openapi.yaml"` instead of `spec` when fetching through `FetchRemoteConfig` (it downloads the spec). Every connector call, read or write, returns `needs_approval`; no request is sent until the user approves. Writes add these constraints:

### Network boundary (default deny)

`http_get`, `FetchRemoteConfig`, `spec_url` and every connector call reject loopback, private, link-local, CGNAT, multicast and reserved addresses by default. The check runs **after DNS resolution on the address actually dialed**, so DNS rebinding and redirects cannot escape it; environment HTTP proxies are not used for these calls.

Enable it only for a trusted on-device or internal deployment:

- code: `SetAllowPrivateNetworks(true)` (also available on `Agent`)
- remote config: top-level `"allow_private_networks": true`

Prefer a narrower allowlist:

```json
{
  "allowed_private_hosts": ["crm.internal"],
  "allowed_private_cidrs": ["10.20.0.0/16", "127.0.0.1/32"]
}
```

or call `SetPrivateNetworkAllowlist(json)`. CIDRs are required; `hosts` is optional and only restricts DNS names resolving into those ranges. IP literals are checked against CIDRs only. An empty allowlist does not disable the default deny.

The flag is session-wide, defaults to `false`, and is not implied by `allow_writes` or `methods`. To fetch a private config URL, opt in before calling `FetchRemoteConfig`.

Writes add these constraints:

- **Method allowlist:** `methods` may contain `GET` / `POST` / `PUT` / `PATCH`; empty or absent falls back to `allow_writes`. DELETE is never supported.
- **JSON:** `application/json` object schemas expand into tool parameters; array/scalar schemas use one `body` parameter carrying the complete JSON.
- **Form:** `application/x-www-form-urlencoded` object schemas expand into fields; body cap 64 KiB.
- **Multipart:** `multipart/form-data` object schemas expand into fields; `type: string, format: binary` fields take a **base64** string, with an optional `<field>_filename` argument. One file is capped at 512 KiB and the whole multipart body at 1 MiB.
- Write requests do not follow 3xx redirects; path/query parameters work as usual; write tools share the same global tool-name namespace.

For multiple connectors use the `readonly_apis` array (mutually exclusive with `readonly_api`; each entry uses either `spec` or `spec_url` and decides its own `allow_writes` / `methods`):

```json
"readonly_apis": [
  {
    "id": "tickets",
    "base_url": "https://tickets.example.com",
    "auth_header": "Authorization",
    "auth_value": "Bearer ...",
    "spec": "openapi: 3.0.3 ...",
    "methods": ["GET", "POST", "PATCH"]
  },
  {
    "id": "crm",
    "base_url": "https://crm.example.com",
    "auth_header": "X-API-Key",
    "auth_value": "...",
    "spec_url": "https://crm.example.com/openapi.yaml"
  }
]
```

Tool names must be unique across every connector and the built-ins; the limit is 8 connectors and 100 tools. The SDK also accepts `ConfigureReadOnlyAPIs(json)` (a JSON array with the same fields). The single-connector `ConfigureReadOnlyAPI(...)` stays GET-only; use the JSON form with `allow_writes` or `methods` for writes.

The Android AAR in this repo already includes the new APIs; the iOS XCFramework must be rebuilt with `./scripts/build-mobileagent-ios.sh` on macOS to pick them up.

The shells’ Android `CredentialStore` / iOS Keychain show “app holds the key, then Configure”—custom UIs should do the same.

**Send a user turn (with tools):**

```kotlin
val raw = mobileagent.Mobileagent.send(userText)
// parse raw (table below), update your bubbles / approval card
```

```swift
let raw = MobileagentSend(userText)
```

**`Send` / `Approve` / `Deny` all return JSON strings** for your UI to parse:

| `status` | Meaning | UI hint |
|----------|---------|---------|
| `done` | Turn finished | Show `text` |
| `needs_approval` | Pending tool (`http_get` or a connector call, including writes) | Show `summary` / `name` / `method`; writes also carry `write: true`. Approve → `Approve(call_id)`, Deny → `Deny(call_id)` |
| `error` | Failure | Show `message` |

On `needs_approval` you also get `call_id`, `name`, `args_json`, `summary`, `method`. A write summary looks like `POST /tickets {"title":"VPN outage"}` and can go straight into the approval card. Do not call `Send` again until the user chooses.

```kotlin
when (JSONObject(raw).getString("status")) {
  "done" -> showAssistant(JSONObject(raw).getString("text"))
  "needs_approval" -> showApproveCard(
    callId = JSONObject(raw).getString("call_id"),
    summary = JSONObject(raw).optString("summary")
  )
  "error" -> showError(JSONObject(raw).getString("message"))
}
// On approve:
val cont = mobileagent.Mobileagent.approve(callId)
// parse cont the same way
```

**Streaming replies:**

```kotlin
mobileagent.Mobileagent.startStream(userText)   // {"status":"streaming"}
// Poll every 100–200ms; delta is new text, text is the accumulated stream
val raw = mobileagent.Mobileagent.pollStream()
// {"status":"streaming","delta":"…","text":"…"}
// Final poll adds "result" (same JSON as Send) and "final":true
mobileagent.Mobileagent.cancelStream()          // optional: stop the stream
mobileagent.Mobileagent.retryStream()           // after error/cancel: reuse the last user turn
```

- `status`: `streaming` / `cancelling` / `done` / `needs_approval` / `error` / `cancelled`.
- Append `delta`, or replace the bubble with `text`; the final text matches `Send`.
- Approval ends the stream: `status=needs_approval` and `result` carries `call_id` / `summary`. Call `Approve` / `Deny` as usual (the post-approval continuation is synchronous and not streamed).
- Cancel aborts the waiting LLM request; a tool already running finishes its current timeout. `text` keeps the deltas received so far.
- A network/SSE interruption ends with `status=error`; `text` keeps the partial reply and `result` carries the reason. `RetryStream()` reuses the last user message instead of adding a duplicate turn.
- If the server ignores `stream` and returns plain JSON, the SDK emits one full `delta`; callers need no special case.
- One stream per session; `Send` returns an error while a stream is active.
- On Activity recreation or return to foreground, the shell reuses the same `Agent` and calls `pollStream()` again; a cold process restart restores the session snapshot and shows an interruption note instead of clearing the partial reply.

**Multiple sessions:**

```kotlin
val a = mobileagent.Mobileagent.newAgent()   // fresh session; configure it
val b = a.newSession()                       // copy model config + connectors, no history/pending
a.send("A")
b.send("B")                                  // histories stay independent
a.pendingApproval()                          // {"status":"none"} or the current approval JSON
```

- Each `Agent` owns its messages, stream and pending approval. Restore the approval card with `pendingApproval()` when switching.
- `newSession()` copies `api_base` / model / key / network policy / connector tools; it does not copy messages, streams or pending calls.
- `reset()` clears only the current session's history and pending.
- Keep one streaming session at a time; finish or cancel the stream before switching.
- Package-level `Send` / `StartStream` still use the default session; multi-session UIs should hold `NewAgent()` instances.

**Attachments (metadata only):**

```kotlin
val atts = """[
  {"id":"a1","name":"photo.jpg","mime":"image/jpeg","size":1234,
   "local_ref":"/data/user/0/.../files/attachments/a1"}
]"""
agent.startStreamWithAttachments("look at this", atts)
agent.sendWithAttachments("one more note", atts)
agent.sessionAttachments()
// {"attachments":[{"message_index":0,"role":"user","attachments":[...]}]}
```

- Only **display metadata** is stored; file bytes stay in the shell/local storage and are never sent to the model.
- Session snapshots include attachment metadata; after `ImportSession`, `sessionAttachments()` restores the cards.
- If the `local_ref` file is missing, the shell should show “file missing” without blocking text history.
- Limits: 8 attachments per message; attachments JSON ≤64 KiB; `local_ref` ≤1024 bytes; metadata `size` 0–50 MiB; the sample shell copies at most 10 MB.
- `RetryStream()` reuses the original user message, so attachment metadata survives a retry.

**Session persistence:**

```kotlin
val snap = agent.exportSession()                 // JSON string
val restored = mobileagent.Mobileagent.newAgent()
restored.importSession(snap)                     // {"status":"done","message_count":N}
```

- The snapshot contains message history (including tool calls/results and attachment metadata), count and network policy; **no API key, connector auth, pending approval, stream, or attachment file bytes**.
- The shell stores one snapshot per session in `EncryptedSharedPreferences` and restores names/texts on launch.
- Incomplete tool calls are not restored: export truncates at the last complete turn to avoid a dangling `tool_call_id`.
- Pending approvals do not survive process restart; the shell replaces old `HITL:` text with “Previous approval expired after restart.”.
- Limits: 4 MiB / 2000 messages; import validates roles and tool pairing.

Other useful APIs:

| API | Role |
|-----|------|
| `ChatOnce(text)` | One turn **without tools** (connectivity check) |
| `StartStream(text)` / `PollStream()` / `CancelStream()` / `RetryStream()` | Background streaming turn, poll deltas, cancel, retry the last turn |
| `SendWithAttachments(text, json)` / `StartStreamWithAttachments(text, json)` / `SessionAttachments()` | Turn with attachment metadata; read attachment cards after restore |
| `PendingApproval()` / `NewSession()` | Current HITL JSON; clone config/tools into a new session |
| `ExportSession()` / `ImportSession(json)` | History snapshot/restore (no keys or approvals) |
| `Reset()` | Clear in-memory history (keeps Configure) |
| `MessageCount()` | Messages in the session |
| `HasAPIKey()` | Whether a key is in memory |
| `ConfigureReadOnlyAPI(base, authHeader, authValue, spec)` | Configure one GET-only OpenAPI connector; failure keeps the old tools |
| `ConfigureReadOnlyAPIs(json)` | Configure several connectors from a JSON array; optional `allow_writes` / `methods`; names unique, up to 8 connectors |
| `ClearReadOnlyAPI()` | Remove connector tools and any pending remote call |
| `RemoteToolCount()` | Current connector tool count |
| `RemoteWriteToolCount()` | How many of them are POST/PUT/PATCH write tools |
| `RemoteToolNames()` | Current tool names (comma-separated; empty means none) |
| `NewAgent()` | Separate session (instance methods instead of package defaults) |

Desktop without a phone: `go run ./cmd/mobilebind`. Local mock LLM: `go run ./cmd/mobilebind/mockllm` (Android emulator often uses `http://10.0.2.2:<port>/v1` for the host).

**Local connector demo:** start one more process (it serves the config, the OpenAPI spec and a demo ticket API with one POST write):

```powershell
go run ./cmd/mobilebind/demo-connector -addr 127.0.0.1:18081
```

Set the shell's Remote config URL to:

- Android emulator: `http://10.0.2.2:18081/config.json`
- iOS simulator: `http://127.0.0.1:18081/config.json`
- Physical device: run with `-addr 0.0.0.0:18081` and use your computer's LAN IP

After Fetch config the shell should show `Tools: list_tickets, create_ticket, get_ticket`; listing tickets pauses on a `GET /tickets?...` approval card, and creating one pauses on `POST /tickets {"title":…}` with `write: true`. The request is only sent after Approve. The `X-API-Key: demo-key` header is for local demos only—replace it in production.

On launch the shell re-fetches a saved Remote config URL once, so tools come back without tapping the button. If that request fails it stays quiet, keeps the local LLM config, and tries again next launch.

### 3. Your job vs the SDK

| Your app | `mobileagent` |
|----------|----------------|
| Layout, theme, history list, composer, approval card | `Configure` / `Send` / `Approve` / `Deny` |
| Secure key storage, remote-config policy | In-memory session and tool loop |
| Off-main-thread network calls | Returns JSON; never touches UI |

Tools today: `get_time` (runs immediately); `http_get` (pauses for approval); up to 8 OpenAPI connectors (GET by default, POST/PUT/PATCH with `allow_writes` or `methods`; every call pauses for approval, path/query and JSON/form/multipart bodies, one custom auth header per connector). No full desktop connectors / MCP / channels.

### 4. iOS App Store (M3)

- Project: [`mobile/rt-shell-ios/`](../../mobile/rt-shell-ios/); includes AppIcon, AccentColor, launch background, `PrivacyInfo.xcprivacy`, and iPhone-only settings.
- Validate: `python mobile/rt-shell-ios/scripts/check-release.py`
- Archive: `DEVELOPMENT_TEAM=<team> ./mobile/rt-shell-ios/scripts/archive-ios.sh`
- Store copy / privacy answers / review notes / screenshot sizes / submission checklist: [`mobile/rt-shell-ios/AppStore/README.md`](../../mobile/rt-shell-ios/AppStore/README.md)

Signing, TestFlight and App Review need an Apple Developer account. The repo contains no certificates, Team ID, or reviewer demo key.

### 5. Android Play Store (M3)

- Project: [`mobile/rt-shell/`](../../mobile/rt-shell/); includes adaptive/legacy icons, launch background, manifest, version 1.0.0, Gradle wrapper, and a signing template.
- Validate: `python mobile/rt-shell/scripts/check-release.py`
- Archive: copy `keystore.properties.example`, fill local signing values, then run `.\mobile\rt-shell\scripts\archive-android.ps1`
- Store copy / Data safety / screenshot sizes / submission checklist: [`mobile/rt-shell/PlayStore/README.md`](../../mobile/rt-shell/PlayStore/README.md)

Play Console, upload keys and review need an account. The repo contains no keystore, passwords, or reviewer demo key.

---

## Which path

- Full settings, connectors, operator approvals → **A**.  
- Branded native UI, on-device loop, less reliance on a always-on sidecar → **B**, using `rt-shell` as the call sample.  
- Both: operators on A; end-user app on B.

## See also

- [Deployment](./deployment.en.md) · [Configuration](./configuration.en.md) · [Architecture](./architecture.en.md)
- Android shell notes: [`mobile/rt-shell/README.md`](../../mobile/rt-shell/README.md)
- iOS shell notes: [`mobile/rt-shell-ios/README.md`](../../mobile/rt-shell-ios/README.md)
