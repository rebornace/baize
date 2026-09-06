# 阶段二设计：进程外 webhook 渠道（统一渠道机制，微信也迁至适配器）

> 状态：设计稿（仅私有仓 docs/superpowers，不导出公开仓）
> 日期：2026-09-06
> 前置：阶段一「渠道注册表化」已合并（Descriptor 注册表 / 出站 Router / api 渠道句柄表 / bootstrap 通用 `wireChannels` / config 声明式启用）。
> 关联：`docs/superpowers/specs/2026-09-06-plugin-decoupling-architecture.md` 阶段二；`docs/superpowers/plans/2026-09-06-phase1-channel-registry.md`。

## 0. 终态与最高约束（铁律）

**终态：只有一套渠道机制。** 所有 IM 渠道（含微信）都是"通用 webhook 渠道实例 + 独立适配器进程"。阶段二结束后 baize 核心**不再保留进程内 weixin 渠道这个特例**——`internal/channel/weixin/` 的 ilink 私有协议实现迁移到同仓适配器 `cmd/weixin-adapter`，核心内删除进程内 weixin 渠道（复用的纯逻辑除外）。飞书/钉钉/Slack 与微信在机制上完全平权，只是各自的适配器进程不同。

**微信功能零丢失、零回归（贯穿整个迁移，最高优先级）。** 迁移必须逐项保持现有能力与运营体验：

- 扫码登录（运营在 baize 管理页发起 → 显示二维码/票据 → 适配器轮询登录态 → 成功落凭据）、登出、凭据本地持久化、登录态 `running`/`reason`/`login_required` 回显。
- 长轮询收消息、白名单入站强制（非白名单 peer 丢弃）、allowlist 热更新。
- 出站：`【客服】`/`【助手】` 前缀、HITL 审批通知与续跑（IM 内回复"同意/拒绝"）、UI 操作员消息镜像、context_token 会话上下文透传。
- 入站媒体（图片/文件下载 → 多模态/附件）；出站媒体投递。
- convID 格式 `weixin:<account>:<peer>` 与历史会话 ID 兼容；meta.Source `"weixin"` 不变（历史会话不失联）。
- 以 `tests/integration` + `api/server_channel_weixin_test.go` + `channel/weixin/*_test.go`（迁移为适配器契约测试）全绿为门槛；每一步迁移都有对等测试覆盖，不允许"迁完发现功能没了"。

**分两步连续交付（都在阶段二内）：**
- **2A 通用 webhook 渠道**：通用渠道类型 + 双向消息协议 + 渠道管理面协议 + 路由自注册 + 多实例 + 媒体 + 示例适配器 `examples/im-adapter`。此时进程内 weixin 仍在、行为不变（系统同时存在旧 weixin 与新 webhook，互不影响）。
- **2B 微信迁移**：把 ilink 实现搬入 `cmd/weixin-adapter`（实现消息 + 管理面协议），baize 核心删除进程内 weixin 渠道、改为装配一个 webhook 实例对接它；可选子进程托管让默认部署"装 baize 即用微信"。终态只剩一套机制。

## 1. 目标与非目标

**目标：** 新增一个 IM 渠道（飞书/钉钉/Slack…）= 部署一个独立适配器进程（任意语言）+ 加一段配置，**baize 核心零改动、零重编译、不用给核心提 PR**。适配器通过文档化的 JSON-over-HTTP 契约与 baize 双向通信，能力与进程内微信对等（文本 + 图片 + 任意文件双向、HITL 审批、busy、多模态）。

**非目标（YAGNI）：**
- 不做公共 Go SDK（阶段三，暂缓）。
- 不做出站长轮询 pull 传输（本阶段定 push：baize 直推适配器）。
- 不做持久化 outbox / 跨重启可靠投递（v1 用同步 + 有限重试；outbox 列后续增强）。
- 不实现真实飞书/钉钉/Slack API 对接——`examples/im-adapter` 只模拟与演示，真实第三方 IM 适配是适配器作者的事（微信适配器除外，微信由本仓 2B 提供）。
- 不动 inbox（系统告警 / HITL 审批回调继续走 `POST /v0/inbox/{id}`，职责不变）。
- 不做适配器市场/动态加载/远程安装等运营化能力；适配器通过配置声明接入。

**2A 阶段约束：** 2A 交付时进程内 weixin 渠道**保持原样、行为不变**（此时系统并存旧 weixin 与新 webhook，两者不互相影响）；微信的迁移与旧包删除发生在 2B。

