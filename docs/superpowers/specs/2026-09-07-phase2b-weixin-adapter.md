# 阶段二 2B 设计：微信适配器迁移与跨平台子进程托管

> 状态：设计稿（仅私有仓 docs/superpowers，不导出公开仓）
> 日期：2026-09-07
> 前置：阶段一「渠道注册表化」、阶段二 2A「通用 webhook 渠道」均已合并并推送双仓。
> 关联：`docs/superpowers/specs/2026-09-06-phase2-webhook-channel-design.md`（阶段二总设计，§9b 为本节雏形）；`docs/superpowers/specs/2026-08-29-channel-weixin-v0-design.md`（微信 v0）。

## 0. 终态与最高约束（铁律）

**终态：只有一套渠道机制。** 微信从"进程内渠道"变为"通用 webhook 实例 + 独立适配器进程 `weixin-adapter`"。2B 同一发布内：

- 新增适配器 `cmd/weixin-adapter`（同仓、跨平台 Go 二进制），承载 iLink 私有协议（扫码登录、长轮询、发消息、媒体下载+**AES 解密**、媒体**上传真发图/文件**、凭据持久化；媒体见 §15）。
- baize 核心**删除进程内 weixin 渠道的注册/装配/专用管理 handler**；微信改为装配一个 `type: webhook`、`source: weixin` 的实例对接适配器。
- 飞书/钉钉/Slack 与微信机制完全平权，只是适配器进程不同。

**决策（已与用户确认）：**
1. **同一发布内大切换（big-bang cutover）**：不长期保留进程内 weixin 作为回退开关；靠"微信功能对等回归清单"（§8）逐项测试全绿兜底。旧代码在 2B 收尾删除。
2. **子进程托管纳入 2B，且必须跨平台**（Linux / macOS / Windows）：默认部署"装 baize 即用微信"，baize 自动 `os/exec` 拉起适配器、健康检查、关停；适配器也可独立部署（`adapter_autostart: false`）。

**微信功能零丢失、零回归（最高优先级）。** §8 回归清单逐项必须有对等测试，不允许"迁完发现功能没了"。

## 1. 目标与非目标

**目标：**
- 微信运行在适配器进程上，经 2A 的 webhook 渠道接入；扫码登录/登出/启停/登录态回显/白名单/收发消息/媒体/context_token/HITL/UI 镜像**逐项功能对等**。
- baize 核心无进程内 weixin 特例：`internal/channel/weixin` 不再注册/装配；微信专用管理 handler 由通用渠道管理代理取代，**管理 URL 与前端页面、JSON 字段不变**。
- 默认部署（autostart）行为与现在等价：**现有 `data/channels/weixin/creds.json` 凭据免重新扫码**。
- 适配器子进程托管跨平台可用（Linux/mac/Windows）。
- `go test ./...`（含 integration）全绿、gofmt/vet 干净。

**非目标（YAGNI）：**
- 不重写 iLink 文本/登录/轮询报文；适配器复用现有 ilink 客户端代码（搬迁而非重写）。
- **媒体升级（用户确认纳入 2B）**：出站真发图/文件、入站 CDN AES 解密（见 §15）；这超出"纯迁移对等"，是适配器侧新增能力。
- 不做语音/视频消息（iLink media_type 2/5）；本期只做图片（type 2）与文件（type 4）。
- 不做适配器市场/远程安装/多适配器编排；一个 webhook 实例对接一个适配器进程。
- 不把适配器拆成独立仓库（同仓 `cmd/weixin-adapter`，复用内部 ilink 库）。

## 2. 总体架构

```
微信用户 ⇄ iLink 后端 ⇄ weixin-adapter 进程（cmd/weixin-adapter）
                              │  (A) 入站：签名 POST /v0/channels/weixin/inbound
                              │  (B) 出站：baize 签名 POST 适配器 /outbound
                              │  (C) 管理：baize 控制面 ⇄ 适配器 /admin/*（HMAC）
                              ▼
                     baize 核心：webhook 渠道实例（source=weixin）
                     ├─ 入站验签/幂等/通用白名单 → Runtime.HandleInbound → 引擎
                     ├─ 出站 Deliver* → SendText/SendMedia → 适配器 /outbound
                     └─ 通用渠道管理代理 /v0/settings/channels/weixin/*
                            ├─ 通用设置（assignee/agent_id/allowlist/enabled）→ baize 渠道设置存储
                            └─ 登录/登出/状态 → 转发适配器 /admin/*
                     baize 装配期（adapter_autostart:true）：
                     os/exec 拉起 weixin-adapter 子进程 → 轮询 /healthz 就绪
                     → 关停时终止子进程（跨平台，纳入 closer LIFO）
```

