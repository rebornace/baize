# 插件解耦架构设计（渠道注册表化 / 进程外渠道 / 公共 SDK）

> 状态：设计稿（仅私有仓 docs/superpowers，不导出公开仓）
> 日期：2026-09-06

## 1. 背景与目标

Baize 已具备多种"不改核心源码即可扩展"的机制，但**消息渠道（尤其主动出站）**仍必须在树内写 Go、改装配、重编译；且所有契约类型都在 `internal/` 下，外部独立 Go module 无法 import。

**目标：** 让"新增一个渠道 / 写一个插件"能独立开发、独立部署、不改动核心源码，与核心发版解耦。

**非目标：**
- 不重写已有的工具/连接器扩展面（OpenAPI / MCP / HTTP sidecar 插件已满足）。
- 不把整个 `internal/` 公开成 SDK；公共契约面保持最小。
- 不引入新消息队列或 RPC 框架；进程外集成继续走 HTTP + 既有 webhook/outbox。

## 2. 现状盘点

### 2.1 已经"不动核心"的扩展面（本次不改）

| 扩展需求 | 机制 | 进程外 |
|---|---|---|
| REST API 工具 | OpenAPI 连接器，运行时贴 spec（`PUT /v0/connectors`，`connector/apply.go`） | 配置即数据 |
| MCP 工具/服务 | MCP 连接器，stdio 子进程或 Streamable HTTP，`tools/list` 自动发现（`connector/mcp`） | 是 |
| 自定义工具插件 | HTTP sidecar：`/healthz`+`/v0/tools`+`/v0/tools/{n}/invoke`（`connector/httpplugin`，范本 `examples/http-plugin`） | 是，任意语言 |
| Agent 技能 | 运行时上传 `.md`/`.zip`（`POST /v0/skills`，`skill/catalog.go`） | 数据/提示词级 |
| 新渠道**入站** | inbox webhook + HMAC（`POST /v0/inbox/{id}`，`internal/inbox`） | 是 |
| 外部复用 baize 能力 | MCP 导出（`/v0/mcp/export`，`mcpexport`）+ 事件 webhook | 是 |

工具类插件"独立开发、装上即用"今天已成立；`examples/http-plugin` 只用标准库、不 import 任何 baize 包，就是外部作者范本。

### 2.2 仍耦合、必须改核心重编译的扩展面

**A. 消息渠道出站（核心缺口）。** `channel.Channel` 接口与注册表（`RegisterChannel/Open/List`）本是 database/sql 式插件设计，weixin 也用 `init()` + blank import 注册（`cmd/baize/main.go:14`）。但：

1. **bootstrap 没走注册表**：`wireWeixinChannel`（`bootstrap.go:556-639`）直接 `weixin.LoadCreds/NewClient/New`，把具体类型塞进 `srv.WeixinILink/WeixinChannel/WeixinRuntime`（`:623-627`），硬绑 `engine.Outbound = ch`（`:629`）。生产代码 `channel.Open/List` 零调用。
2. **source 硬编码 `weixin`**：入站 `runtime.go:87` 会话 ID 前缀、`:91` `Source:"weixin"`；出站 `outbound.go:43` 判定 `meta.Source != "weixin"` 即跳过。
3. **API 专用路由/字段**：weixin settings/login/logout 路由（`server.go:403-407`）与 `srv.Weixin*` 字段专用。
4. **UI 出站入口**：`api/run_start.go:96-98` 直接用 `s.WeixinRuntime/WeixinChannel` 投递 `/ui` 发言。

**B. 无外部 Go 契约包。** 所有接口/DTO（`channel.Channel`、`tool.Invoker`、`httpplugin.ToolDesc/InvokeMeta/InvokeResult`、`inbox.Payload`）都在 `github.com/rebornace/baize/internal/...`。Go `internal/` 规则使外部 module 无法 import，第三方无法写编译期类型安全的 Go 插件，只能照文档手写 JSON-over-HTTP。

## 3. 目标架构（三阶段）

```
阶段一（internal 内解耦）         阶段二（进程外渠道）             阶段三（公共 SDK）
渠道注册表驱动装配               出站 webhook 渠道适配器         最小公共契约 module
  -> in-tree 新渠道不碰             -> 飞书/钉钉/Slack 独立进程      -> 第三方写 Go 渠道/存储插件
     bootstrap/outbound                不改核心、任意语言             （可选，最后做）
```