## 2. 总体架构

新增 in-tree 包 `internal/channel/webhook/`：一个**通用**渠道类型（不绑定任何具体 IM）。

- 实现 `channel.Channel`（`Name/Start/Stop/SendText/SendMedia`）、`channel.SourceSourced`（`Source()` 返回实例配置的 source）、`channel.Bootstrapper`（`Bootstrap` 返回自己的 `*channel.Runtime`）。
- `init()` 注册 `channel.Descriptor{Name: "webhook", Build: openFromConfig, EnabledByDefault: false}`——**默认不启用**（没有适配器无意义），必须在 config 显式声明。
- 飞书/钉钉/Slack **不是新渠道类型**，而是 `webhook` 类型的多个配置实例 + 各自独立部署的适配器进程。

数据流：

```
IM 用户 → 适配器进程(任意语言) → 签名 POST /v0/channels/{name}/inbound
       → webhook 渠道验签/限流/幂等 → Runtime.HandleInbound（阶段一路径）→ 引擎
引擎回复 → DeliverAssistantReply → Router 按 meta.Source 选 webhook 实例
       → SendText/SendMedia → 签名 POST 适配器 outbound_url → 适配器调 IM API
```

与 inbox 的分工：inbox = 机器系统（监控/CI/工单）的结构化动作（create_run / HITL resume）；webhook 渠道 = 人 ↔ agent 的 IM 双向聊天（自带 IM 内 HITL 审批、回复、通知）。两者并存、互不改动。

## 3. 多实例配置模型

### 3.1 `ChannelConfig` 增加实例名

```go
type ChannelConfig struct {
    Name    string            `yaml:"name"`    // 实例名；同一 type 多实例时必填且唯一。缺省 = type
    Type    string            `yaml:"type"`    // "webhook" / "weixin"
    Enabled bool              `yaml:"enabled"` // 声明式模式下列出即需 enabled:true 才装配
    Config  map[string]string `yaml:"config"`  // 不透明配置；传给 Descriptor.Build
}
```

### 3.2 `wireChannels` 按实例遍历（声明式模式）

- **省略 `channels:` 段（非声明式）**：保持阶段一行为——遍历 `channel.Descriptors()`，装配全部 `EnabledByDefault=true` 的渠道（= weixin）。**此路径零改动，微信不受影响。**
- **声明式模式（`channels:` 段非空）**：改为**逐条配置**处理，而不是逐 Descriptor：
  1. 对每条 `{name,type,enabled,config}`：`enabled:false` 跳过；`channel.Describe(type)` 找描述符，找不到 → 启动报错（未知渠道类型）。
  2. 合并默认 `creds_dir`（`desc.DefaultCredsDir`，多实例时用实例 name 区分目录，如 `data/channels/webhook/<name>`）与 `config` 覆盖，调 `desc.Build(channel.Config)` 建**一个实例**。
  3. 实例实现 `Bootstrapper` → `Bootstrap(deps)` 返回自己的 `Runtime`（webhook 的 `Runtime.Source` = 该实例 `source`）→ `router.Add(ch)`（按实例 `Source()` 注册）+ `router.BindRuntime(rt)` + `srv.RegisterChannel(&ChannelHandle{Name: 实例 name, Channel: ch, Runtime: rt, ...})` + 条件 `Start` + 注册 Stop closer。
  4. 向后兼容：声明式段里若列出 weixin（`type: weixin`），仍能正确装配（单实例，name 缺省 = "weixin"）。
- 重复 `name` → 启动报错（避免静默覆盖）。
- 重复 `source`（两个实例配了相同 source）→ 启动报错（Router 按 source 路由，冲突会串话）。
- 鉴权豁免：入站端点 `POST /v0/channels/{name}/inbound` 走**渠道密钥 HMAC** 鉴权（不要求 operator/admin token），与 inbox webhook 同款思路——适配器只持有本渠道 secret。该路径在 api 路由层显式加入"渠道入站白名单"，不套用控制面 token 中间件。

### 3.3 webhook 实例配置键（`config map[string]string`）