组件：
- **`cmd/weixin-adapter/`**（新）：独立 main。迁入 iLink 客户端 + 长轮询 + 扫码登录 + 媒体下载 + 凭据持久化（复用，见 §4）；实现 HTTP 服务：`/outbound`（收 baize 出站→ilink SendMessage）、`/admin/*`（登录/状态/登出/启停）、`/healthz`；长轮询 goroutine 把消息组装为入站 payload 签名 POST 给 baize。
- **`internal/channel/webhook/`**（2A 已有，2B 增强）：动态 account（§5）、通用渠道设置热更新（§6）、管理面代理客户端（§7）、子进程托管（§9）。
- **`internal/api/`**：通用渠道管理路由 `/v0/settings/channels/{name}/...` 取代微信专用 handler（§7）。
- **`internal/channel/weixin/`**：从"渠道实现"降级为"适配器可复用库"——移除 `init()` 注册与 `Bootstrap`/渠道装配相关；ilink 客户端/凭据/settings 逻辑供 `cmd/weixin-adapter` 使用（包保留或移至 `cmd/weixin-adapter/internal/`，见 §4 取舍）。

## 3. 职责划分（迁移后）

| 能力 | 现状（进程内 weixin） | 2B 后归属 |
|---|---|---|
| iLink 扫码登录 / 轮询登录态 / 凭据持久化 | `weixin/client.go` + `creds.go` | **适配器** `/admin/login/*` + 适配器 creds 目录 |
| 长轮询收消息 / 调 iLink 发消息 / CDN 下载 | `weixin` 轮询 goroutine + client | **适配器**（入站 POST baize；出站收 `/outbound`） |
| 登录态 `running/reason/login_required` 推导 | api 层 `weixinRuntimeState` | **baize 通用代理**：合并适配器 `/admin/status` + 本地 enable/settings 态（§7） |
| 白名单入站强制 | `weixin.Channel.peerAllowed` | **baize webhook 入站 handler 通用执行**（按实例 allowlist） |
| allowlist / assignee / agent_id / enabled 热更新 | `settings.json` + api 内存协调 | **baize 通用渠道设置存储**（§6）；enabled=false 通知适配器停轮询 |
| 群聊 `@chatroom` 丢弃 | `weixin.isGroupPeer` | **适配器**（IM 专有过滤，转发前丢弃；baize 不感知微信群概念） |
| convID `weixin:<account>:<peer>` / meta.Source | runtime + weixin accountID | **baize**，account 取适配器入站上报的动态 account（§5） |
| 【客服】/【助手】前缀、HITL、UI 镜像、context_token | 通用 `channel` 包 | **不变**（本就在核心，webhook 渠道复用） |
| 入站媒体→多模态/附件 | runtime `buildInboundContent` | **不变**（适配器下载后 base64/URL 上报，baize 复用 2A 附件解析） |

## 4. 适配器代码复用与搬迁

**原则：搬迁/复用，不重写。** iLink 协议代码已经过测试（`client_test.go` 协议级、`fake.go`），2B 把它从"渠道包"变为"适配器内部库"。

- 复用文件（原样迁入适配器，含测试）：`ilink.go`（接口+wire 类型+常量）、`client.go`（HTTP 客户端）、`creds.go`（凭据原子读写）、`fake.go`（测试 ILlink）。
- 适配器新增：`main.go`（HTTP 服务 + 长轮询 goroutine + 信号处理）、`inbound.go`（轮询→入站 payload→签名 POST baize）、`outbound.go`（`/outbound` 验签→ilink SendMessage）、`admin.go`（`/admin/*` 扫码/状态/登出/启停）、`config.go`（适配器自身配置：baize URL、secret、监听端口、creds 目录、base_url）。
- **包位置取舍：** 采用 **`cmd/weixin-adapter/internal/weixinlink/`**（把 ilink/client/creds/fake 迁入适配器私有 internal 库）。理由：核心不再 import 这些微信专有代码，依赖方向清晰（适配器→自己的 internal 库），核心 `internal/channel/weixin/` 整个删除，终态干净。迁移时保留 `client_test.go`/`fake.go` 作为适配器库测试。
- **媒体新增（适配器库 `weixinlink`）**：新增 `media.go`——AES-128-ECB/PKCS7 加解密 + `GetUploadURL`/CDN 上传/`SendImage`/`SendFile`（出站真发），并把 `DownloadMedia` 下载后解密（入站，见 §15）；`ilink.go` 接口补 `UploadMedia`/`SendMediaItem` 方法，`fake.go` 同步。
- **凭据目录兼容：** 适配器默认 creds 目录沿用 `./data/channels/weixin`（与现状同路径），`creds.json` 格式不变（`{account_id, token}`），实现"现有凭据免重新扫码"。适配器由 baize 以相同工作目录拉起时即读到旧凭据。
- **settings.json 不再由适配器管理**：`agent_id/allowlist/assignee/enabled` 上移到 baize 通用渠道设置（§6）；适配器只持有连接态凭据（creds.json）与运行态（轮询启停、登录态）。适配器 `/admin/start|stop` 对应轮询 goroutine 启停；`enabled` 由 baize 在设置变更时调用。

