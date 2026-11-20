# iOS 机上空壳（示例 App）

演示如何把 `Mobileagent.xcframework` 嵌进原生界面：**你画 UI，SDK 跑对话**。完整步骤与 JSON 约定见：

→ [docs/developers/mobile.md](../../docs/developers/mobile.md)（「B. 机上 SDK」）· [EN](../../docs/developers/mobile.en.md)

本目录是最小可运行样板；可只嵌入 Framework，换成自己的 SwiftUI / UIKit。

## 产物

| 产物 | 路径 |
|------|------|
| XCFramework | [`Frameworks/Mobileagent.xcframework`](Frameworks/Mobileagent.xcframework) |
| Xcode 工程 | [`BaizeMobileRT.xcodeproj`](BaizeMobileRT.xcodeproj) |

## 调用摘录

```swift
var err: NSError?
MobileagentConfigure(apiBase, apiKey, model, &err)
MobileagentFetchRemoteConfig(configURL) // optional
let json = MobileagentSend(userText)
// status=needs_approval → MobileagentApprove(callId) / MobileagentDeny(callId)
```

密钥由 App 侧 `CredentialStore`（Keychain）保管。Android 对位见 [`../rt-shell/`](../rt-shell/)。

模拟器访问宿主机 mock：`http://127.0.0.1:18080/v1`（先 `go run ./cmd/mobilebind/mockllm`）。

## M3 上架

已备：AppIcon / AccentColor / LaunchBackground、`PrivacyInfo.xcprivacy`、iPhone-only 工程、`ExportOptions.plist`、归档脚本与商店资料。

```bash
python mobile/rt-shell-ios/scripts/check-release.py
DEVELOPMENT_TEAM=<team> ./mobile/rt-shell-ios/scripts/archive-ios.sh
```

签名、TestFlight 与 App Review 需要 Apple Developer 账号；审核演示端点、真实截图与隐私政策 URL 由 App 所有者补齐。商店文案与提审清单见 [`AppStore/README.md`](AppStore/README.md)。

## 重建

```bash
./scripts/build-mobileagent-ios.sh
# 若工程丢失：python3 mobile/rt-shell-ios/scripts/gen-xcodeproj.py
```

| 路径 | 作用 |
|------|------|
| `mobile/`（Capacitor） | 遥控远端完整操作台 |
| `mobile/rt-shell-ios/`（本目录） | 机上 SDK + 示例原生 UI |