| 键 | 必填 | 说明 |
|---|---|---|
| `source` | 是（缺省取实例 name） | convID 前缀 / meta.Source，如 `feishu`；同进程内唯一 |
| `secret` | 是 | 入站验签 HMAC 密钥（适配器→baize） |
| `outbound_url` | 是 | 适配器出站端点（baize→适配器），如 `http://feishu-adapter:8080/outbound` |
| `outbound_secret` | 否 | 出站签名密钥；缺省用 `secret` |
| `assignee` | 是 | 会话 owner（操作员/用户 ID） |
| `agent_id` | 否 | 默认 agent；缺省用系统默认 |
| `supports_vision` | 否 | `"true"` 时入站图片转多模态 ContentPart |
| `allowlist` | 否 | 逗号分隔的允许 peer.id 列表；非空时仅白名单入站（通用白名单，2B 微信白名单上移到此） |
| `admin_url` | 否 | 适配器管理端点基址（如 `http://127.0.0.1:8090`）；存在则 baize 渠道设置页代理 `/admin/*`（2B 微信需要） |
| `adapter_command` | 否 | 适配器可执行文件/命令行；配 `adapter_autostart: "true"` 时 baize 自动拉起子进程（2B） |
| `adapter_autostart` | 否 | `"true"` 时 baize 托管适配器子进程生命周期（2B） |
| 其余键 | 否 | 不透明，保留（可透传/未来扩展） |

> **account（IM 账号/机器人标识，convID 第二段）不来自 config，而来自入站 payload 的 `account` 字段**（适配器最清楚消息属于哪个机器人；一个适配器甚至可承载多个机器人账号）。入站 `account` 必填，用于拼 `ConvID(source, account, peer)`。这与微信 accountID 来自登录态同理。

YAML 示例：

```yaml
channels:
  - name: feishu
    type: webhook
    enabled: true
    config:
      source: feishu
      secret: ${FEISHU_CHANNEL_SECRET}
      outbound_url: http://feishu-adapter:8080/outbound
      assignee: u-admin
      agent_id: agent-default
      supports_vision: "true"
  - name: dingtalk
    type: webhook
    enabled: true
    config:
      source: dingtalk
      secret: ${DINGTALK_CHANNEL_SECRET}
      outbound_url: http://dingtalk-adapter:8080/outbound
      assignee: u-admin
```

会话隔离：`feishu:feishu:u123` 与 `dingtalk:dingtalk:u456` 是不同 source 命名空间，Router 按 `meta.Source` 选对应实例，天然互不串话。

## 4. 渠道 HTTP 路由自注册（方案 A）

为让"新增带 HTTP 面的渠道零改 `server.go`"（顺带清阶段一 backlog 第 1 项），在 `channel` 包定义最小路由接口（只依赖标准库 `net/http`，**不依赖 api 包**，防循环依赖）：

```go
// RouteRegistrar 由 api.Server 实现，供渠道在装配期挂载自己的 HTTP 端点。
type RouteRegistrar interface {
    RegisterRoute(pattern string, h http.Handler) // pattern 形如 "POST /v0/channels/feishu/inbound"
}
```

- `BuildDeps` 增加字段 `Routes RouteRegistrar`（可能为 nil；渠道需判空）。
- `api.Server` 实现 `RegisterRoute(pattern, h)`：转发到内部 mux（Go 1.22 方法路由 `mux.Handle(pattern, h)`）。
- webhook 渠道在 `Bootstrap(deps)` 里，若 `deps.Routes != nil`，挂载入站路由：`deps.Routes.RegisterRoute("POST /v0/channels/"+name+"/inbound", handler)`。
- weixin **不实现**路由注册，其现有 5 条路由保持在 `server.go` 硬编码（行为不变）；未来可选迁移，非本阶段范围。

## 5. 有线协议（JSON-over-HTTP，文档化，任意语言可实现）

协议头统一带 `X-Baize-Protocol: v0`（版本标记）。

### 5.1 入站（适配器 → baize）

`POST /v0/channels/{name}/inbound`

请求头：
- `X-Baize-Protocol: v0`
- `X-Baize-Channel-Timestamp: <unix-seconds>`
- `X-Baize-Channel-Signature: v1=<hex>`，签名 = `HMAC-SHA256(secret, "<timestamp>."+rawBody)` 的 hex，300s 时间窗防重放。
- **实现方式**：把 `internal/inbox/verify.go` 的签名/验签算法抽到一个共享小包（如 `internal/sign` 或 `internal/webhooksig`，只依赖标准库），inbox 与 webhook 渠道共同引用；避免在 webhook 包复制一份算法产生分叉。inbox 行为不变（纯重构，现有 inbox 测试须保持全绿）。