## 5. 动态 account（关键，防历史会话失联）

微信 convID 第二段是扫码登录后才确定的 `accountID`（iLink `ilink_bot_id`），**不是静态配置值**。2A 的 webhook 渠道用静态 config account、忽略入站 account；2B 必须改为动态：

**入站（适配器→baize）：**
- 适配器在每条入站 payload 带 `"account": "<ilink_bot_id>"`（登录态已知）。
- webhook 入站 handler：`account` 取**入站 payload 非空值**，回落实例配置 `account`（2A 行为，供无动态 account 的第三方适配器）。即 `acct = orDefault(msg.Account, c.cfg.Account)`，用它拼 `ConvID(source, acct, peer)` 与 `extras["account"]`。
- 适配器登录成功后 account 才稳定；登录前无入站消息，故不影响。

**出站（baize→适配器）：**
- convID 含 account（`weixin:<acct>:<peer>`）。出站 body 已带 `account` 字段（2A 固定用 `c.cfg.Account`）。
- **机制（稳健，不依赖 context_token）：** 因"一个 webhook 实例 = 一个 IM 账号"是设计不变量，webhook 渠道从入站消息缓存该实例的 `activeAccount`（入站 handler 见到非空 `msg.Account` 即记下，带 RWMutex）。任何出站都发生在某会话有过入站之后（渠道会话由入站 EnsureMeta 建立），故 `activeAccount` 在首次出站前必然已知。
- `SendText/SendMedia` 解析 account 顺序：`extras["account"]`（若 `Runtime.OutboundExtras` 按 convID 回填，精确到会话）→ 实例 `activeAccount`（入站学到）→ `c.cfg.Account`（静态配置，2A 行为/第三方无动态 account 时）。据此拼 `ConvID(source, acct, peerID)` 并填出站 body `account`。
- 新增 `channel.AccountFromConvID(convID) string` helper（与 `SourceFromConvID` 对称，取 `source:account:peer` 第二段）；`Runtime.OutboundExtras` 在现有 context_token 外顺带回填 `account`（有缓存条目时），作为精确路径。
- weixin 旧渠道只读 `extras["context_token"]`，新增 account 键被忽略，无副作用（该包 2B 删除）。

**历史兼容：** 历史会话 convID 为 `weixin:<旧 accountID>:<peer>`，适配器登录后上报相同 accountID（同一微信号 ilink_bot_id 稳定），故 meta.Source=`weixin` + account 匹配，历史会话不失联、不串号。

## 6. 通用渠道设置热更新（baize 侧）

webhook 实例需要一层可热更新、可持久化的渠道设置（取代微信专用 `settings.json` + `applyWeixinSettings`），且对所有 webhook 实例通用：

- **设置 DTO（每实例持久化）**：`{assignee, agent_id, allowlist []string, enabled bool}`，存于 baize 数据目录（如 `data/channels/webhook/<name>/settings.json`，原子写，复用现有 creds/settings 持久化模式）。
- **装配期**：webhook 实例 Bootstrap 时 `LoadSettings`：`assignee/agent_id` 回落实例 config（config 为基线，settings 可覆盖）；`allowlist` 决定入站白名单；`enabled=false` 时不启动适配器轮询（通知适配器 stop / 不拉子进程）。
- **热更新**：通用渠道设置 PUT → 落盘 + 立即内存生效（不重启）：
  - `assignee/agent_id` → 更新 `Runtime.Assignee/DefaultAgentID`（下一条入站生效）。
  - `allowlist` → 更新 webhook 入站 handler 的白名单 map（2A 已有白名单过滤，改为可热更新的实例字段 + RWMutex）。
  - `enabled:true` → 确保适配器轮询运行（autostart 则确保子进程在跑 + 调 `/admin/start`；独立部署则调 `/admin/start`）；`enabled:false` → 调 `/admin/stop`（保凭据，与微信现状"停用保凭据"一致）。
