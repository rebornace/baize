# MOBILE-RT Android 空壳（真机 AAR）

机上编排：Go `mobileagent` → `gomobile bind` → `app/libs/mobileagent.aar` → 本空壳 App。**路径上无 `baize serve`。**

## 已产出（本机）

| 产物 | 路径 |
|------|------|
| AAR | [`app/libs/mobileagent.aar`](app/libs/mobileagent.aar)（含 arm64-v8a / armeabi-v7a / x86 / x86_64） |
| sources | `app/libs/mobileagent-sources.jar` |
| Debug APK | `app/build/outputs/apk/debug/app-debug.apk`（本地构建；gitignore） |

Java / Kotlin API（M2）：

```kotlin
mobileagent.Mobileagent.configure(apiBase, apiKey, model)
mobileagent.Mobileagent.fetchRemoteConfig(configUrl) // optional
val json = mobileagent.Mobileagent.send(userText)
// needs_approval → approve(callId) / deny(callId)
```

**密钥保险箱：** Android 用 `CredentialStore`（EncryptedSharedPreferences）；Go **不**把 key 写盘。  
**iOS：** 对位 Keychain，启动后注入 `configure`；`gomobile bind -target=ios` 须在 Mac 执行。

模拟器访问宿主机 mock：API base 用 `http://10.0.2.2:18080/v1`（先跑 `go run ./cmd/mobilebind/mockllm`）。

## 重建 AAR

```powershell
# 需：JDK 17+、ANDROID_HOME、NDK、gomobile（见 scripts）
.\scripts\build-mobileagent-aar.ps1
```

绑定包为公开路径 `./mobileagent`（包装 `internal/mobileagent`；gomobile **不能** bind `internal/`）。

## 构建 APK

```powershell
$env:JAVA_HOME = "D:\tools\jdk-17.0.13+11"   # 按本机调整
$env:ANDROID_HOME = "D:\android-sdk"
# 编辑 local.properties 的 sdk.dir，或复制示例
cd mobile\rt-shell
gradle assembleDebug   # 或 Android Studio Open 本目录
adb install -r app\build\outputs\apk\debug\app-debug.apk
```

## 与 Capacitor `mobile/` 的关系

| 路径 | 作用 |
|------|------|
| `mobile/`（Capacitor） | 遥控远端 `baize serve` |
| `mobile/rt-shell/`（本目录） | **机上** Run 循环（MOBILE-RT） |