请求体：

```json
{
  "event": "message",
  "account": "feishu-bot-1",
  "peer": {"id": "u123", "name": "张三"},
  "text": "你好",
  "attachments": [
    {"name": "a.png", "mime": "image/png", "content_base64": "…", "url": "https://…/a.png"}
  ],
  "context_token": "可选，IM 会话上下文",
  "idempotency_key": "可选，去重键"
}
```

baize 处理：验签 → 时间窗校验 → `idempotency_key` 幂等去重 → 组装 `channel.Inbound{PeerID: peer.id, Text, Files: attachments 解析, Extras: {account, context_token, peer_name}}` → `Runtime.HandleInbound(ctx, ch, in)`。之后自动获得：convID 生成、`EnsureMeta`、busy/HITL 审批解析（回复"同意/拒绝"）、`buildInboundContent`（文本+附件多模态）、`CreateRun`、消息落库——与微信完全对等。

响应：`200 {"ok":true}`；签名错误 `401`；时间窗/参数错误 `400`；重复幂等键 `200`（视为已处理）。

### 5.2 出站（baize → 适配器）

baize 对 `outbound_url` 发签名 POST。

请求头：
- `Content-Type: application/json`
- `X-Baize-Protocol: v0`
- `X-Baize-Run-Id: <runID>`（有 run 上下文时）
- `X-Baize-Channel-Timestamp`、`X-Baize-Channel-Signature: v1=<hex>`（用 `outbound_secret`，算法同入站）

请求体：

```json
{
  "kind": "assistant|operator|notify",
  "conversation_id": "feishu:feishu-bot-1:u123",
  "account": "feishu-bot-1",
  "peer": {"id": "u123"},
  "text": "【助手】……",
  "media": [
    {"name": "f.png", "mime": "image/png", "content_base64": "…"}
  ],
  "run_id": "…",
  "context_token": "…"
}
```

- `kind`：`assistant`=助手回复（【助手】前缀，由 `DeliverAssistantReply` 加）、`operator`=客服/操作员镜像（【客服】前缀，`DeliverUserText`）、`notify`=HITL/系统通知。
- 适配器返回 2xx 表示已接收；baize `SendText/SendMedia` 同步 POST，超时 10s，失败重试 3 次（指数退避，如 1s/2s/4s）；最终失败返回 error（引擎侧现有逻辑记日志，不中断 run）。
- 适配器侧职责：验签 → 按 `peer.id` / `context_token` 调对应 IM API 发文本/上传媒体 → 返回 2xx。

### 5.3 渠道管理面协议（适配器管理 API；2B 微信运营能力需要）

微信的扫码登录/登出/启停/登录态回显目前是 baize 进程内逻辑。外迁后这些由适配器承担，baize 渠道设置页**通用代理**到适配器，运营体验与 URL 不变。

适配器暴露管理端点（baize → 适配器，同样 HMAC 签名头；适配器本地监听，仅 baize 可达）：

| 端点 | 作用 | 微信 ilink 对应 |
|---|---|---|
| `GET  /admin/status` | 登录/运行态：`{running, reason, login_required, has_credentials, account_id?}` | HasCredentials / 轮询态 |
| `POST /admin/login/start` | 发起登录，返回 `{ticket, qr_url, qr_image?}` | apply 二维码票据 |
| `GET  /admin/login/status?ticket=` | 轮询登录进度 `{status: waiting/scanned/confirmed/logged_in/failed}` | 轮询二维码状态 |
| `POST /admin/logout` | 清凭据、停轮询 | 登出/清 creds |
| `POST /admin/start` / `POST /admin/stop` | 启用/停用渠道轮询 | 渠道启停 |

凭据由适配器持久化在自己的 creds 目录（迁移时把现有 `data/channels/weixin` 凭据带到适配器可读位置）。

**职责划分：**
- baize 侧（通用，不针对微信）：`assignee`、`agent_id`、`supports_vision`、`allowlist`（白名单入站强制在 baize 入站 handler 通用执行，见 §6）、会话/路由。
- 适配器侧（IM 专有）：登录/凭据/二维码、长轮询收消息、调 IM API 发消息/传媒体、IM 账号标识。