- **白名单语义对等**：空 allowlist = 放行全部；非空则非白名单 peer 在入站静默丢弃（2A 已实现，改为读热更新字段）。群聊由适配器丢弃，不依赖白名单。

## 7. 渠道管理面：通用代理协议

取代微信专用 5 条路由。baize 在控制面鉴权（**RoleAdmin**，与现状一致）下提供通用渠道管理路由，按 `{name}` 找渠道句柄：

**baize 本地处理（通用设置，不转发适配器）：**
- `GET /v0/settings/channels/{name}` → 返回 `{agent_id, allowlist, assignee, enabled, running, reason}`。
- `PUT /v0/settings/channels/{name}` → 写通用设置（§6 热更新），返回同 GET。

**转发适配器（webhook 实例配置了 `admin_url` 时，baize 加 HMAC 头代理到适配器 `/admin/*`）：**

| baize 路由 | 适配器端点 | 说明 |
|---|---|---|
| `POST .../{name}/login/start` | `POST /admin/login/start` | 返回 `{ticket, qr_url}`（适配器调 iLink GetQR） |
| `GET .../{name}/login/status?ticket=` | `GET /admin/login/status?ticket=` | `{status: pending/success/expired}`；success 时适配器落凭据 |
| `POST .../{name}/logout` | `POST /admin/logout` | 适配器清凭据、停轮询 |
| （启停内含在 PUT enabled） | `POST /admin/start` `/admin/stop` | baize 在 enabled 变更/装配时调用 |

- **`running/reason` 合并推导（baize 侧，对等 `weixinRuntimeState`）：**
  - 适配器 `/admin/status` 返回 `{has_credentials, polling, account_id?}`（HMAC）。
  - baize 合并：`enabled=false` → `running:false, reason:""`（有意停用）；否则 `!has_credentials` → `running:false, reason:"login_required"`；`has_credentials && !polling`（含适配器不可达/admin 调用失败）→ `running:false, reason:"start_failed"`；`polling` → `running:true`。
  - 适配器不可达（admin 调用失败）保守归入 `start_failed`（**复用现有 reason 值，前端零改动**；baize 日志记录真实原因 adapter unreachable 便于排查）。
- **URL/前端不变：** 微信实例名固定 `weixin`、source `weixin`，故管理 URL 仍是 `/v0/settings/channels/weixin/*`，前端 `WeixinChannelSettings.tsx` 与 `api.ts` **零改动**（响应 JSON 字段 `ticket/qr_url/status/agent_id/allowlist/assignee/enabled/running/reason` 逐一对等）。前端路由/导航/标签（`weixin:` 前缀）不变。
- **HMAC：** baize→适配器 `/admin/*` 与 `/outbound` 同样用 `outbound_secret`（回落 `secret`）签名（`webhooksig.Sign` + 协议头）；适配器验签。适配器本地监听回环，仅 baize 可达。

## 8. 微信功能对等回归清单（2B 逐项必须有测试）

迁移后每一项都要有对等测试（适配器契约测试 / baize 集成测试），全绿为合并门槛：

