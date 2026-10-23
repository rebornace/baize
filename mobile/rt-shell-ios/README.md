# iOS 机上空壳（XCFramework）

机上精简编排：Go `mobileagent` → `gomobile bind` → `Frameworks/Mobileagent.xcframework` → 本空壳 App。**路径上无 `baize serve`。**

## 已产出（本机 Mac）

| 产物 | 路径 |
|------|------|
| XCFramework | [`Frameworks/Mobileagent.xcframework`](Frameworks/Mobileagent.xcframework)（ios-arm64 + ios-arm64_x86_64-simulator） |
| Xcode 工程 | [`BaizeMobileRT.xcodeproj`](BaizeMobileRT.xcodeproj) |
| 模拟器 Debug App | DerivedData `BaizeMobileRT.app`（本地构建；gitignore） |

ObjC / Swift API（与 Android 同包）：

```swift
MobileagentConfigure(apiBase, apiKey, model, &error)
MobileagentFetchRemoteConfig(configURL)
let json = MobileagentSend(userText)
// needs_approval → MobileagentApprove(callId) / MobileagentDeny(callId)
```

**密钥保险箱：** iOS 用 `CredentialStore`（Keychain，`com.rebornace.baize.mobilert`）；Go **不**把 key 写盘。  
**Android：** 对位 EncryptedSharedPreferences，见 [`../rt-shell/`](../rt-shell/)。

模拟器访问宿主机 mock：API base 用 `http://127.0.0.1:18080/v1`（先跑 `go run ./cmd/mobilebind/mockllm`）。

## 重建 XCFramework

```bash
# 需：macOS、Xcode、gomobile（go install …/gomobile@latest && gomobile init）
./scripts/build-mobileagent-ios.sh
```

绑定包为公开路径 `./mobileagent`（包装 `internal/mobileagent`；gomobile **不能** bind `internal/`）。

若 Xcode 工程丢失，可再生成：

```bash
python3 mobile/rt-shell-ios/scripts/gen-xcodeproj.py
```

## 构建 / 运行

```bash
cd mobile/rt-shell-ios
xcodebuild -project BaizeMobileRT.xcodeproj -scheme BaizeMobileRT \
  -destination 'generic/platform=iOS Simulator' \
  -configuration Debug CODE_SIGNING_ALLOWED=NO build
# 或 Xcode 打开 BaizeMobileRT.xcodeproj → 选模拟器 Run
```

真机需在 Xcode 里填 Development Team 并签名。

## 与 Capacitor `mobile/` 的关系

| 路径 | 作用 |
|------|------|
| `mobile/`（Capacitor） | 遥控远端 `baize serve` |
| `mobile/rt-shell/` | Android **机上** Run 循环 |
| `mobile/rt-shell-ios/`（本目录） | iOS **机上** Run 循环 |
