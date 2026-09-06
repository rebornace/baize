# 阶段二设计：进程外 webhook 渠道（IM 插件，任意语言、独立部署）

> 状态：设计稿（仅私有仓 docs/superpowers，不导出公开仓）
> 日期：2026-09-06
> 前置：阶段一「渠道注册表化」已合并（Descriptor 注册表 / 出站 Router / api 渠道句柄表 / bootstrap 通用 `wireChannels` / config 声明式启用）。
> 关联：`docs/superpowers/specs/2026-09-06-plugin-decoupling-architecture.md` 阶段二；`docs/superpowers/plans/2026-09-06-phase1-channel-registry.md`。

## 0. 最高约束（铁律）

**微信（weixin）现有功能与行为零丢失、零回归。** 本阶段所有装配/配置改动必须保证：

- 省略 `channels:` 配置段时，装配结果与阶段一完全一致（装配全部已注册且 `EnabledByDefault=true` 的渠道 = weixin）。
- 微信的登录 / 启停 / 白名单入站强制 / `running`/`reason` 回显 / 出站【客服】【助手】前缀 / HITL 审批续跑 / UI 操作员镜像 / context_token 出站 / convID 格式 `weixin:<account>:<peer>` / meta.Source `"weixin"`，全部由既有测试锁定且全绿。
- 微信的 HTTP 设置面（`server.go` 里 5 条 weixin 路由）**本阶段不迁移**到新的路由钩子，保持原样（避免行为变更）；钩子是**可选接口**，weixin 不实现即不受影响。
- `wireChannels` 改造为"按实例遍历"时，单渠道/省略配置路径必须逐字节等价；以 `tests/integration` + `api/server_channel_weixin_test.go` + `channel/weixin/*_test.go` 全绿为门槛。

## 1. 目标与非目标

**目标：** 新增一个 IM 渠道（飞书/钉钉/Slack…）= 部署一个独立适配器进程（任意语言）+ 加一段配置，**baize 核心零改动、零重编译、不用给核心提 PR**。适配器通过文档化的 JSON-over-HTTP 契约与 baize 双向通信，能力与进程内微信对等（文本 + 图片 + 任意文件双向、HITL 审批、busy、多模态）。

**非目标（YAGNI）：**
- 不做公共 Go SDK（阶段三，暂缓）。
- 不做出站长轮询 pull 传输（本阶段定 push：baize 直推适配器）。
- 不做持久化 outbox / 跨重启可靠投递（v1 用同步 + 有限重试；outbox 列后续增强）。
- 不迁移微信路由到新钩子、不改微信任何行为。
- 不实现真实飞书/钉钉/Slack API 对接——`examples/im-adapter` 只模拟与演示，真实 IM 适配是适配器作者的事。
- 不动 inbox（系统告警 / HITL 审批回调继续走 `POST /v0/inbox/{id}`，职责不变）。

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
- **全量门槛**：`go build ./...`、`go test ./...`（含 integration）、`gofmt -l internal/`、`go vet ./internal/...` 全绿；微信既有测试零修改通过。

## 10. 涉及文件（预估）

- 新建 `internal/channel/webhook/`：`channel.go`（Channel/Bootstrapper/Source/注册）、`config.go`（实例配置解析）、`protocol.go`（DTO + 签名，复用/对齐 inbox verify）、`inbound.go`（入站 handler、验签、幂等、URL 下载）、`outbound.go`（出站 HTTP 客户端、重试）、`*_test.go`。
- 修改 `internal/channel/bootstrap.go`：`BuildDeps` 加 `Routes RouteRegistrar`；`channel.go` 或新文件加 `RouteRegistrar` 接口。
- 修改 `internal/api/server.go`：实现 `RegisterRoute`（转发 mux）。
- 修改 `internal/config/config.go`：`ChannelConfig` 加 `Name` 字段 + 测试。
- 修改 `internal/bootstrap/bootstrap.go`：`wireChannels` 声明式模式改为按实例遍历（`channel.Open(type, cfg)` 建实例）；省略段路径保持不变；传 `Routes` 进 BuildDeps。
- 新建 `examples/im-adapter/`：`main.go`、`handler.go`、`hmac.go`、`README.md`。
- 新建集成测试 `tests/integration/webhook_channel_test.go`（或等价）。
- 文档：公开契约文档放 `docs/`（开源可见），本设计文档留 `docs/superpowers/specs/`（私有）。

## 11. 验收标准

1. 新增一个 IM 无需改 baize 核心：部署适配器 + 加一段 `channels:` 配置即可，`wireChannels`/`outbound`/`runtime`/`server.go` 通用路径零改动（server.go 仅新增通用 `RegisterRoute` 方法，无 webhook 专用逻辑）。
2. 多实例：配 2 个 webhook 实例，文本/图片双向互不串话，会话按 source 隔离。
3. 能力对等微信：入站文本/图片/文件 → 多模态/附件；HITL 审批在 IM 内可续跑；助手回复/客服镜像/通知经出站到达；context_token 透传。
4. `examples/im-adapter` 独立进程、仅标准库、不 import baize，端到端跑通文本+图片双向。
5. **微信零回归**：省略配置与显式配置 weixin 两路径下，微信登录/启停/白名单/running/reason/前缀/HITL/UI 镜像/convID 全部由既有测试锁定且全绿。
6. `go test ./...` 全绿、gofmt/vet 干净。
