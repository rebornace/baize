**中文** | [English](./mobile.en.md)

# 移动端

白泽有两条互不替代的移动相关路径：

| 路径 | 跑在哪 | 作用 |
|------|--------|------|
| **Capacitor / 浏览器壳**（本文主体） | 手机 WebView / 浏览器 | **遥控**已部署的旁挂 `baize serve`（控制台客户端） |
| **MOBILE-RT**（机上编排） | 用户设备内嵌的裁剪版 Go Agent | **分摊服务端**：Run 循环在设备上跑；默认设备直连 LLM API |

**MOBILE-RT** 规格（real，公开仓不同步）：[`../superpowers/specs/2026-10-23-mobile-rt-design.md`](../superpowers/specs/2026-10-23-mobile-rt-design.md)。**M0 已通**（薄包 + 桌面 demo）；**不是**把完整 `baize serve` 搬进 App。产品路径：可行后推进 **商店上架**（规格 M3）。

## 架构边界

| 层级 | 跑在哪 | 说明 |
|------|--------|------|
| Runtime（`baize serve`） | Linux / macOS / Windows 服务器或本机 | HTTP API、SSE、连接器、HITL、渠道；旁挂主线 |
| 控制台 UI（`web/chat`） | 浏览器，或 Capacitor WebView | 同源 `/ui` + `/v0`，或壳内配置 `BAIZE_API_ORIGIN` |
| gomobile（MOBILE-RT） | 公开包 `mobileagent/` → AAR / XCFramework | `Configure` / `Send` / HITL；AAR 在 `mobile/rt-shell/app/libs/`，iOS 在 `mobile/rt-shell-ios/Frameworks/`；**禁止** bind `./cmd/baize` |

```text
手机 Capacitor 壳  ──HTTPS──►  远端 baize（public_base_url / 局域网 IP）
       │                              │
       └──── 系统浏览器打开 ──────────┴── hosted login / OAuth
```

## 手机浏览器基线（先于壳）

在真机或 DevTools 设备模式访问 `https://<host>/ui/`（须已配置可从手机到达的地址，常见为 `runtime.public_base_url`）：

| 能力 | 预期 | 代码落点 |
|------|------|----------|
| 鉴权 | Gate 开启时 Bearer 存 `localStorage`；刷新后仍可用 | [`controlAuth.ts`](../../web/chat/src/controlAuth.ts)、[`GateRoot`](../../web/chat/src/pages/GateRoot.tsx) |
| SSE 流式 | 发消息后助手增量输出；断流可致命退出 | [`openRunStream`](../../web/chat/src/api/runs.ts)（`fetch` + `ReadableStream`，非 `EventSource`） |
| HITL | 审批卡可批准/拒绝；窄屏抽屉可关 | 聊天消息列表 + ≤768 抽屉（[`layout.css`](../../web/chat/src/styles/layout.css)） |
| Hosted login | 「浏览器登录」打开系统页；完成后回对话可续跑 | [`openExternal`](../../web/chat/src/openExternal.ts) → Capacitor Browser / `window.open` |
| 附件上传 | Composer / 技能上传走 `<input type="file">` | 系统文件选择器；iOS 需相册/文件权限（见下方 Capacitor） |

走查不通过时，优先修 **可达性与 CORS/混合内容**（手机访问 `http://127.0.0.1` 无效；用局域网 IP 或 HTTPS）。

## Capacitor 壳（`mobile/`）

仓库 [`mobile/`](../../mobile/) 用 Capacitor 包装同一套 `web/chat` 构建产物。

### 一次性准备

```powershell
cd mobile
copy .env.example .env
# 推荐：BAIZE_SERVER_URL=http://192.168.x.x:8080  （手机能访问的 baize，同源 /ui + SSE）
npm install
npm run sync
npm run add:android   # 或 add:ios（需 macOS + Xcode）
npm run open:android
```

### 环境变量

| 变量 | 作用 |
|------|------|
| `BAIZE_SERVER_URL` | **推荐**；WebView 直接加载远端 `/ui/`，与 API 同源，SSE/HITL 无需 CORS |
| `BAIZE_API_ORIGIN` | 打包进 www 的 `VITE_API_ORIGIN`（仅在不设 `BAIZE_SERVER_URL`、使用本地 www 时需要） |
| Runtime `BAIZE_CORS_ORIGINS` | 逗号分隔 Origin；捆绑 www 跨域调 API 时在 **baize 进程**上设置，例如 `https://localhost,capacitor://localhost`；`*` 仅建议开发用 |

嵌入服务端的默认构建 **不受影响**（`BAIZE_MOBILE` 未设时仍输出到 `internal/ui/dist`，`base=/ui/`）。

### WebView 适配（已做）

- **安全区**：`viewport-fit=cover`；移动顶栏 / Composer 使用 `env(safe-area-inset-*)`
- **键盘**：`interactive-widget=resizes-content`（Android Chrome）
- **深链登录 / OAuth**：原生平台走 `@capacitor/browser`
- **上传**：沿用隐藏 file input；Android/iOS 工程生成后按商店要求补权限说明

### 明确不做（Capacitor 壳路径）

- 用壳冒充「机上 Agent」（壳不跑本机 Engine；那是 MOBILE-RT）
- App 内嵌完整 `baize serve`、推送通知、React Native / Flutter 重写控制台

### MOBILE-RT（另路径，摘要）

- **API：** `mobileagent/` — `Configure` / `ChatOnce` / `Send` / `Approve` / `Deny`；实现 `internal/mobileagent`。  
- **M0：** 一轮无工具对话 + AAR。  
- **M1：** `get_time` + `http_get`；`Send`/`Approve`/`Deny`。  
- **M2：** Android `EncryptedSharedPreferences` 保险箱；`FetchRemoteConfig`；iOS Keychain 对位。重建 AAR：`scripts/build-mobileagent-aar.ps1`。  
- **iOS bind：** XCFramework + 空壳 `mobile/rt-shell-ios/`（Keychain）；重建：`./scripts/build-mobileagent-ios.sh`；证据 `docs/superpowers/notes/2026-10-23-mobile-rt-ios-bind-evidence.md`。  
- **其后：** M3 商店上架；公开 README 达标前不写承诺。

## 相关文档

- [部署](./deployment.md) — 服务端安装与 `public_base_url`
- [配置](./configuration.md) — Runtime 公开基址
- [架构](./architecture.md) — 运行时边界
