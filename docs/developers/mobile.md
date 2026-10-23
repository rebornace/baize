**中文** | [English](./mobile.en.md)

# 移动端：两套用法

| 你想要… | 用这条 | 界面谁做 |
|---------|--------|----------|
| 手机上用完整操作台（对话、设置、审批） | **A. 遥控旁挂服务** | 白泽自带 `/ui`（浏览器或 Capacitor 壳） |
| 自己的 App 界面，对话引擎跑在手机上 | **B. 机上 SDK** | **你自己画 UI**；调用 Go 编译出的 `mobileagent` |

两条路径互不替代。B **不是**把完整 `baize serve` 塞进 App：没有连接器农场、MCP、渠道、完整 HITL 产品面；有的是内存会话、兼容 OpenAI 的对话，以及少量工具（`get_time` / 需审批的 `http_get`）。

---

## A. 遥控旁挂服务（最快上手）

手机打开已部署的白泽操作台，或用 Capacitor 包一层 WebView。

**先确认手机浏览器能打开** `https://<可达主机>/ui/`（`runtime.public_base_url` 或局域网 IP；不要用手机访问电脑的 `127.0.0.1`）。鉴权、流式回复、审批卡、浏览器登录应可用。

再用 Capacitor 壳（仓库 [`mobile/`](../../mobile/)）：

```powershell
cd mobile
copy .env.example .env
# 推荐：BAIZE_SERVER_URL=http://192.168.x.x:8080
npm install
npm run sync
npm run add:android   # 或 add:ios（需 macOS）
npm run open:android
```

| 变量 | 作用 |
|------|------|
| `BAIZE_SERVER_URL` | WebView 直接加载远端 `/ui/`（同源，SSE/审批最省事） |
| `BAIZE_API_ORIGIN` | 仅在打本地 www、跨域调 API 时需要 |
| 旁挂进程 `BAIZE_CORS_ORIGINS` | 跨域时在 **baize** 上配置，如 `https://localhost,capacitor://localhost` |

壳已处理安全区、键盘、系统浏览器打开登录页。它**不**跑机上对话引擎。

服务端安装与公开基址：[部署](./deployment.md)、[配置](./configuration.md)。

---

## B. 机上 SDK：自定义界面

把编排编译进 App：你负责 Kotlin/Swift（或其它原生）界面；Go 包负责对话与工具循环。参考空壳（可当最小示例，也可只抄调用方式）：

- Android：[`mobile/rt-shell/`](../../mobile/rt-shell/)（依赖 `app/libs/mobileagent.aar`）
- iOS：[`mobile/rt-shell-ios/`](../../mobile/rt-shell-ios/)（依赖 `Frameworks/Mobileagent.xcframework`）

### 1. 编译绑定包

公开绑定入口是 [`mobileagent/`](../../mobileagent/)（gomobile **不能** bind `internal/`）。

```powershell
# Android AAR（需 JDK 17+、ANDROID_HOME、NDK、gomobile）
.\scripts\build-mobileagent-aar.ps1
# 产出：mobile/rt-shell/app/libs/mobileagent.aar
```

```bash
# iOS XCFramework（需 macOS、Xcode、gomobile）
./scripts/build-mobileagent-ios.sh
# 产出：mobile/rt-shell-ios/Frameworks/Mobileagent.xcframework
```

把 AAR / XCFramework 链进**你自己的**工程（Gradle `implementation files(...)` / Xcode Embed & Sign），不必使用空壳 App。

### 2. 调用约定（你画 UI）

**启动时配置一次**（兼容 OpenAI 的 `base` + key + model；key 只放内存，**不要**指望 Go 落盘）：

```kotlin
// Android（包名 mobileagent）
mobileagent.Mobileagent.configure(
  "https://api.example.com/v1",
  apiKeyFromYourVault,  // 建议用 EncryptedSharedPreferences 等由 App 保管
  "your-model"
)
```

```swift
// iOS
var err: NSError?
MobileagentConfigure("https://api.example.com/v1", apiKeyFromKeychain, "your-model", &err)
```

可选：从你们自己的配置 URL 拉取（JSON 含 `api_base` / `model`，可选 `api_key`）：

- `FetchRemoteConfig(url)` → 返回 JSON 状态字符串  
- 或 `ApplyRemoteConfigJSON(raw)`  

空壳里 Android `CredentialStore` / iOS Keychain 演示了「App 持钥、再 Configure」——自定义 UI 也应同样做。

**发消息（带工具循环）：**

```kotlin
val raw = mobileagent.Mobileagent.send(userText)
// 解析 raw（见下表），更新你的聊天气泡 / 审批卡片
```

```swift
let raw = MobileagentSend(userText)
```

**`Send` / `Approve` / `Deny` 返回的都是 JSON 字符串**，由你的界面解析：

| `status` | 含义 | 界面建议 |
|----------|------|----------|
| `done` | 本轮结束 | 展示字段 `text` |
| `needs_approval` | 待批工具（当前为 `http_get`） | 展示 `summary` / `name`；用户点批准 → `Approve(call_id)`，驳回 → `Deny(call_id)` |
| `error` | 失败 | 展示 `message` |

`needs_approval` 时还会有 `call_id`、`name`、`args_json`、`summary`。在用户点选之前不要再 `Send`。

```kotlin
when (val status = JSONObject(raw).getString("status")) {
  "done" -> showAssistant(JSONObject(raw).getString("text"))
  "needs_approval" -> showApproveCard(
    callId = JSONObject(raw).getString("call_id"),
    summary = JSONObject(raw).optString("summary")
  )
  "error" -> showError(JSONObject(raw).getString("message"))
}
// 用户批准：
val cont = mobileagent.Mobileagent.approve(callId)
// 再按同样方式解析 cont
```

其它常用 API：

| API | 作用 |
|-----|------|
| `ChatOnce(text)` | 单轮、**不走工具**（快速试连通） |
| `Reset()` | 清空内存历史（保留已 Configure 的地址/模型） |
| `MessageCount()` | 当前会话消息条数 |
| `HasAPIKey()` | 内存里是否已有 key |
| `NewAgent()` | 独立会话（多会话时用实例方法，而不是包级默认会话） |

桌面可先不绑手机：`go run ./cmd/mobilebind`；本地假模型：`go run ./cmd/mobilebind/mockllm`（模拟器里 Android 访问宿主机常用 `http://10.0.2.2:端口/v1`）。

### 3. 你负责 / SDK 负责

| 你（App） | `mobileagent` |
|-----------|----------------|
| 布局、主题、历史列表、输入框、审批卡片 | `Configure` / `Send` / `Approve` / `Deny` |
| 安全存 key、远程配置拉取策略 | 内存里的会话与工具循环 |
| 线程：网络调用勿堵主线程 | 返回 JSON，不碰 UI |

当前工具面：`get_time`（直接执行）；`http_get`（先 `needs_approval`）。没有完整旁挂上的连接器 / MCP / 渠道。

---

## 怎么选

- 要完整设置页、连接器、运营审批流程 → **路径 A**。  
- 要品牌化原生界面、对话在设备上跑、少依赖常驻旁挂 → **路径 B**，以 `rt-shell` 为调用样板自己画 UI。  
- 需要两者：运营用 A；面向终端用户的 App 用 B。

## 相关文档

- [部署](./deployment.md) · [配置](./configuration.md) · [架构](./architecture.md)
- Android 空壳细节：[`mobile/rt-shell/README.md`](../../mobile/rt-shell/README.md)
- iOS 空壳细节：[`mobile/rt-shell-ios/README.md`](../../mobile/rt-shell-ios/README.md)