1. **扫码登录全流程**：`login/start` → `{ticket, qr_url}` → 轮询 `login/status`（pending→success/expired）→ success 落适配器 `creds.json`（account_id+token）→ 自动开始轮询。
2. **登录态回显**：`GET settings` 返回 `running/reason`：未登录 `login_required`；有凭据未轮询/适配器不可达 `start_failed`；轮询中 `running:true`；停用 `running:false, reason:""`。
3. **登出**：`logout` → 适配器停轮询、清 creds.json；baize 侧 settings（assignee/agent/allowlist/enabled）保留。
4. **启停**：`enabled:false` → 通知适配器停轮询（保凭据）；`enabled:true` → 启动轮询（无凭据则 `login_required` 不启动）。
5. **白名单**：非白名单 peer 入站静默丢弃（baize 通用入站 handler）；白名单内正常建 run；空表白名单放行全部；allowlist 热更新（PUT 后下一条消息生效，不重启）。
6. **群聊丢弃**：`xxx@chatroom` 消息适配器不转发（或 baize 不建 run）。
7. **出站前缀**：助手回复带【助手】、客服镜像带【客服】（通用 outbound 层，e2e 断言出站 text 含前缀）。
8. **HITL 审批**：run 进入 waiting_human → 适配器收到 notify；IM 内回复"同意/拒绝" → 续跑/终止 + ack。
9. **UI 操作员镜像**：UI 发言 → 适配器收到 operator 出站。
10. **context_token 透传**：入站 context_token 缓存 → 出站 payload 带回（适配器→iLink SendMessage 的 context_token）。
11. **入站媒体（含解密）**：微信图片（supports_vision）→ 多模态 part；文件 → 附件（display 带文件名）；适配器对 CDN 密文做 AES-128-ECB 解密（§15），下载后可得明文；解密失败的附件退化为文件名占位但不丢消息。
12. **出站媒体（真发）**：助手回复带图片/文件 → baize 出站 `media[]` → 适配器走 `getuploadurl`+AES 加密+CDN 上传+`sendmessage` 图片/文件项真发到微信（§15）；e2e 用 httptest 假 iLink 断言上传三步与 item 引用；出站媒体失败不中断文本投递。
13. **历史会话兼容**：convID `weixin:<accountID>:<peer>`、meta.Source `weixin` 不变；同一微信号登录后 accountID 一致，历史会话不失联。
14. **动态 account**：入站 payload account 用于拼 convID；出站 body account 与入站一致。
15. **管理面鉴权**：`/v0/settings/channels/weixin/*` 需 admin（operator 403）；入站 `/inbound` 走 HMAC 不需 token。
16. **默认部署**：autostart 配置下 baize 启动自动拉起适配器、健康检查通过、复用 `data/channels/weixin/creds.json` 免重扫。

## 9. 跨平台子进程托管

webhook 实例 config 新增键（2A 已预留 `adapter_command`/`adapter_autostart`，2B 实现）：

| 键 | 说明 |
|---|---|
| `adapter_command` | 适配器可执行文件路径或命令行（如 `./bin/weixin-adapter` 或 `weixin-adapter`）。支持带参数（空格分隔或用 `adapter_args`）。 |
| `adapter_args` | 可选，命令参数（逗号/空格分隔），避免 command 里解析空格的跨平台问题。 |
| `adapter_autostart` | `"true"` 时 baize 托管子进程生命周期。 |

**托管流程（跨平台，仅用标准库 `os/exec` + `net/http`）：**
- 装配/Start 该 webhook 实例且 `autostart:true` 时：`exec.Command(adapter_command, args...)` 启动子进程；设置 `cmd.Stdout/Stderr` 透传到 baize 日志（便于排查）；**不假设 shell**（直接 exec，不走 `sh -c`/`cmd /c`，跨平台安全）。
- 适配器监听 `127.0.0.1:<port>`（端口由 baize 分配或适配器配置；建议 baize 传 `-port=0` 让适配器选端口后经 `/healthz` 或约定发现——v1 简化为**固定/配置端口**，baize 轮询 `http://127.0.0.1:<port>/healthz` 直到 200，超时如 30s）。
- baize 把 baize 入站 URL、secret、creds 目录等通过**命令行参数/环境变量**传给适配器（适配器不读 baize 配置）：如 `-baize=http://127.0.0.1:<baize-port> -secret=<secret> -creds=./data/channels/weixin -addr=127.0.0.1:<port>`。
- **关停**：baize 关停（closer LIFO，先于/随渠道 Stop）终止子进程——跨平台用 `cmd.Process.Kill()`（SIGKILL 等价，Windows TerminateProcess）；可选先尝试优雅退出（v1 直接 Kill 即可，适配器无长写事务）。子进程设 `Setpgid`/进程组处理留待需要时（Linux 孤儿进程回收由 OS 接管）。
- **崩溃重启（v1 简化）**：子进程异常退出时 baize 记录日志；v1 不做自动重启循环（列为后续增强），健康检查失败在管理面反映为 `adapter_unreachable`。
- **独立部署**：`adapter_autostart:false`（或缺省）时 baize 不拉子进程，仅按 `outbound_url`/`admin_url` 连远程/手动启动的适配器（与第三方适配器同等待遇）。
- **跨平台验证**：在 Linux（CI）与 Windows（开发机）都跑托管拉起/健康检查/关停测试；用一个**测试用假适配器子程序**（test fixture，`go run` 一个最小 HTTP server）验证，不依赖真实微信。

## 10. 默认部署配置与核心删除

**`configs/minimal.yaml`：** 默认渠道改为 webhook 类型的 `weixin` 实例 + autostart（注释或默认开启，保持"装 baize 即用微信"等价）。形如：

