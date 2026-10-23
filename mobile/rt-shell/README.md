# Android 机上空壳（示例 App）

演示如何把 `mobileagent.aar` 嵌进原生界面：**你画 UI，SDK 跑对话**。完整步骤与 JSON 约定见开发者文档：

→ [docs/developers/mobile.md](../../docs/developers/mobile.md)（「B. 机上 SDK」）

本目录是最小可运行样板，不是唯一界面。可只取 AAR + 调用代码，换成自己的 Activity / Compose。

## 产物

| 产物 | 路径 |
|------|------|
| AAR | [`app/libs/mobileagent.aar`](app/libs/mobileagent.aar) |
| sources | `app/libs/mobileagent-sources.jar` |

## 调用摘录

```kotlin
mobileagent.Mobileagent.configure(apiBase, apiKey, model)
mobileagent.Mobileagent.fetchRemoteConfig(configUrl) // 可选
val json = mobileagent.Mobileagent.send(userText)
// status=needs_approval → approve(callId) / deny(callId)
```

密钥由 App 侧 `CredentialStore`（EncryptedSharedPreferences）保管，再传入 `configure`。iOS 对位见 [`../rt-shell-ios/`](../rt-shell-ios/)。

模拟器访问宿主机 mock：`http://10.0.2.2:18080/v1`（先 `go run ./cmd/mobilebind/mockllm`）。

## 重建 AAR / 安装 APK

```powershell
.\scripts\build-mobileagent-aar.ps1
cd mobile\rt-shell
gradle assembleDebug
adb install -r app\build\outputs\apk\debug\app-debug.apk
```

| 路径 | 作用 |
|------|------|
| `mobile/`（Capacitor） | 遥控远端完整操作台 |
| `mobile/rt-shell/`（本目录） | 机上 SDK + 示例原生 UI |