**baize 通用渠道管理代理：** api 层提供控制面鉴权下的通用渠道管理路由 ` /v0/settings/channels/{name}/...`，按 `{name}` 找到渠道句柄；若该渠道实现管理接口（webhook 渠道实现，转发到适配器 `/admin/*`），则代理请求并回传响应。**微信默认实例名取 `weixin`、source `weixin`**，使现有管理 URL `/v0/settings/channels/weixin/*` 与前端页面**完全不变**，微信专用 handler 被通用代理取代后响应 JSON 字段保持一致（`enabled/running/reason/credentials/allowlist/login_url/polling/login_required`）。2A 阶段 webhook 渠道可先不实现管理面（示例适配器不需要登录）；管理面协议在 2B 随微信迁移落地并被微信设置页测试锁定。

## 6. 媒体全量双向（文本 + 图片 + 任意文件）

与微信能力对等（微信入站支持图片/文件下载→多模态/附件；出站 SendMedia 微信当前是占位，webhook 渠道实现真实投递）。

**入站附件** `attachments[]`，每项二选一：
- `content_base64`：内联 base64（整请求 body ≤ 2MiB；超限应改用 url）。
- `url`：http/https 可下载地址，由 webhook 渠道服务端下载（超时 15s、大小上限如 10MiB、仅 http/https）。

下载/解码后得到 `[]byte`，装入 `channel.InboundFile{Name, MIME, Data}`，复用现有 `buildInboundContent → attach.Process`：图片在 `supports_vision=true` 时转 `llm.ContentPart{Type:"image"}` 多模态，文档类抽文本，其余作为附件记录。**核心 `channel.InboundFile` / `attach.AttachmentIn` 结构不改**（已是内联字节）；URL 下载器封在 webhook 包内（不引入对 weixin 私有 CDN 逻辑的依赖）。

**出站媒体**：`SendMedia(ctx, peer, filename, mime, data, extras)` 把 `data` base64 放入出站 body 的 `media[].content_base64` 内联 POST（引擎产出的附件本就是 `[]byte`）；任意 MIME 透传，适配器负责上传到 IM。文本与媒体在一次助手回复中：先发 text 再逐个 media（与 `DeliverAssistantReply` 现有顺序一致）。

## 7. 示例适配器 `examples/im-adapter`

独立 Go 进程，**只依赖标准库、不 import 任何 baize 包**（对标 `examples/http-plugin`）：

- 一个触发端点（如 `POST /simulate-inbound` 或启动时演示）模拟"IM 用户来消息"：构造一条文本 + 一张测试图片，按协议签名 POST 到 baize `POST /v0/channels/{name}/inbound`。
- 实现 `POST /outbound`：验签，把 baize 推来的助手回复 / 媒体打印到日志（标注"此处替换为调 IM API"），返回 200。
- 内置与 baize 一致的 HMAC 签名/验签辅助函数（自包含，证明任意语言可实现）。
- README：完整契约、签名算法、配置（baize 侧 channels 段 + 适配器侧 secret/url）、飞书/钉钉/Slack 适配要点（把"打印/模拟"换成真实 IM API 调用的位置）。

## 8. 错误处理 / 幂等 / 重试

- **入站验签**：HMAC-SHA256 + 时间戳，时间窗 300s 防重放；签名不符 `401`，时间戳过期 `400`。
- **入站幂等**：`idempotency_key` 非空时落一条投递记录（store/KV）去重，重复返回 `200 {"ok":true,"duplicate":true}`（与 inbox 幂等同思路，但封在 webhook 渠道/独立表，不共用 inbox 表）。
- **入站限流**：轻量限流（可复用既有 limiter 模式），防适配器异常刷量。
- **出站**：同步 HTTP，10s 超时，3 次指数退避重试（1s/2s/4s，仅对连接错误/5xx 重试；4xx 不重试——协议/配置错误）；最终失败返回 error，引擎记日志不中断 run。
- **不做持久化 outbox**（进程崩溃时在途出站消息丢失）——v1 接受，列为后续增强。

## 9. 测试策略（TDD）

- **webhook 包单测**：
  - 签名校验：正确签名通过、错误密钥 401、过期时间戳 400。
  - 入站：纯文本 → 组装 Inbound 正确；图片 base64 → InboundFile；图片 url → httptest 服务器提供图片、下载成 InboundFile；幂等键重复去重；缺 account/peer 报错。
  - 出站：SendText/SendMedia 产生正确 payload 与签名头（用 httptest 适配器服务器收包断言）；重试（服务器前两次 500 第三次 200）；4xx 不重试。
  - `Source()` 返回实例 source；多实例 source 互不相同。