```yaml
channels:
  - name: weixin
    type: webhook
    enabled: true
    config:
      source: weixin
      # secret 留空：autostart 时 baize 自动生成随机 HMAC 并经 -secret 传给适配器
      outbound_url: http://127.0.0.1:8090/outbound
      admin_url: http://127.0.0.1:8090
      assignee: channel:weixin
      supports_vision: "true"
      adapter_autostart: "true"
      adapter_command: weixin-adapter   # baize 跨平台解析：PATH / ./bin，Windows 自动补 .exe
      adapter_args: "-addr=127.0.0.1:8090,-creds=./data/channels/weixin"
      # allowlist 留空=放行全部；creds 复用 ./data/channels/weixin/creds.json 免重扫
```
> **跨平台二进制解析**：`adapter_command` 先走 `exec.LookPath`，再尝试 `./bin/<cmd>`；Windows 上自动补 `.exe` 后缀（构建产物 `bin/weixin-adapter.exe`）。适配器 `-addr/-creds/-baize/-secret` 等参数由 baize 拼装（§9），`adapter_args` 提供额外覆盖。
>
> **legacy 裸部署**：省略 `channels:` 段时不再隐式装配微信（weixin Descriptor 已删）。开箱微信能力由随附的 minimal/demo 配置显式提供；完全省略 `channels:` = 无 IM 渠道（其余能力不受影响）。

**核心删除/替换（2B 收尾）：**
- 删除 `internal/channel/weixin/`（整个包迁入 `cmd/weixin-adapter/internal/weixinlink/`）；移除 `bootstrap.go` 与 `cmd/baize/main.go` 的 weixin blank import。
- 删除 `internal/api/server_channel_weixin.go`（微信专用 handler）与 `server.go` 里 5 条微信硬编码路由；由通用渠道管理路由 + webhook 管理代理取代。
- 删除 `weixinRuntimeState`/`applyWeixinSettings` 等微信专用协调逻辑（能力并入通用 webhook 渠道设置 + 状态合并）。
- `internal/api/channels_test.go` 中以 `weixin.Channel` 为具体类型的句柄表测试改用 webhook 或 fake 渠道。
- 微信相关测试迁移：`channel/weixin/*_test.go`（ilink 协议级）随库迁入适配器；`api/server_channel_weixin_test.go` 改写为通用渠道管理 + 适配器 fake 的契约测试；新增适配器 e2e。

## 11. 测试策略（TDD）

- **适配器库（`cmd/weixin-adapter/internal/weixinlink/`）**：迁入的 `client_test.go`/`fake.go` 全绿（协议级不变）；新增 `media_test.go`：AES-128-ECB/PKCS7 round-trip、入站下载解密、出站 `getuploadurl`+CDN 上传+`sendmessage` 图片/文件项（httptest 假 iLink/CDN 断言，见 §15）。
- **适配器 HTTP 面**：用 fake ilink + httptest 测 `/outbound`（验签、调 SendMessage、context_token）、`/admin/login/*`（start→ticket/qr、poll pending/success/expired、落凭据）、`/admin/logout`、`/admin/status`（凭据/轮询态）、`/healthz`；入站组装（fake GetUpdates → 签名 POST baize，断言 payload account/peer/text/媒体）。
- **baize webhook 渠道增强**：动态 account（入站 payload account 拼 convID；出站从 convID 回填 account）；通用渠道设置持久化 + 热更新（assignee/agent/allowlist/enabled）；管理代理（login/logout/status 转发 fake 适配器，HMAC 头）；`running/reason` 合并推导五态 + `adapter_unreachable`；入站白名单读热更新字段。
- **子进程托管（跨平台）**：用最小假适配器子程序（HTTP `/healthz`+`/outbound`）测 exec 拉起、健康检查就绪、关停终止；Linux CI 与 Windows 本机都跑。
- **端到端 `tests/integration`**：假/真适配器经 webhook 渠道双向闭环（文本+图片），走通用管理路由登录态、白名单、HITL、前缀、context_token、动态 account、历史 convID 兼容——覆盖 §8 清单中可自动化的项。
- **全量门槛**：`go build ./...`、`go test ./...`（含 integration 与适配器）、`gofmt -l`、`go vet ./...` 全绿；前端 `npm run build` 通过（前端零改动，验证不破坏）。

## 12. 涉及文件（预估）