- 阶段一是二、三的前置（理顺接缝），风险最低、收益直接。
- 阶段二是"渠道插件独立开发"主交付：不依赖 Go SDK，任意语言、独立部署。
- 阶段三仅在确有第三方要写编译期类型安全 Go 插件时才做，属较大重构。

---

## 阶段一：渠道注册表驱动装配（internal 内解耦）

**目标：** 新增 in-tree 渠道只需"新建 `internal/channel/<name>` 包 + `init()` 注册 + config 声明"，不再改 `bootstrap.go` / `outbound.go` / `runtime.go` / `api/server.go` 的通用路径；weixin 改为注册表的一个实例，行为完全不变。

### P1.1 渠道自报 source，去掉硬编码

**文件：**
- 修改：`internal/channel/channel.go`（接口扩展）
- 修改：`internal/channel/runtime.go:87,91`（前缀/source 参数化）
- 修改：`internal/channel/outbound.go:36-51`（`weixinPeer` 泛化）
- 修改：`internal/channel/weixin/channel.go`（实现新方法）

**接口扩展（可选接口，向后兼容）：** 在 `channel.go` 增加

```go
// SourceSourced is implemented by channels whose conversations carry a
// distinct meta.Source / conv-id prefix (weixin today; feishu tomorrow).
type SourceSourced interface {
	Source() string // meta.Source value, e.g. "weixin"
}
```

`Runtime` 增加字段 `Source string`（空则默认 `"weixin"` 保持兼容）；`HandleInbound` 用它：

```go
src := strings.TrimSpace(r.Source)
if src == "" { src = "weixin" }
convID := src + ":" + account + ":" + peerID
// ...
Source: src,
```

`outbound.go` 把 `weixinPeer` 改为 `channelPeer(ch, meta)`：不再判 `meta.Source != "weixin"`，改为"ch 实现了 SourceSourced 且 `meta.Source == ch.Source()` 且 `meta.ChannelPeer` 非空"才投递；`startedReporter` 判定保留。weixin `Channel` 增加 `func (c *Channel) Source() string { return "weixin" }`。

> 注意：`DeliverUserText/DeliverAssistantReply` 签名不变；多渠道路由在 P1.3 用路由器解决，此处只去掉 source 字面量。

### P1.2 渠道描述符 + 注册表增强

**文件：**
- 修改：`internal/channel/registry.go`
- 创建：`internal/channel/descriptor.go`

注册表目前只存 `Factory func(Config) (Channel, error)`。增加**描述符**，让装配层能通用地遍历、读取默认配置目录名等：

```go
// Descriptor declares a channel's static metadata to the bootstrap layer.
type Descriptor struct {
	Name     string // "weixin"
	Build    func(Config) (Channel, error)
	// DefaultCredsDir is the per-channel settings/creds directory basename.
	DefaultCredsDir string
}

func Register(desc Descriptor)            // 新；RegisterChannel 保留为薄封装
func Describe(name string) (Descriptor, bool)
func Descriptors() []Descriptor           // 按名排序
```

weixin 改为 `channel.Register(channel.Descriptor{Name:"weixin", Build: openFromConfig, DefaultCredsDir: api.DefaultWeixinCredsDir})`。

### P1.3 出站路由器

**文件：** 创建 `internal/channel/router.go`

引擎与 UI 出站当前持有单个 `engine.Outbound channel.Channel`。引入多渠道路由器，它本身实现 `channel.Channel`（`Name()="router"`，`Start/Stop` 扇出），并按 `meta.Source` 选子渠道：

```go
type Router struct {
	mu       sync.RWMutex
	bySource map[string]Channel // "weixin" -> ch
}
func (r *Router) Add(c Channel)            // 若 c 实现 SourceSourced，按 Source() 注册
func (r *Router) SendText(ctx, peer, text, extras) error  // 由 extras["__source"] 选路
```

投递辅助改为：`DeliverAssistantReply(ctx, r, meta, ...)` 内部用 `meta.Source` 从路由器取子渠道再发送（路由器也提供 `For(meta) (Channel, bool)`）。`engine.Outbound` 与 `run_start.go` 改持有 `*channel.Router`；单渠道时路由器只含 weixin，行为不变。

### P1.4 通用化 bootstrap 装配

