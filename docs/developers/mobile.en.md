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
| `needs_approval` | Pending tool (currently `http_get`) | Show `summary` / `name`; Approve → `Approve(call_id)`, Deny → `Deny(call_id)` |
| `error` | Failure | Show `message` |

On `needs_approval` you also get `call_id`, `name`, `args_json`, `summary`. Do not call `Send` again until the user chooses.

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

Other useful APIs:

| API | Role |
|-----|------|
| `ChatOnce(text)` | One turn **without tools** (connectivity check) |
| `Reset()` | Clear in-memory history (keeps Configure) |
| `MessageCount()` | Messages in the session |
| `HasAPIKey()` | Whether a key is in memory |
| `NewAgent()` | Separate session (instance methods instead of package defaults) |

Desktop without a phone: `go run ./cmd/mobilebind`. Local mock LLM: `go run ./cmd/mobilebind/mockllm` (Android emulator often uses `http://10.0.2.2:<port>/v1` for the host).

### 3. Your job vs the SDK

| Your app | `mobileagent` |
|----------|----------------|
| Layout, theme, history list, composer, approval card | `Configure` / `Send` / `Approve` / `Deny` |
| Secure key storage, remote-config policy | In-memory session and tool loop |
| Off-main-thread network calls | Returns JSON; never touches UI |

Tools today: `get_time` (runs immediately); `http_get` (pauses for approval). No full sidecar connectors / MCP / channels.

---

## Which path

- Full settings, connectors, operator approvals → **A**.  
- Branded native UI, on-device loop, less reliance on a always-on sidecar → **B**, using `rt-shell` as the call sample.  
- Both: operators on A; end-user app on B.

## See also

- [Deployment](./deployment.en.md) · [Configuration](./configuration.en.md) · [Architecture](./architecture.en.md)
- Android shell notes: [`mobile/rt-shell/README.md`](../../mobile/rt-shell/README.md)
- iOS shell notes: [`mobile/rt-shell-ios/README.md`](../../mobile/rt-shell-ios/README.md)