**新增：**
- `cmd/weixin-adapter/main.go`、`config.go`、`inbound.go`、`outbound.go`、`admin.go`、`healthz` + `internal/weixinlink/`（迁入 ilink.go/client.go/creds.go/fake.go + 测试；**新增 `media.go`：AES-ECB 加解密 + getuploadurl/CDN 上传/发图发文件 + 入站下载解密，见 §15**）。
- `internal/channel/webhook/settings.go`（通用渠道设置持久化/热更新）、`admin.go`（管理面代理客户端）、`supervisor.go`（子进程托管）+ 对应 `*_test.go`。
- 适配器 e2e / 子进程托管跨平台测试 / 通用渠道管理 api 测试。

**修改：**
- `internal/channel/webhook/channel.go`/`inbound.go`/`outbound.go`：动态 account、出站 account 从 convID 回填、热更新白名单、装配 settings、admin_url/supervisor 接线。
- `internal/channel/channel.go`：`AccountFromConvID` helper。
- `internal/channel/runtime.go`：出站 account 回填支持（或在 webhook 层解析 convID）。
- `internal/api/`：通用渠道管理路由 `/v0/settings/channels/{name}/...`（GET/PUT 设置 + login/logout/status 代理），注册到 mux + ACL（RoleAdmin）。
- `internal/controlplane/acl.go`：通用渠道管理路由的 RoleAdmin 规则（取代微信专用规则）。
- `internal/bootstrap/bootstrap.go`：装配 webhook weixin 实例、子进程托管接线、移除 weixin 进程内装配。
- `configs/minimal.yaml`/`demo.yaml`：默认 webhook-weixin + autostart。
- 前端：**预期零改动**（URL/字段不变）；仅在发现字段缺口时最小修正。

**删除：**
- `internal/channel/weixin/`（迁入适配器）、`internal/api/server_channel_weixin.go`、`server.go` 微信路由、相关微信专用装配/blank import。

## 13. 验收标准

1. 微信经适配器运行：`cmd/weixin-adapter` 经 webhook 渠道接入，§8 回归清单 16 项逐项有测试且全绿。
2. baize 核心无进程内 weixin：`internal/channel/weixin` 删除、不注册/装配；微信专用 handler 删除；通用管理代理接管，管理 URL `/v0/settings/channels/weixin/*` 与前端页面、JSON 字段不变（前端零改动）。
3. 默认部署等价：autostart 跨平台（Linux/mac/Windows）拉起适配器、健康检查、关停；复用 `data/channels/weixin/creds.json` 免重扫。
4. 终态一套机制：所有 IM（含微信）都是 webhook 实例 + 适配器；飞书等第三方适配器路径不受影响。
5. `go build ./...`、`go test ./...`（含 integration/适配器）、gofmt/vet 全绿；前端 build 通过。
6. 双仓发布：real 完整历史；public 开源切片（适配器同仓开源，契约文档公开）。

## 14. 关键决策（已定）

- **secret 注入**：autostart 模式下，若实例 config 未配 `secret`，baize 用 `crypto/rand` 生成随机 HMAC secret，经命令行参数 `-secret` 传给子进程（适配器不读 baize 配置）；回环 + 随机密钥，默认部署免手工配密钥。独立部署（`adapter_autostart:false`）时 `secret` 为必填，由运维在 baize config 与适配器启动参数双方约定。**相应放宽 2A 校验**：`parseConfig` 的"secret 必填"改为"`secret` 必填，除非 `adapter_autostart:true`（运行期生成并注入子进程）"；出站 `outbound_secret` 回落逻辑不变。
- **legacy 裸部署**：weixin Descriptor 删除后，省略 `channels:` 段不再隐式装配微信。**采用方案 (a)**：`configs/minimal.yaml` 与 `demo.yaml` 显式声明 webhook-weixin 实例（`enabled:true` + `adapter_autostart:true` + 自动 secret），使"装 baize 即用微信"的开箱体验由**随附配置**提供（配置即文档，不引入隐式魔法）。完全省略 `channels:` 的裸部署 = 无 IM 渠道（引擎/UI/inbox 等其余能力不受影响），在 README/配置注释说明。
- **端口分配**：v1 用固定/配置端口——适配器 `-addr` 默认 `127.0.0.1:8090`，baize config 的 `outbound_url`/`admin_url` 指向它；autostart 时 baize 经 `-addr` 显式传端口。多实例需配不同端口（文档注明）。动态端口发现（`-addr=127.0.0.1:0` + 适配器把实际端口写到 stdout/健康文件）列为后续增强。
- **子进程崩溃**：v1 不自动重启（管理面状态归为 `start_failed`，日志记录）；自动重启/退避列为后续。
- **适配器与 baize 同机假设**：autostart 模式下回环；独立部署时适配器可在远端（`outbound_url`/`admin_url` 指向远端，HMAC 保障）。

