# Google Play 上架资料（M3）

> 状态：**工程与文案已备，签名/提审需要 Play Console 账号。**  
> 目标 App：`mobile/rt-shell`（机上 Runtime 空壳）。

---

## 1. 基本信息

| 字段 | 建议值 |
|------|--------|
| App 名称 | Baize MOBILE-RT |
| 包名 | `com.rebornace.baize.mobilert` |
| 版本 | `1.0.0`（versionCode `1`） |
| 分类 | Productivity |
| 价格 | 免费 |
| 内容分级 | 4+ / Everyone（无用户生成内容展示、无广告） |
| 隐私政策 URL | 需发布公开页面（占位：`https://github.com/rebornace/baize/blob/main/docs/developers/mobile.md`） |
| 支持 URL | https://github.com/rebornace/baize |

---

## 2. 商店文案

### 中文

**短描述**

把智能体运行时放到你的 Android 设备上。

**完整描述**

Baize MOBILE-RT 把智能体运行时放到你的 Android 设备上。你配置一个兼容 OpenAI 的模型端点与 API Key，对话、工具调用与审批都在设备本地完成；密钥保存在 EncryptedSharedPreferences，不经过白泽服务器。

- 机上运行：不依赖常驻 `baize serve`
- 工具审批：需要写入或敏感调用时，先由你同意或拒绝
- 自带模型：兼容 OpenAI 的云端或本地端点
- 最小权限：无账号、无广告、无跨应用跟踪

适合希望把智能体能力嵌入自己 App、或在内网/本机模型上试验的开发者。

### English

**Short description**

Put an agent runtime on your Android device.

**Full description**

Baize MOBILE-RT puts an agent runtime on your Android device. Bring an OpenAI-compatible endpoint and API key; conversation, tool calls and approvals run on device. The key stays in EncryptedSharedPreferences and never goes through a Baize server.

- On-device runtime: no always-on `baize serve`
- Human approval before sensitive tool calls
- Bring your own cloud or local OpenAI-compatible endpoint
- Minimal permissions: no account, no ads, no cross-app tracking

For developers embedding agent capabilities or experimenting with local models.

---

## 3. Data safety（数据安全）

| 项 | 建议回答 |
|----|----------|
| 是否收集或共享数据 | 开发者不收集、不共享 |
| 用户内容 | 用户输入发送到**用户自己配置的**模型端点；App 不向开发者回传 |
| 密钥 | 仅保存在设备 EncryptedSharedPreferences |
| 分析 / 广告 / 跟踪 | 无 |
| 数据加密 | 传输取决于用户配置的端点；生产建议 HTTPS |
| 删除数据 | 清除 Key / 卸载 App 即可；无开发者侧账号数据 |

> 审核备注需写明：端点与 key 由用户配置，开发者不接收用户内容。默认 `http://10.0.2.2:18080/v1` 只是模拟器开发用；生产用户应配置 HTTPS。

---

## 4. 权限与网络

- 仅 `INTERNET`。
- 保留 `usesCleartextTraffic=true`：允许用户连接本地/内网 OpenAI 兼容端点；商店资料与审核备注明确建议生产端点使用 HTTPS。
- 不使用后台服务、定位、通讯录、相册、通知等权限。

---

## 5. 截图与素材

Play Console 需要：

| 素材 | 建议 |
|------|------|
| 手机截图 | 至少 2 张，竖屏 1080×1920 或 1440×2560；16:9 / 9:16，边长 320–3840 |
| 7 英寸平板 | 可选；当前只做手机 |
| Feature graphic | 1024×500 |
| App icon | 512×512（Play 会再裁切） |

建议画面：

1. 配置页（API base / model / Save vault）
2. 对话与回复
3. HITL 审批（Approve / Deny）
4. 远程配置拉取成功

> 截图需在模拟器或真机上真实截取；不要用伪造 UI。

---

## 6. 签名与提审清单

- [ ] Google Play Console 账号
- [ ] 注册包名 `com.rebornace.baize.mobilert`
- [ ] 创建上传密钥（推荐启用 Play App Signing）
- [ ] 复制 `keystore.properties.example` 为本地 `keystore.properties`，填真实路径/密码
- [ ] `python mobile/rt-shell/scripts/check-release.py`
- [ ] `.\mobile\rt-shell\scripts\archive-android.ps1 -Version 1.0.0 -VersionCode 1`
- [ ] Play Console 上传 `dist/android/BaizeMobileRT-1.0.0.aab`
- [ ] 填商店资料、Data safety、内容分级、隐私政策 URL
- [ ] 内部测试轨道：配置真实 HTTPS 端点，跑一次 Send/Approve/Deny
- [ ] 提交审核

`keystore.properties`、`.jks`、`.aab` 都不入库。

---

## 7. 已知阻塞

| 项 | 说明 |
|----|------|
| Play Console / 签名 | 需要账号、上传密钥与商店资料；仓库不包含 |
| 审核演示端点 | 需要可用的 HTTPS 端点与测试 key；由 App 所有者提供，不入库 |
| 真实截图 | 需要模拟器/真机；本仓库不伪造 |
| 隐私政策页面 | 需要公开 URL；当前仅占位 |
| targetSdk | 当前 35；Play 要求会更新，提审前按 Console 提示升级 |
| 机上工具面 | 当前只有 `get_time` / `http_get`；完整连接器/MCP 仍在桌面 Runtime |

---

## 8. 一句话

**图标、启动、Manifest、签名模板、AAB 脚本和商店文案已备；把账号、keystore、演示端点、截图和隐私政策 URL 补上，就能走内部测试。**
