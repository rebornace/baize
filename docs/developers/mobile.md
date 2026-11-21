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

远程配置也可以带一个 **OpenAPI 连接器**。默认只解析 GET；连接器设置 `"allow_writes": true` 后才解析 POST/PUT/PATCH（**DELETE 不支持**）：

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
    "allow_writes": true
  }
}
```

也可用 `"spec_url": "https://.../openapi.yaml"` 代替 `spec`（`FetchRemoteConfig` 会代下载）。无论读写，每次连接器调用都先返回 `needs_approval`，用户同意后才发请求。写操作额外约束：

- 只支持 `application/json` request body；对象 schema 展开为工具参数，数组 / 标量 schema 用单个 `body` 参数传完整 JSON。
- 请求体上限 64 KiB；不跟随 3xx 重定向（直接返回状态，避免请求体或认证头被转到别处）。
- path / query 参数照常支持；写工具与读工具共用全局唯一工具名。

需要多个连接器时用 `readonly_apis` 数组（与 `readonly_api` 互斥；每个连接器 `spec` / `spec_url` 二选一，并各自决定是否 `allow_writes`）：

```json
"readonly_apis": [
  {
    "id": "tickets",
    "base_url": "https://tickets.example.com",
    "auth_header": "Authorization",
    "auth_value": "Bearer ...",
    "spec": "openapi: 3.0.3 ...",
    "allow_writes": true
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

工具名必须在所有连接器与内置工具之间唯一；最多 8 个连接器、100 个工具。SDK 也可直接调用 `ConfigureReadOnlyAPIs(json)`（JSON 数组，字段同上）。单连接器 API `ConfigureReadOnlyAPI(...)` 始终只读；需要写操作时用 JSON 形式并设置 `allow_writes`。

当前仓库中的 Android AAR 已包含新 API；iOS XCFramework 需要在 macOS 上重跑 `./scripts/build-mobileagent-ios.sh` 后才包含。

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
| `needs_approval` | 待批工具（`http_get` 或连接器调用，含写操作） | 展示 `summary` / `name` / `method`；写操作还带 `write: true`。用户点批准 → `Approve(call_id)`，驳回 → `Deny(call_id)` |
| `error` | 失败 | 展示 `message` |

`needs_approval` 时还会有 `call_id`、`name`、`args_json`、`summary`、`method`。写操作的 `summary` 形如 `POST /tickets {"title":"VPN 故障"}`，可直接放进审批卡。在用户点选之前不要再 `Send`。

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
| `ConfigureReadOnlyAPI(base, authHeader, authValue, spec)` | 配置一个 GET-only OpenAPI 连接器；失败保留旧工具 |
| `ConfigureReadOnlyAPIs(json)` | 用 JSON 数组一次配置多个连接器；`allow_writes` 可选；工具名全局唯一，最多 8 个连接器 |
| `ClearReadOnlyAPI()` | 清空连接器工具与待批远程调用 |
| `RemoteToolCount()` | 当前连接器工具数 |
| `RemoteWriteToolCount()` | 其中 POST/PUT/PATCH 写工具数 |
| `RemoteToolNames()` | 当前工具名（逗号分隔；空串表示没有） |
| `NewAgent()` | 独立会话（多会话时用实例方法，而不是包级默认会话） |

桌面可先不绑手机：`go run ./cmd/mobilebind`；本地假模型：`go run ./cmd/mobilebind/mockllm`（模拟器里 Android 访问宿主机常用 `http://10.0.2.2:端口/v1`）。

**本地演示连接器：** 再开一个进程（同一进程同时提供 config、OpenAPI spec 和演示票据 API，含一个 POST 写操作）：

```powershell
go run ./cmd/mobilebind/demo-connector -addr 127.0.0.1:18081
```

空壳里的 Remote config URL 填：

- Android 模拟器：`http://10.0.2.2:18081/config.json`
- iOS 模拟器：`http://127.0.0.1:18081/config.json`
- 真机：用 `-addr 0.0.0.0:18081`，填电脑的局域网 IP

点 `Fetch config` 后界面应显示 `Tools: list_tickets, create_ticket, get_ticket`（Android 空壳在有写工具时追加 `(1 write)`）；让模型列出工单时会出现 `GET /tickets?...` 审批卡，创建工单时会出现 `POST /tickets {"title":…}` 审批卡（带 `write: true`），同意后才发请求。演示头 `X-API-Key: demo-key` 只用于本机，生产必须换成自己的认证。

空壳启动时若已保存过 Remote config URL，会自动重拉一次，无需再点按钮；网络失败则静默保留本地 LLM 配置（工具为空），下次启动再试。

### 3. 你负责 / SDK 负责

| 你（App） | `mobileagent` |
|-----------|----------------|
| 布局、主题、历史列表、输入框、审批卡片 | `Configure` / `Send` / `Approve` / `Deny` |
| 安全存 key、远程配置拉取策略 | 内存里的会话与工具循环 |
| 线程：网络调用勿堵主线程 | 返回 JSON，不碰 UI |

当前工具面：`get_time`（直接执行）；`http_get`（先 `needs_approval`）；最多 8 个 OpenAPI 连接器（GET 默认解析，设置 `allow_writes` 后增加 POST/PUT/PATCH；每次调用都先 `needs_approval`，支持 path/query/JSON body 与每个连接器一个自定义认证头）。没有完整桌面连接器 / MCP / 渠道。

### 4. iOS 上架（M3）

- 工程：[`mobile/rt-shell-ios/`](../../mobile/rt-shell-ios/)；已含 AppIcon、AccentColor、启动背景、`PrivacyInfo.xcprivacy`、iPhone-only 设置。
- 校验：`python mobile/rt-shell-ios/scripts/check-release.py`
- 归档：`DEVELOPMENT_TEAM=<team> ./mobile/rt-shell-ios/scripts/archive-ios.sh`
- 商店文案 / 隐私回答 / 审核备注 / 截图尺寸 / 提审清单：[`mobile/rt-shell-ios/AppStore/README.md`](../../mobile/rt-shell-ios/AppStore/README.md)

签名、TestFlight、App Review 需要 Apple Developer 账号；仓库不包含证书、Team ID 或审核用演示 key。

### 5. Android 上架（M3）

- 工程：[`mobile/rt-shell/`](../../mobile/rt-shell/)；已含自适应/传统图标、启动背景、Manifest、版本 1.0.0、Gradle Wrapper、签名模板。
- 校验：`python mobile/rt-shell/scripts/check-release.py`
- 归档：复制 `keystore.properties.example` 并填本地签名，再运行 `.\mobile\rt-shell\scripts\archive-android.ps1`
- 商店文案 / Data safety / 截图尺寸 / 提审清单：[`mobile/rt-shell/PlayStore/README.md`](../../mobile/rt-shell/PlayStore/README.md)

Play Console、上传密钥与提审需要账号；仓库不包含 keystore、密码或审核用演示 key。

---

## 怎么选

- 要完整设置页、连接器、运营审批流程 → **路径 A**。  
- 要品牌化原生界面、对话在设备上跑、少依赖常驻旁挂 → **路径 B**，以 `rt-shell` 为调用样板自己画 UI。  
- 需要两者：运营用 A；面向终端用户的 App 用 B。

## 相关文档

- [部署](./deployment.md) · [配置](./configuration.md) · [架构](./architecture.md)
- Android 空壳细节：[`mobile/rt-shell/README.md`](../../mobile/rt-shell/README.md)
- iOS 空壳细节：[`mobile/rt-shell-ios/README.md`](../../mobile/rt-shell-ios/README.md)