## 15. iLink 媒体协议（2B 新增：出站真发 + 入站解密）

开源参考：cc-weixin `docs/API-REFERENCE.md`、openclaw-weixin `weixin-bot-api.md`、BotTalk `ilink.sendImage`、ciphertalk `weixinIlinkClient.ts`（同一套 `ilinkai.weixin.qq.com` 协议）。Go 侧仅用标准库 `crypto/aes`+`crypto/cipher`(AES-ECB/PKCS7)+`crypto/md5`+`crypto/rand`+`encoding/base64`/`hex`，无新依赖。

**入站媒体（微信→系统）解密：**
- iLink 入站图片/文件经 CDN 下发，`item.media` 带 `encrypt_query_param` + `aes_key` + `encrypt_type`；下载字节是 **AES-128-ECB + PKCS7** 密文（密钥即消息项的 `aes_key`，16 字节）。
- 适配器 `DownloadMedia` 下载后，用该 `aes_key`（按消息项 `aeskey`/`media.aes_key` 归一）做 AES-128-ECB 解密、去 PKCS7，得到明文，再 base64 进入站 payload `attachments[].content_base64`（或本地 URL）。密钥解析需兼容两种形态：`image_item.aeskey`（32 字符 hex）与 `media.aes_key`（base64）。
- 解密失败（密钥缺失/格式不符）时：该附件退化为"仅文件名占位"（不丢整条消息），日志记录；保持健壮性。
- 这同时修掉现状 README 承认的"CDN AES 解密未实现、加密媒体下载后不可用"。

**出站媒体（系统→微信）真发（3 步）：**
1. **生成密钥**：`crypto/rand` 16 字节 AES key；明文 MD5（hex）；AES-128-ECB + PKCS7 加密文件字节，记录 `rawsize`（明文长度）、`filesize`（密文长度）。
2. **取上传地址**：`POST /ilink/bot/getuploadurl`（Bearer 鉴权头同 sendmessage），body：
   ```json
   {"filekey":"<随机 hex>","media_type":1,"to_user_id":"<peer>",
    "rawsize":<明文>,"rawfilemd5":"<md5 hex>","filesize":<密文>,
    "no_need_thumb":true,"aeskey":"<AES key hex>","base_info":{"channel_version":"1.0.0"}}
   ```
   `media_type`：`1`=图片、`3`=文件（语音 2 / 视频 5 本期不做）。响应取 `upload_param`。
3. **上传 CDN + 发消息**：`POST {CDNBase}/upload?encrypted_query_param=<upload_param>&filekey=<filekey>`，`Content-Type: application/octet-stream`，body=密文字节；响应头 **`x-encrypted-param`** 为下载句柄。然后 `POST /ilink/bot/sendmessage`，`item_list` 引用：
   - 图片项：`{"type":2,"image_item":{"media":{"encrypt_query_param":"<x-encrypted-param>","aes_key":"<base64(AES key)>","encrypt_type":1},"mid_size":<密文长度>}}`
   - 文件项：`{"type":4,"file_item":{"media":{"encrypt_query_param":"<x-encrypted-param>","aes_key":"<base64(AES key)>","encrypt_type":1},"file_name":"<名>","len":"<大小>"}}`
   - 仍带 `context_token`（出站 payload 透传，来自入站缓存）。
- webhook 出站 body 的 `media[].content_base64` 由适配器解码后走上述流程；文本与媒体顺序：先发 text 再逐个 media（与 `DeliverAssistantReply` 一致）。
- 出站媒体在适配器侧失败（上传/发送错误）：记日志、该媒体不中断文本投递（与"出站失败不中断 run"一致）。

**测试（CI 可自动化，真机手验）：**
- 入站解密：httptest CDN 返回 AES-ECB 密文，断言适配器解出明文并入 `attachments`（多模态/附件）。
- 出站上传：httptest 假 iLink 断言 `getuploadurl` 请求字段（media_type/rawsize/filesize/md5/aeskey）、CDN 收到密文（可解密回明文验证）、`sendmessage` 的 image_item/file_item 引用 `x-encrypted-param` 与 base64 aes_key。
- AES round-trip 单测（加密→解密还原；PKCS7 边界）。
- 真机扫码 + 私信收发图片/文件为手工验收（README 已声明 iLink 无 CI 真机测）。


