**中文** | [English](./mobile.en.md)

# 移动端（客户端壳）

白泽是 **旁挂在服务端的 Agent Runtime**。本文描述的 Capacitor / 浏览器路径是 **控制台客户端**（连已部署实例）。

**机上嵌入裁剪版 Go Agent（MOBILE-RT）** 已录入规划、**暂缓开刀**（大工程；仅壳遥控对「手机自带 Agent」意义有限）。需求在 `baize_real`：`docs/superpowers/notes/2026-10-22-mobile-on-device-runtime.md`（公开仓不同步）。

## 架构边界

| 层级 | 跑在哪 | 说明 |
|------|--------|------|
| Runtime（`baize serve`） | Linux / macOS / Windows 服务器或本机 | HTTP API、SSE、连接器、HITL、渠道 |
| 控制台 UI（`web/chat`） | 浏览器，或 Capacitor WebView | 同源 `/ui` + `/v0`，或壳内配置 `BAIZE_API_ORIGIN` |
| gomobile | **不采用** | 适合极薄 Go 库绑定；无法替代长驻 Runtime + React 控制台 |

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

### 明确不做（本阶段）

- 用 gomobile / `GOOS=android` 编译 `./cmd/baize` 上架
- App 内嵌完整 Runtime、离线 Agent、推送通知
- React Native / Flutter 重写控制台

## 相关文档

- [部署](./deployment.md) — 服务端安装与 `public_base_url`
- [配置](./configuration.md) — Runtime 公开基址
- [架构](./architecture.md) — 运行时边界