- **config / bootstrap 测试**：
  - `ChannelConfig.Name` 解析；重复 name 启动报错；未知 type 报错。
  - 声明式 config 配 2 个 webhook 实例 → 装配 2 个渠道、Router 按 source 各自路由、各自入站路由经 `RegisterRoute` 挂载（httptest 打通入站→引擎）。
  - **微信回归**：省略 channels 段 → 仅装配 weixin 且行为不变；声明式段含 `type: weixin` → 仍正确装配。
- **端到端**：`tests/integration` 用 `examples/im-adapter` 契约（或等价 httptest 适配器）跑文本+图片双向闭环：适配器推入站 → baize 建 run（图片进多模态）→ 回复经出站回到适配器。
- **全量门槛**：`go build ./...`、`go test ./...`（含 integration）、`gofmt -l internal/`、`go vet ./internal/...` 全绿；2A 阶段微信既有测试零修改通过；2B 阶段微信测试改为针对适配器契约的对等测试。

## 9b. 2B：微信适配器迁移与子进程托管

**目标：** 微信从进程内渠道变为"webhook 实例 + `cmd/weixin-adapter` 进程"，核心删除进程内 weixin 渠道，终态一套机制。

**适配器 `cmd/weixin-adapter`（同仓独立二进制，可复用 ilink 代码，不重写）：**
- 复用现有 `internal/channel/weixin/` 的 ilink 客户端、长轮询、二维码登录、CDN 媒体下载、凭据/settings 读写逻辑——把这些从"渠道实现"重构为"适配器内部库"（包可保留为库，但不再 `channel.Register`、不再被核心装配）。
- 长轮询收到消息 → 组装入站 payload（文本 + 媒体；媒体下载后 base64 或给本地 URL）→ 签名 POST baize `/v0/channels/weixin/inbound`。
- 暴露 `/outbound`（收 baize 出站 → ilink SendMessage/传媒体，带 context_token）与 `/admin/*`（扫码登录/状态/登出/启停）。
- 凭据持久化在适配器可读的 creds 目录；**迁移时复用现有 `data/channels/weixin` 凭据，无需重新扫码**。

**白名单职责调整：** 白名单入站强制从 weixin 进程内逻辑**上移为 baize webhook 入站 handler 的通用能力**（按实例 `allowlist` 配置过滤 `peer.id`）；适配器不做白名单、全量转发，由 baize 丢弃非白名单。allowlist 仍由 baize 渠道设置页管理（通用渠道设置）。

**子进程托管（可选，默认部署"装 baize 即用微信"）：**
- webhook 实例 config 支持 `adapter_command`（如 `cmd/weixin-adapter` 的路径/参数）与 `adapter_autostart: true`。
- baize 在装配/Start 该实例时 `exec` 拉起适配器子进程，等待其 `/healthz` 就绪；Stop/关停时终止子进程（纳入现有 closer LIFO）。适配器监听本地回环端口，`outbound_url` 指向它；适配器入站 POST 到 baize（同机即 `http://127.0.0.1:<port>`）。
- 适配器也可独立部署（`adapter_autostart: false`，手工指定 `outbound_url`），与未来第三方 IM 适配器同等待遇。

**核心侧删除/替换（2B 收尾）：**
- `internal/channel/weixin/` 不再注册渠道（移除 `init()` 的 `channel.Register` 与 `Bootstrap`）；ilink 等逻辑仅供 `cmd/weixin-adapter` 使用（或移动到 `cmd/weixin-adapter/internal/`）。
- `api/server_channel_weixin.go` 微信专用 handler 删除，由 §5.3 通用渠道管理代理取代（URL/JSON 字段不变）。
- `configs/minimal.yaml` 默认渠道改为"webhook 类型的 weixin 实例 + autostart 适配器"，保持默认部署行为等价。

**微信功能对等回归清单（2B 必须逐项有测试）：** 扫码登录全流程（start→ticket/qr→轮询→logged_in 落凭据）、登录态 `running/reason/login_required`、登出、启停、白名单非白名单丢弃 + allowlist 热更新、出站【客服】【助手】前缀、HITL 审批通知 + IM 内回复续跑、UI 操作员镜像、context_token 透传、入站图片多模态/附件、出站媒体、历史会话 ID `weixin:...` 与 meta.Source 兼容。

## 10. 涉及文件（预估）