**文件：**
- 修改：`internal/bootstrap/bootstrap.go:556-639`（`wireWeixinChannel` → `wireChannels`）
- 修改：`internal/api/server.go`（`Weixin*` 专用字段收敛为渠道句柄表；weixin 专用 settings/login 路由暂保留，见下）

把 `wireWeixinChannel` 重构为遍历 `channel.Descriptors()`：

```go
func wireChannels(srv *api.Server, deps channelDeps) (*channel.Router, error) {
	router := channel.NewRouter()
	for _, desc := range channel.Descriptors() {
		ch, rt, err := wireOneChannel(desc, deps)   // 通用：建 Runtime、open、allowlist、start
		if err != nil { return nil, err }
		router.Add(ch)
		srv.RegisterChannel(desc.Name, ch, rt)      // 替代 srv.Weixin* 专用字段
	}
	engine.Outbound = router
	engine.OutboundExtras = routerExtras(router)    // 按 convID 的 source 选 rt.OutboundExtras
	return router, nil
}
```

- `Runtime` 的公共字段（Runs/Meta/Messages/Assignee/DefaultAgentID/SupportsVision/AfterCreateRun/ResumeHITL/Source）对所有渠道一致，通用装配可建；weixin 特有的 ilink/credsDir/login 通过**可选接口**暴露（`LoginCapable`、`CredsDirCapable`），API 层做类型断言——weixin 专用 settings/login 路由本阶段保留、通过 `srv.Channel("weixin")` 取句柄，不再用 `srv.Weixin*` 字段。
- 每个渠道的 `assignee/agentID/enabled/allowlist` 来自各自 settings 文件（目录 `DefaultCredsDir`），通用加载。

### P1.5 config 声明式启用（可选但建议）

**文件：** 修改 `internal/config/config.go`（新增 `Channels []ChannelConfig`）

```go
type ChannelConfig struct {
	Type    string            `yaml:"type"`     // "weixin"
	Enabled bool              `yaml:"enabled"`
	Config  map[string]string `yaml:"config"`   // creds_dir/base_url 覆盖
}
```

`wireChannels` 遍历 `cfg.Channels`（缺省向后兼容：未配置时若注册了 weixin 则按今天的默认行为启用 weixin）。

**阶段一验收：** weixin 全量行为（登录/启停/白名单/出站前缀/HITL/UI 镜像）不变；新增一个 in-tree 测试渠道（`internal/channel/stub`，仅测试用或示例）证明"零改动 bootstrap"即可收发。测试：`internal/channel/*_test.go`、`internal/bootstrap/*_test.go`、`tests/integration`。

---

## 阶段二：出站 webhook 渠道（进程外 IM 适配器，主交付）

**目标：** 飞书/钉钉/Slack 等新 IM 渠道做成**独立部署的小进程**（任意语言），不改核心、不重编译 baize。入站复用已有 inbox webhook；出站新增"出站渠道 webhook"——把助手回复/主动消息作为事件投递给适配器，适配器调各 IM API。

### 设计

核心新增一个**进程外出站渠道**，实现 `channel.Channel`：

- `SendText/SendMedia` 不直接调 IM API，而是**签名 POST** 到适配器 URL（复用 `webhook/outbox` 的重试/死信，或 `plugincallback` 同款 HMAC）。
- 它不做长轮询入站；入站由适配器反向调 baize 既有 `POST /v0/inbox/{id}`（HMAC）。
- 会话 meta：入站 inbox 已建会话（`resolveConversation` + external_id 线程映射）。出站时按会话绑定的"出站渠道 + peer"投递——需要 inbox 渠道配置里增加 `outbound_webhook_url`（适配器地址）与把 `meta.Source` 标记为该渠道（如 `im:feishu`），阶段一的 `SourceSourced`/路由器据此选到这个进程外渠道。

**适配器契约（任意语言实现，文档化 JSON-over-HTTP）：**

| 方向 | 端点 | 载荷 |
|---|---|---|
| 入站（适配器→baize） | `POST /v0/inbox/{channel_id}`（已有，HMAC） | `{action:"create_run", input, external_id, ...}` |
| 出站（baize→适配器） | 适配器提供 `POST /outbound`（HMAC 头 `X-Baize-Protocol:v0`） | `{conversation_id, peer, text, media?, run_id, kind:"assistant"|"operator"}` |
| 回执（适配器→baize，可选） | 复用 plugin-callback 或 webhook 投递状态 | 投递成功/失败 |

