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

连接器演示（含一个 POST 写操作）：另开 `go run ./cmd/mobilebind/demo-connector -addr 127.0.0.1:18081`，Remote config URL 填 `http://10.0.2.2:18081/config.json`；点 Fetch config 后应显示 `Tools: list_tickets, create_ticket, get_ticket (1 write)`。写调用会先弹审批卡（`POST /tickets …`）。之后启动 App 会自动重拉已保存的配置；失败静默保留本地 LLM 配置。

## M3 上架

已备：自适应/传统图标、启动背景、Manifest、版本 1.0.0、Gradle Wrapper、签名模板、AAB 脚本与 Play 商店资料。

```powershell
python mobile\rt-shell\scripts\check-release.py
copy mobile\rt-shell\keystore.properties.example mobile\rt-shell\keystore.properties
# 填本地 keystore 路径与密码后：
.\mobile\rt-shell\scripts\archive-android.ps1 -Version 1.0.0 -VersionCode 1
```

Play Console 签名与提审需要账号；审核演示端点、真实截图与隐私政策 URL 由 App 所有者补齐。商店文案与提审清单见 [`PlayStore/README.md`](PlayStore/README.md)。

## 重建 AAR / 安装 APK

```powershell
cd mobile\rt-shell
..\..\scripts\build-mobileagent-aar.ps1
.\gradlew.bat assembleDebug
adb install -r app\build\outputs\apk\debug\app-debug.apk
```

| 路径 | 作用 |
|------|------|
| `mobile/`（Capacitor） | 遥控远端完整操作台 |
| `mobile/rt-shell/`（本目录） | 机上 SDK + 示例原生 UI |