**2A（通用 webhook 渠道）：**
- 新建 `internal/channel/webhook/`：`channel.go`（Channel/Bootstrapper/Source/注册）、`config.go`（实例配置解析）、`protocol.go`（消息 DTO + 签名）、`inbound.go`（入站 handler、验签、幂等、URL 下载、通用白名单过滤）、`outbound.go`（出站 HTTP 客户端、重试）、`admin.go`（管理面代理客户端，2B 用，2A 可先留接口）、`*_test.go`。
- 新建共享签名小包（如 `internal/webhooksig/`）：从 `internal/inbox/verify.go` 抽出 HMAC 签名/验签/时间窗，inbox 与 webhook 共用。
- 修改 `internal/channel/bootstrap.go`：`BuildDeps` 加 `Routes RouteRegistrar`；`channel.go` 加 `RouteRegistrar` 接口。
- 修改 `internal/api/server.go`：实现 `RegisterRoute`（转发 mux）；入站端点豁免控制面 token（渠道 HMAC 鉴权）。
- 修改 `internal/api/`：通用渠道管理代理路由 `/v0/settings/channels/{name}/...`（2A 可先落地路由与代理框架，webhook 渠道 2B 接上适配器 admin）。
- 修改 `internal/config/config.go`：`ChannelConfig` 加 `Name` 字段 + 测试。
- 修改 `internal/bootstrap/bootstrap.go`：`wireChannels` 声明式模式改为按实例遍历（`channel.Open(type, cfg)` 建实例）；省略段路径保持不变；传 `Routes` 进 BuildDeps。
- 新建 `examples/im-adapter/`：`main.go`、`handler.go`、`hmac.go`、`README.md`。
- 新建集成测试 `tests/integration/webhook_channel_test.go`。

**2B（微信迁移）：**
- 新建 `cmd/weixin-adapter/`：独立 main，复用/迁入 ilink 客户端、长轮询、扫码登录、媒体下载、凭据读写；实现 `/outbound`、`/admin/*`、入站 POST 到 baize、`/healthz`。
- 把 `internal/channel/weixin/` 的 ilink 逻辑重构为适配器可复用的库（移除 `channel.Register`/`Bootstrapper`，不再被核心装配）。
- webhook 渠道接管子进程托管（`adapter_command`/`adapter_autostart`，`exec` + 健康检查 + closer 关停）。
- 删除 `api/server_channel_weixin.go` 微信专用 handler（由通用管理代理取代）；删除核心对进程内 weixin 渠道的装配。
- `configs/minimal.yaml`：默认渠道改为 webhook 类型的 `weixin` 实例 + autostart。
- 微信测试迁移为适配器契约对等测试（功能对等回归清单逐项覆盖）。

**文档：** 公开适配器契约文档放 `docs/`（开源可见）；本设计文档留 `docs/superpowers/specs/`（私有）。

## 11. 验收标准

**2A：**
1. 新增一个 IM 无需改 baize 核心：部署适配器 + 加一段 `channels:` 配置即可；`wireChannels`/`outbound`/`runtime` 通用路径零 webhook 专用逻辑（`server.go` 仅新增通用 `RegisterRoute` 与通用渠道管理代理框架）。
2. 多实例：配 2 个 webhook 实例，文本/图片/文件双向互不串话，会话按 source 隔离。
3. 能力对等：入站文本/图片/文件 → 多模态/附件；HITL 审批在 IM 内可续跑；助手回复/客服镜像/通知经出站到达；context_token 透传；通用白名单入站过滤生效。
4. `examples/im-adapter` 独立进程、仅标准库、不 import baize，端到端跑通文本+图片双向。
5. 2A 阶段微信（仍为进程内）零回归：既有微信测试全绿、行为不变。

**2B：**
6. 微信运行在适配器上：`cmd/weixin-adapter` 经 webhook 渠道接入，扫码登录/登出/启停/登录态回显/白名单/收发消息/媒体/context_token/HITL/UI 镜像**逐项功能对等**（回归清单全绿）。
7. baize 核心无进程内 weixin 渠道特例：`internal/channel/weixin` 不再注册/装配，`server_channel_weixin.go` 删除，通用管理代理接管且管理 URL/JSON 字段与前端页面不变；默认部署（autostart）行为等价、现有凭据无需重新扫码。
8. 终态只有一套渠道机制：所有 IM（含微信）都是 webhook 实例 + 适配器。
9. `go test ./...`（含 integration）全绿、gofmt/vet 干净。