**文件（阶段二，in-tree 部分很小）：**
- 创建：`internal/channel/webhookout/channel.go`（实现 `channel.Channel`，出站走签名 HTTP；`init()` 注册为渠道类型 `webhook`）
- 修改：`internal/inbox/model.go`（渠道增加 `OutboundWebhookURL`/`OutboundSecret` 字段）、`internal/api/server_inbox.go`（透传配置、入站 meta.Source 标记）
- 修改：阶段一的 `wireChannels` 能为启用了出站 URL 的 inbox 渠道装配一个 webhookout 渠道进 Router
- 文档：`docs/` 新增适配器契约说明；`examples/` 可加一个 `im-adapter/` 范本（Node/Go，只依赖标准库风格）

**验收：** 用一个示例适配器（examples）端到端：适配器收消息→POST inbox→baize 建 run→回复经出站 webhook 回到适配器→适配器打印/转发；适配器进程独立、可用任意语言、不 import baize。

---

## 阶段三：最小公共契约 module（可选，第三方 Go 插件）

**目标：** 第三方可用 Go 写编译期类型安全的渠道/（未来）存储插件。仅在确有外部 Go 插件需求时做。

**方案：** 新建 `sdk/`（同仓子 module `github.com/rebornace/baize/sdk`，go.mod 独立）或顶层 `pkg/`，**只放接口 + DTO + 注册助手，不含实现**：

- `sdk/channel`：`Channel`、`Inbound`、`InboundFile`、`Config`、`Factory`、`Register`、`SourceSourced`（从 `internal/channel` 上移契约，`internal/channel` 改为 type-alias / re-export 自 sdk，保证 in-tree 代码不破）。
- `sdk/httpplugin`：`ToolDesc`、`InvokeMeta`、`InvokeResult`（sidecar 契约类型，供外部 Go 插件复用，替代手写 JSON）。
- 其余（store/tool 引擎内部）**暂不公开**，YAGNI。

`internal/channel` 保留实现与 Runtime（含对 store/conversation/llm 的依赖，这些不进 sdk）；sdk 只依赖标准库 + 最小 DTO，保证外部 module 能轻量 import。外部插件仓库 `import "github.com/rebornace/baize/sdk/channel"`，写 `init(){ channel.Register(...) }`，通过阶段二的进程外方式接入（Go 插件同样推荐进程外，不做 `.so` 热加载——Go 插件跨版本/编译约束多，YAGNI）。

> 关键取舍：即便有了 sdk，**渠道仍推荐走阶段二的进程外 HTTP 契约**（语言无关、部署解耦、版本独立）。sdk 主要价值是给 sidecar 工具插件和 in-tree 贡献者提供类型安全；不承诺进程内 Go `.so` 插件。

---

## 4. 风险与兼容性

- **行为回归（阶段一）**：weixin 是唯一现存渠道，重构装配必须保持其登录/启停/白名单/出站前缀/HITL/UI 镜像完全不变。对策：行为锁定测试先行（现有 `channel/*_test.go`、`api/server_channel_weixin_test.go`、`tests/integration` 全绿为门槛），纯结构性重构、不改 wire 协议。
- **source 前缀迁移**：会话 ID `"weixin:..."` 已落库（meta.ID）。阶段一 `Source` 空值默认 `"weixin"`，**不改既有会话 ID 格式**，避免历史会话失联。
- **多渠道路由**：`Router` 必须在单渠道（现状）下与直连行为逐字节一致；`OutboundExtras` 按会话 source 选对应 Runtime。
- **公开仓**：本设计与阶段一/二代码若开源，`docs/superpowers` 不导出；契约文档放公开 `docs/`。
- **YAGNI**：阶段三没有真实外部 Go 插件需求前不启动；不做 `.so` 热加载、不公开 store/engine 内部。

## 5. 建议落地顺序

1. **阶段一**（P1.1→P1.5）纯重构 + 注册表化，风险可控，先合并；产出"新增 in-tree 渠道零改动装配"。
2. **阶段二** webhookout 渠道 + 一个 `examples/im-adapter` 范本，打通"独立进程 IM 渠道"。
3. **阶段三** 按需抽取 sdk，配合阶段二契约提供类型安全。

> 详细 TDD 任务拆解（每任务的测试/实现/提交步骤）在进入某一阶段时，按 writing-plans 规范另出 `docs/superpowers/plans/` 计划文档；本文件是架构决策与边界。

