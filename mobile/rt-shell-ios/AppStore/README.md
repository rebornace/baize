# App Store 上架资料（M3）

> 状态：**工程与文案已备，签名/提审需要 Apple Developer 账号。**  
> 目标 App：`mobile/rt-shell-ios`（机上 Runtime 空壳）。

---

## 1. 基本信息

| 字段 | 建议值 |
|------|--------|
| App 名称 | Baize MOBILE-RT |
| 副标题 | On-device agent runtime |
| Bundle ID | `com.rebornace.baize.mobilert` |
| SKU | `baize-mobile-rt-1` |
| 主分类 | Productivity |
| 价格 | Free |
| 年龄分级 | 4+（无用户生成内容展示、无广告） |
| 支持 URL | https://github.com/rebornace/baize |
| 隐私政策 URL | 需发布一个公开页面（占位：`https://github.com/rebornace/baize/blob/main/docs/developers/mobile.md`） |
| 版权 | Copyright © 2026 rebornace |

---

## 2. 宣传文案

### 中文

**描述**

Baize MOBILE-RT 把智能体运行时放到你的 iPhone 上。你配置一个兼容 OpenAI 的模型端点与 API Key，对话、工具调用与审批都在设备本地完成；密钥保存在 Keychain，不经过白泽服务器。

- 机上运行：不依赖常驻 `baize serve`
- 工具审批：需要写入或敏感调用时，先由你同意或拒绝
- 自带模型：兼容 OpenAI 的云端或本地端点
- 最小权限：无账号、无广告、无跨应用跟踪

适合希望把智能体能力嵌入自己 App、或在内网/本机模型上试验的开发者。

**关键词**

agent, runtime, on-device, OpenAI, tool calling, HITL, developer

### English

**Description**

Baize MOBILE-RT puts an agent runtime on your iPhone. Bring an OpenAI-compatible endpoint and API key; conversation, tool calls and approvals run on device. The key stays in Keychain and never goes through a Baize server.

- On-device runtime: no always-on `baize serve`
- Human approval before sensitive tool calls
- Bring your own cloud or local OpenAI-compatible endpoint
- Minimal permissions: no account, no ads, no cross-app tracking

For developers embedding agent capabilities or experimenting with local models.

**Keywords**

agent, runtime, on-device, OpenAI, tool calling, HITL, developer

---

## 3. App 隐私

| 项 | 建议回答 |
|----|----------|
| 是否跟踪 | 否 |
| 是否收集数据 | 开发者不收集 |
| 用户内容 | 用户输入发送到**用户自己配置的**模型端点；App 不向开发者回传 |
| 密钥 | 仅保存在设备 Keychain |
| 分析/广告 | 无 |
| 所需理由 API | 无（未使用 UserDefaults/文件时间戳等） |

> App Review 可能追问“用户内容发送到第三方模型”。审核备注需写明：端点由用户配置，开发者不接收数据。

---

## 4. 审核备注（Review Notes）

```text
This app is an on-device agent runtime for developers.
It requires the user to enter an OpenAI-compatible API base URL, API key and model.
No account or in-app purchase is required.
Tool calls that may write or touch sensitive systems stop for an explicit
Approve/Deny decision before execution.
The API key is stored only in the iOS Keychain.
The default endpoint is a local mock used for development; production users
must configure their own HTTPS endpoint.
Local Network permission is used only when the user points the app at a
local/on-prem model. Production endpoints are HTTPS.
No analytics, ads or cross-app tracking.
Demo endpoint/key for review: <TO BE PROVIDED BY THE APP OWNER — never commit>.
```

---

## 5. 截图

需要 iPhone 截图（App Store Connect 当前要求）：

| 尺寸 | 用途 |
|------|------|
| 1320 × 2868 | iPhone 6.9"（必需） |
| 1284 × 2778 | iPhone 6.5"（兼容） |
| 1242 × 2208 | iPhone 5.5"（可选/兼容） |

建议画面：

1. 配置页（API base / model / Save vault）
2. 对话与回复
3. HITL 审批（Approve / Deny）
4. 远程配置拉取成功

> 截图需在 macOS + iOS Simulator 上真实截取；不要用伪造 UI。

---

## 6. 签名与提审清单

- [ ] Apple Developer Program 账号；Team ID
- [ ] 在 Certificates, Identifiers & Profiles 注册 `com.rebornace.baize.mobilert`
- [ ] Xcode 打开 `mobile/rt-shell-ios/BaizeMobileRT.xcodeproj`，Team 选择自己的 Team
- [ ] 确认 Signing & Capabilities 为 Automatic；Keychain 无需额外 capability
- [ ] `python mobile/rt-shell-ios/scripts/check-release.py`
- [ ] `DEVELOPMENT_TEAM=<team> ./mobile/rt-shell-ios/scripts/archive-ios.sh`
- [ ] 用 Transporter 或 `xcrun altool` 上传 `.ipa`
- [ ] App Store Connect 填基本信息、隐私、审核备注、截图
- [ ] TestFlight 内部测试：配置真实 HTTPS 端点、跑一次 Send/Approve/Deny
- [ ] 提交审核

---

## 7. 已知阻塞

| 项 | 说明 |
|----|------|
| 签名/Team | 需要 Apple Developer 账号；本仓库不包含证书或 Team ID |
| 审核演示端点 | App Review 需要可用端点与测试 key；由 App 所有者提供，不入库 |
| 真实截图 | 需要 macOS/Simulator；本仓库不伪造 |
| 隐私政策页面 | 需要一个公开 URL；当前仅占位 |
| 机上工具面 | 当前只有 `get_time` / `http_get`；完整连接器/MCP 仍在桌面 Runtime |

---

## 8. 一句话

**工程、图标、隐私清单、归档脚本和商店文案已备；把 Team、演示端点、截图和隐私政策 URL 补上，就能走 TestFlight。**
