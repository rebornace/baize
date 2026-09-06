# 阶段一：渠道注册表驱动装配 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）跟踪进度。

**目标：** 把消息渠道从"bootstrap 硬编码 weixin + source 字面量"重构为"注册表/描述符驱动 + 渠道路由器"，使新增 in-tree 渠道不再改 `bootstrap.go`/`outbound.go`/`runtime.go`/`api` 通用路径，且 **weixin 行为逐字节不变**。

**架构：** ① `Runtime` 增加 `Source` 字段（空值默认 `"weixin"`，会话 ID 前缀与 meta.Source 参数化）；② 渠道可选实现 `SourceSourced` 接口自报 source；出站投递按 `meta.Source` 选渠道（替代写死的 `weixinPeer`）；③ 新增 `channel.Router`（实现 `channel.Channel`），按 source 持有多个渠道；④ 注册表增加 `Descriptor`；⑤ `api.Server` 用渠道句柄表替代 5 个 `Weixin*` 字段；⑥ bootstrap 遍历 `channel.Descriptors()` 通用装配。

**技术栈：** Go 1.25、标准库 `testing`、`github.com/rebornace/baize/internal/...`。TDD：每个任务先写失败测试 → 跑红 → 最小实现 → 跑绿 → commit。

**铁律：** 每个任务结束后 `go build ./... && go test ./internal/channel/... ./internal/api/... ./internal/bootstrap/... ./tests/integration/...` 全绿。weixin 是唯一现存渠道，任何任务都不得改变其登录/启停/白名单/出站前缀/HITL/UI 镜像行为。

参考设计：`docs/superpowers/specs/2026-09-06-plugin-decoupling-architecture.md`（阶段一 P1.1–P1.5）。

## 文件结构

- `internal/channel/channel.go`：新增 `SourceSourced` 可选接口、`ConvID`/`SourceFromConvID` 助手。
- `internal/channel/registry.go`：新增 `Descriptor`、`Register`、`Describe`、`Descriptors`（任务 3）。
- `internal/channel/descriptor.go`：Descriptor 类型与 BuildDeps（任务 3、5）。
- `internal/channel/router.go`：**新建**，多渠道路由器 `Router`（任务 2）。
- `internal/channel/runtime.go`：`Runtime` 增加 `Source` 字段；`HandleInbound` 用它拼会话 ID/meta（任务 1）。
- `internal/channel/outbound.go`：`weixinPeer` → `resolveChannel`，按 `meta.Source` 选渠道（任务 1、2）。
- `internal/channel/weixin/channel.go`：实现 `Source()`；openFromConfig 自读 settings 应用 allowlist/enabled（任务 1、5）。
- `internal/channel/weixin/settings.go`：**新建**，把 settings JSON DTO + load/save 从 api 移入（任务 5）。
- `internal/api/server.go`：5 个 `Weixin*` 字段 → 通用渠道句柄表（任务 4）。
- `internal/api/server_channel_weixin.go`、`run_start.go`：改用句柄表/路由器（任务 4）。
- `internal/bootstrap/bootstrap.go`：`wireWeixinChannel` → 遍历 `channel.Descriptors()` 的 `wireChannels`（任务 5）。

---

## 任务 1：Runtime.Source 参数化 + 出站按 source 解析（去 "weixin" 字面量）

**文件：**
- 修改：`internal/channel/runtime.go:37-53`（Runtime 加字段）、`:87-93`（convID/meta）
- 修改：`internal/channel/channel.go`（加 `SourceSourced` 接口与 conv-id 助手）
- 修改：`internal/channel/weixin/channel.go`（加 `Source()`）
- 测试：`internal/channel/runtime_test.go`（新增）
- 注：`outbound.go` 的 source 解析泛化在任务 2 与 Router 一起做；本任务后 weixin 出站仍走原 `weixinPeer`（Source 空默认 weixin，行为不变）。

- [ ] **步骤 1：写失败测试 —— Runtime 使用自定义 Source 拼会话 ID**

在 `internal/channel/runtime_test.go`（若不存在则新建，包 `channel`，复用 `fakeRuns`/`newTestRuntime` 风格的内存夹具）加：

```go
func TestHandleInboundUsesRuntimeSource(t *testing.T) {
	runs := &fakeRuns{active: map[string]bool{}}
	meta := conversation.NewMemoryStore()
	rt := &Runtime{
		Runs: runs, Meta: meta, Messages: meta,
		Assignee: "alice", DefaultAgentID: "agent-1",
		Source: "feishu", // 非 weixin 的渠道
	}
	in := Inbound{PeerID: "p1", Text: "hi", Extras: map[string]string{"account": "acc"}}
	if err := rt.HandleInbound(context.Background(), &fakeChannel{name: "feishu"}, in); err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	m, err := meta.GetMeta("feishu:acc:p1")
	if err != nil {
		t.Fatalf("expected conv id feishu:acc:p1, err=%v", err)
	}
	if m.Source != "feishu" {
		t.Fatalf("meta.Source=%q want feishu", m.Source)
	}
}

func TestConvIDHelpers(t *testing.T) {
	id := ConvID("weixin", "acc", "peer")
	if id != "weixin:acc:peer" {
		t.Fatalf("ConvID=%q", id)
	}
	if src := SourceFromConvID(id); src != "weixin" {
		t.Fatalf("SourceFromConvID=%q", src)
	}
	if src := SourceFromConvID("ui:abc"); src != "ui" {
		t.Fatalf("ui prefix=%q", src)
	}
}
```

- [ ] **步骤 2：跑红**

运行：`go test ./internal/channel/ -run 'TestHandleInboundUsesRuntimeSource|TestConvIDHelpers' -v`
预期：FAIL（`Runtime.Source` 未定义、`ConvID` 未定义；且 `HandleInbound` 仍产出 `weixin:acc:p1`）。

- [ ] **步骤 3：最小实现**

`internal/channel/channel.go` 末尾加：

```go
// SourceSourced is implemented by channels whose conversations carry a distinct
// meta.Source / conv-id prefix (weixin today; feishu/dingtalk tomorrow).
type SourceSourced interface {
	Source() string // meta.Source value, e.g. "weixin"
}

// ConvID builds a channel conversation id "<source>:<account>:<peer>".
func ConvID(source, account, peer string) string {
	return source + ":" + account + ":" + peer
}

// SourceFromConvID returns the prefix before the first ":" (the channel source).
func SourceFromConvID(convID string) string {
	if i := strings.IndexByte(convID, ':'); i >= 0 {
		return convID[:i]
	}
	return convID
}
```
（`channel.go` 需 import `"strings"`。）

`internal/channel/runtime.go`：`Runtime` 结构体在 `ResumeHITL` 字段后加：

```go
	// Source is the meta.Source / conv-id prefix for this channel (e.g. "weixin").
	// Empty defaults to "weixin" to preserve historical conversation ids.
	Source string
```

`HandleInbound` 内（`runtime.go:87-93`）把写死的两处改为：

```go
	src := strings.TrimSpace(r.Source)
	if src == "" {
		src = "weixin"
	}
	convID := ConvID(src, account, peerID)
	if err := r.Meta.EnsureMeta(conversation.Meta{
		ID:          convID,
		OwnerID:     assignee,
		Source:      src,
		ChannelPeer: peerID,
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("channel: ensure meta: %w", err)
	}
```

`internal/channel/weixin/channel.go` 加方法（紧邻 `Name()`）：

```go
// Source returns the meta.Source / conv-id prefix for weixin conversations.
func (c *Channel) Source() string { return "weixin" }
```

- [ ] **步骤 4：跑绿（含既有 weixin 测试不回归）**

运行：`go test ./internal/channel/... -v`
预期：PASS。既有 `runtime_test`/weixin `channel_test`（会话 id 仍为 `weixin:...`，因 Source 空默认 weixin）全绿。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/
git commit -m "refactor(channel): 参数化 Runtime.Source 与会话ID前缀，去硬编码 weixin"
```

---

## 任务 2：出站路由器 Router + 投递按 meta.Source 选渠道

**文件：**
- 创建：`internal/channel/router.go`
- 修改：`internal/channel/outbound.go:36-51`（`weixinPeer` 泛化为"按 source 在 Router 里选渠道"）
- 测试：`internal/channel/router_test.go`（新建）、`internal/channel/outbound_test.go`

设计：引擎/UI 仍只持有一个 `channel.Channel`（类型不变），但该实例是 `*Router`。`DeliverAssistantReply/DeliverUserText` 的签名不变；内部把"从单一 ch 判 source"改为"若 ch 是 `*Router` 则按 `meta.Source` 取子渠道，否则用 ch 本身（向后兼容测试里的裸 fake）"。

- [ ] **步骤 1：写失败测试 —— Router 按 source 路由**

创建 `internal/channel/router_test.go`（包 `channel`）：

```go
package channel

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

// sourcedFake 是带 Source() 的 fakeChannel。
type sourcedFake struct {
	fakeChannel
	source string
}

func (s *sourcedFake) Source() string { return s.source }

func TestRouterRoutesByMetaSource(t *testing.T) {
	wx := &sourcedFake{fakeChannel: fakeChannel{name: "wx"}, source: "weixin"}
	fs := &sourcedFake{fakeChannel: fakeChannel{name: "fs"}, source: "feishu"}
	r := NewRouter()
	r.Add(wx)
	r.Add(fs)

	// weixin 会话的回复只发到 wx
	DeliverAssistantReply(context.Background(), r, conversation.Meta{
		Source: "weixin", ChannelPeer: "peer-w",
	}, "hi", nil, nil)
	if n := len(wx.texts()); n != 1 {
		t.Fatalf("wx SendText=%d want 1", n)
	}
	if n := len(fs.texts()); n != 0 {
		t.Fatalf("feishu should not receive, got %d", n)
	}

	// feishu 会话的回复只发到 fs
	DeliverAssistantReply(context.Background(), r, conversation.Meta{
		Source: "feishu", ChannelPeer: "peer-f",
	}, "yo", nil, nil)
	if n := len(fs.texts()); n != 1 {
		t.Fatalf("fs SendText=%d want 1", n)
	}
}

func TestRouterUnknownSourceSkips(t *testing.T) {
	r := NewRouter()
	r.Add(&sourcedFake{fakeChannel: fakeChannel{name: "wx"}, source: "weixin"})
	// 未知 source（如纯 ui 会话）：不投递、不 panic
	DeliverUserText(context.Background(), r, conversation.Meta{Source: "ui"}, "x", nil)
	DeliverAssistantReply(context.Background(), r, conversation.Meta{Source: "dingtalk", ChannelPeer: "p"}, "y", nil, nil)
}

func TestRouterSingleWeixinBehavesLikeDirect(t *testing.T) {
	// 单渠道（现状）：Router 只含 weixin，weixin 会话正常投递
	wx := &sourcedFake{fakeChannel: fakeChannel{name: "wx"}, source: "weixin"}
	r := NewRouter()
	r.Add(wx)
	DeliverAssistantReply(context.Background(), r, conversation.Meta{
		ID: "weixin:acc:p", Source: "weixin", ChannelPeer: "p",
	}, "回复", nil, nil)
	sent := wx.texts()
	if len(sent) != 1 || sent[0].text != OutboundPrefixAssistant+"回复" {
		t.Fatalf("single-router deliver = %+v", sent)
	}
}
```

- [ ] **步骤 2：跑红**

运行：`go test ./internal/channel/ -run TestRouter -v`
预期：FAIL（`NewRouter`/`Add` 未定义；`Deliver*` 还不会按 source 路由，feishu 断言失败）。

- [ ] **步骤 3：最小实现 —— Router**

创建 `internal/channel/router.go`：

```go
package channel

import (
	"context"
	"sync"
)

// Router is a Channel that fans out to registered child channels, selecting
// the outbound target by conversation meta.Source. It implements Channel so the
// engine/UI can hold it as a single Outbound. Inbound (Start/Stop) is fanned out
// to all children; SendText/SendMedia are routed via Deliver* helpers (which use
// Router.For), so their direct invocation is a no-op (no implicit source).
type Router struct {
	mu       sync.RWMutex
	bySource map[string]Channel
	order    []Channel
	runtimes map[string]*Runtime // source -> Runtime（用于 Extras 解析 context_token）
}

// NewRouter returns an empty channel router.
func NewRouter() *Router {
	return &Router{bySource: map[string]Channel{}, runtimes: map[string]*Runtime{}}
}

// Add registers a child. Channels implementing SourceSourced are keyed by
// Source(); others are retained only for Start/Stop fan-out (not routable).
func (r *Router) Add(c Channel) {
	if c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, c)
	if ss, ok := c.(SourceSourced); ok {
		if src := ss.Source(); src != "" {
			r.bySource[src] = c
		}
	}
}

// For returns the channel responsible for a conversation source.
func (r *Router) For(source string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.bySource[source]
	return c, ok
}

// Name identifies the router.
func (r *Router) Name() string { return "router" }

// Start fans out to all children (in-process channels start their own loops).
func (r *Router) Start(ctx context.Context) error {
	r.mu.RLock()
	chans := append([]Channel(nil), r.order...)
	r.mu.RUnlock()
	for _, c := range chans {
		if err := c.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Stop fans out to all children.
func (r *Router) Stop(ctx context.Context) error {
	r.mu.RLock()
	chans := append([]Channel(nil), r.order...)
	r.mu.RUnlock()
	var firstErr error
	for _, c := chans {
		if err := c.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SendText/SendMedia on the Router itself are no-ops: outbound delivery always
// goes through Deliver* which selects the child by meta.Source. Kept to satisfy
// the Channel interface.
func (r *Router) SendText(ctx context.Context, peerID, text string, extras map[string]string) error {
	return nil
}

func (r *Router) SendMedia(ctx context.Context, peerID, filename, mime string, data []byte, extras map[string]string) error {
	return nil
}

// BindRuntime registers a channel Runtime keyed by its Source (default "weixin")
// so Extras can resolve per-conversation context tokens.
func (r *Router) BindRuntime(rt *Runtime) {
	if rt == nil {
		return
	}
	src := strings.TrimSpace(rt.Source)
	if src == "" {
		src = "weixin"
	}
	r.mu.Lock()
	if r.runtimes == nil {
		r.runtimes = map[string]*Runtime{}
	}
	r.runtimes[src] = rt
	r.mu.Unlock()
}

// Extras returns per-conversation outbound extras from the Runtime owning the
// conversation's source (used as engine.OutboundExtras).
func (r *Router) Extras(conversationID string) map[string]string {
	src := SourceFromConvID(conversationID)
	r.mu.RLock()
	rt := r.runtimes[src]
	r.mu.RUnlock()
	if rt == nil {
		return nil
	}
	return rt.OutboundExtras(conversationID)
}
```
（Router 结构体加字段 `runtimes map[string]*Runtime`，并在 `NewRouter` 初始化；`router.go` 需 import `"strings"`。）

加测试（`router_test.go`）：

```go
func TestRouterExtrasByConvSource(t *testing.T) {
	r := NewRouter()
	rt := &Runtime{Source: "weixin"}
	rt.rememberContextToken("weixin:acc:p1", map[string]string{"context_token": "tok-1"})
	r.BindRuntime(rt)
	if got := r.Extras("weixin:acc:p1"); got["context_token"] != "tok-1" {
		t.Fatalf("extras=%v", got)
	}
	if got := r.Extras("feishu:x:y"); got != nil {
		t.Fatalf("unknown source extras=%v want nil", got)
	}
}
```

- [ ] **步骤 4：最小实现 —— outbound.go 按 source 选渠道**

把 `outbound.go` 的 `weixinPeer(ch, meta)` 替换为 `resolveOutbound(ch, meta) (Channel, string, bool)`：

```go
// resolveOutbound picks the channel + peer for a conversation. When ch is a
// *Router it selects the child matching meta.Source; otherwise ch is used
// directly (back-compat for tests/nil). Returns false for ui-only / unknown.
func resolveOutbound(ch Channel, meta conversation.Meta) (Channel, string, bool) {
	if ch == nil {
		return nil, "", false
	}
	if r, ok := ch.(*Router); ok {
		c, ok := r.For(strings.TrimSpace(meta.Source))
		if !ok {
			return nil, "", false
		}
		ch = c
	} else {
		// 非 Router：保持原语义——只有渠道自报 source 与 meta.Source 一致才投递。
		if ss, ok := ch.(SourceSourced); ok {
			if strings.TrimSpace(meta.Source) != ss.Source() {
				return nil, "", false
			}
		} else if strings.TrimSpace(meta.Source) != "weixin" {
			return nil, "", false
		}
	}
	if sr, ok := ch.(startedReporter); ok && !sr.IsStarted() {
		return nil, "", false
	}
	peer := strings.TrimSpace(meta.ChannelPeer)
	if peer == "" {
		return nil, "", false
	}
	return ch, peer, true
}
```

`DeliverUserText` 与 `DeliverAssistantReply` 内把：

```go
	peer, ok := weixinPeer(ch, meta)
	if !ok {
		return
	}
```

改为：

```go
	target, peer, ok := resolveOutbound(ch, meta)
	if !ok {
		return
	}
```

并把后续 `ch.SendText(...)` / `ch.SendMedia(...)` 的接收者改为 `target`（`DeliverUserText` 一处 `ch.SendText`；`DeliverAssistantReply` 一处 `ch.SendText` + 一处 `ch.SendMedia`）。删除旧 `weixinPeer` 函数。

- [ ] **步骤 5：跑绿**

运行：`go test ./internal/channel/... -v`
预期：PASS。既有 `outbound_test.go` 用裸 `fakeChannel`（无 Source()）+ `meta.Source=="weixin"` → 走 else 分支默认 weixin 语义，仍投递；`Source:"ui"` 仍跳过。新增 Router 测试全绿。

- [ ] **步骤 6：Commit**

```bash
git add internal/channel/router.go internal/channel/router_test.go internal/channel/outbound.go internal/channel/outbound_test.go
git commit -m "feat(channel): 新增出站 Router 并按 meta.Source 选渠道投递"
```

---

## 任务 3：注册表 Descriptor（元数据驱动）

**文件：**
- 修改：`internal/channel/registry.go`（加 `Descriptor`、`Register`、`Describe`、`Descriptors`）
- 修改：`internal/channel/weixin/channel.go:18-20`（`init` 改用 `Register`）
- 测试：`internal/channel/registry_test.go`（新建）

- [ ] **步骤 1：写失败测试**

创建 `internal/channel/registry_test.go`（包 `channel`）：

```go
package channel

import (
	"context"
	"testing"
)

type stubChannel struct{ name string }

func (s *stubChannel) Name() string                                                    { return s.name }
func (s *stubChannel) Start(ctx context.Context) error                                  { return nil }
func (s *stubChannel) Stop(ctx context.Context) error                                   { return nil }
func (s *stubChannel) SendText(ctx context.Context, p, t string, e map[string]string) error  { return nil }
func (s *stubChannel) SendMedia(ctx context.Context, p, f, m string, d []byte, e map[string]string) error {
	return nil
}

func TestRegisterAndDescribe(t *testing.T) {
	resetRegistryForTest()
	Register(Descriptor{
		Name:            "stub",
		Build:           func(Config) (Channel, error) { return &stubChannel{name: "stub"}, nil },
		DefaultCredsDir: "./data/channels/stub",
	})
	desc, ok := Describe("stub")
	if !ok || desc.DefaultCredsDir != "./data/channels/stub" {
		t.Fatalf("Describe=%+v ok=%v", desc, ok)
	}
	ch, err := Open("stub", nil)
	if err != nil || ch.Name() != "stub" {
		t.Fatalf("Open ch=%v err=%v", ch, err)
	}
	names := Descriptors()
	if len(names) != 1 || names[0].Name != "stub" {
		t.Fatalf("Descriptors=%+v", names)
	}
}

func TestRegisterChannelBackCompat(t *testing.T) {
	resetRegistryForTest()
	RegisterChannel("legacy", func(Config) (Channel, error) { return &stubChannel{name: "legacy"}, nil })
	if _, ok := Describe("legacy"); !ok {
		t.Fatal("RegisterChannel should also be discoverable via Describe")
	}
	if got := List(); len(got) != 1 || got[0] != "legacy" {
		t.Fatalf("List=%v", got)
	}
}
```

- [ ] **步骤 2：跑红**

运行：`go test ./internal/channel/ -run 'TestRegisterAndDescribe|TestRegisterChannelBackCompat' -v`
预期：FAIL（`Descriptor`/`Register`/`Describe`/`Descriptors` 未定义）。

- [ ] **步骤 3：最小实现**

`internal/channel/registry.go`：把 `factories map[string]Factory` 改为 `descs map[string]Descriptor`，保留公开 API：

```go
// Descriptor declares a channel's static metadata for the bootstrap layer.
type Descriptor struct {
	// Name is the channel type key (e.g. "weixin").
	Name string
	// Build constructs a Channel from opaque Config.
	Build func(Config) (Channel, error)
	// DefaultCredsDir is the per-channel settings/creds directory basename.
	DefaultCredsDir string
}

var (
	registryMu sync.RWMutex
	descs      = map[string]Descriptor{}
)

// Register registers a channel Descriptor (preferred over RegisterChannel).
func Register(desc Descriptor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if desc.Name == "" {
		panic("channel: empty channel name")
	}
	if desc.Build == nil {
		panic("channel: nil build for " + desc.Name)
	}
	descs[desc.Name] = desc
}

// RegisterChannel registers a bare factory (back-compat); metadata is empty.
func RegisterChannel(name string, factory Factory) {
	Register(Descriptor{Name: name, Build: factory})
}

// List returns registered channel names in sorted order.
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(descs))
	for name := range descs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Describe returns the Descriptor for a channel name.
func Describe(name string) (Descriptor, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	d, ok := descs[name]
	return d, ok
}

// Descriptors returns all registered Descriptors sorted by Name.
func Descriptors() []Descriptor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Descriptor, 0, len(descs))
	for _, d := range descs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Open constructs a Channel by registered name.
func Open(name string, cfg Config) (Channel, error) {
	registryMu.RLock()
	d, ok := descs[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown channel %q", name)
	}
	return d.Build(cfg)
}
```

（删除旧的 `factories` 变量与旧 `RegisterChannel/List/Open` 体；`resetRegistryForTest` 改为 `descs = map[string]Descriptor{}`。）

`internal/channel/weixin/channel.go:18-20` 的 `init` 改为：

```go
func init() {
	channel.Register(channel.Descriptor{
		Name:            "weixin",
		Build:           openFromConfig,
		DefaultCredsDir: defaultCredsDir,
	})
}
```

- [ ] **步骤 4：跑绿**

运行：`go build ./... && go test ./internal/channel/... -v`
预期：PASS（weixin 仍注册为 "weixin"；`TestRegisterChannelWeixin` 等既有测试不回归）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/registry.go internal/channel/registry_test.go internal/channel/weixin/channel.go
git commit -m "feat(channel): 注册表增加 Descriptor 元数据，RegisterChannel 保持兼容"
```

---

## 任务 4：api.Server 渠道句柄表（替代 5 个 Weixin* 专用字段）

**文件：**
- 创建：`internal/api/channels.go`（`ChannelHandle` + Server 注册表）
- 修改：`internal/api/server.go:147-151`（删 5 个 Weixin* 字段，加 `Outbound`/`OutboundExtras`/注册表）
- 修改：`internal/api/server_channel_weixin.go`（全部 `s.WeixinXxx` 改用句柄访问器）
- 修改：`internal/api/run_start.go:82-99`（`deliverWeixinUserOutbound` 改通用，走 Router）
- 修改：`internal/api/server_channel_weixin_test.go:19-44`（`weixinTestServer` 用新注册表）

- [ ] **步骤 1：写失败测试 —— 句柄表注册/读取**

创建 `internal/api/channels_test.go`（包 `api_test`）：

```go
package api_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/store"
)

type noopCh struct{}

func (noopCh) Name() string                                                       { return "noop" }
func (noopCh) Start(context.Context) error                                        { return nil }
func (noopCh) Stop(context.Context) error                                         { return nil }
func (noopCh) SendText(context.Context, string, string, map[string]string) error  { return nil }
func (noopCh) SendMedia(context.Context, string, string, string, []byte, map[string]string) error {
	return nil
}

func TestServerRegisterChannel(t *testing.T) {
	srv := api.NewServer(store.NewMemory(), nil, nil)
	rt := &channel.Runtime{Source: "weixin"}
	srv.RegisterChannel(&api.ChannelHandle{
		Name: "weixin", Channel: noopCh{}, Runtime: rt, CredsDir: "/tmp/wx",
	})
	h, ok := srv.Channel("weixin")
	if !ok || h.CredsDir != "/tmp/wx" || h.Runtime.Source != "weixin" {
		t.Fatalf("handle=%+v ok=%v", h, ok)
	}
	if _, ok := srv.Channel("nope"); ok {
		t.Fatal("unknown channel should not be found")
	}
}
```

- [ ] **步骤 2：跑红**

运行：`go test ./internal/api/ -run TestServerRegisterChannel -v`
预期：FAIL（`api.ChannelHandle`/`RegisterChannel`/`Channel` 未定义；随后步骤会暴露 server.go 的 Weixin* 字段被引用）。

- [ ] **步骤 3：最小实现 —— channels.go**

创建 `internal/api/channels.go`：

```go
package api

import (
	"context"
	"sync"

	"github.com/rebornace/baize/internal/channel"
)

// ChannelHandle is the api layer's per-channel runtime handle. Channel-specific
// handlers (e.g. weixin login) type-assert h.Channel to their concrete type.
type ChannelHandle struct {
	Name     string
	Channel  channel.Channel
	Runtime  *channel.Runtime
	CredsDir string
	RunCtx   context.Context
}

// RegisterChannel registers a channel handle by name.
func (s *Server) RegisterChannel(h *ChannelHandle) {
	if h == nil || h.Name == "" {
		return
	}
	s.channelsMu.Lock()
	if s.channels == nil {
		s.channels = map[string]*ChannelHandle{}
	}
	s.channels[h.Name] = h
	s.channelsMu.Unlock()
}

// Channel returns the handle for a registered channel name.
func (s *Server) Channel(name string) (*ChannelHandle, bool) {
	s.channelsMu.RLock()
	defer s.channelsMu.RUnlock()
	h, ok := s.channels[name]
	return h, ok
}
```

`internal/api/server.go`：
- 删除字段 `WeixinILink`、`WeixinChannel`、`WeixinRuntime`、`WeixinCredsDir`、`WeixinRunCtx`（`:147-151`）。
- 在 Server 结构体加：

```go
	// Outbound is the channel.Router used to deliver replies/UI turns to peers.
	Outbound       channel.Channel
	OutboundExtras func(conversationID string) map[string]string

	channelsMu sync.RWMutex
	channels   map[string]*ChannelHandle
```
（`server.go` 已 import `channel`；确认 `sync` 已 import。）

- [ ] **步骤 4：weixin.Channel 暴露 ILink getter；迁移 server_channel_weixin.go 到句柄**

先在 `internal/channel/weixin/channel.go` 加 getter（紧邻 `HasCredentials`）：

```go
// ILink exposes the iLink client (used by the api login handlers after the
// channel is retrieved generically from the channel registry).
func (c *Channel) ILink() ILink { return c.ilink }
```

在 `server_channel_weixin.go` 加访问器（weixin handler 本就是 weixin 专属，直接断言具体类型）：

```go
func (s *Server) weixinHandle() (*ChannelHandle, *weixin.Channel, bool) {
	h, ok := s.Channel("weixin")
	if !ok {
		return nil, nil, false
	}
	ch, ok := h.Channel.(*weixin.Channel)
	if !ok {
		return nil, nil, false
	}
	return h, ch, true
}
```

逐处替换（保持逻辑不变）：
- 每个 handler 开头：

```go
	h, ch, ok := s.weixinHandle()
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "channel_unavailable", "weixin channel not wired")
		return
	}
```

- `s.WeixinCredsDir` → `dir := strings.TrimSpace(h.CredsDir); if dir == "" { dir = DefaultWeixinCredsDir }`。
- `s.WeixinILink` → `ch.ILink()`（nil 判定：`if ch.ILink() == nil`）。
- `s.WeixinChannel` → `ch`；`s.WeixinRuntime` → `h.Runtime`。
- `s.weixinRunCtx()`：改为 `if h.RunCtx != nil { return h.RunCtx }; return context.Background()`；删除旧 `WeixinRunCtx` 版本。
- `applyWeixinSettings`/`weixinRuntimeState`：改为方法内自行 `h, ch, _ := s.weixinHandle()`（或接收 `h, ch` 参数），用 `ch`/`h.Runtime`；`ch.SetAllowlist(...)`、Start/Stop 逻辑不变。

- [ ] **步骤 5：迁移 run_start.go 为通用出站**

把 `deliverWeixinUserOutbound`（`run_start.go:82-99`）改名为 `deliverUserOutbound` 并改用 Router（调用点同文件内搜索改名）：

```go
func (s *Server) deliverUserOutbound(ctx context.Context, convID, text string) {
	if s == nil || s.Outbound == nil || strings.TrimSpace(text) == "" || strings.TrimSpace(convID) == "" {
		return
	}
	ms := s.metaStore()
	if ms == nil {
		return
	}
	meta, err := ms.GetMeta(convID)
	if err != nil {
		return
	}
	var extras map[string]string
	if s.OutboundExtras != nil {
		extras = s.OutboundExtras(convID)
	}
	channel.DeliverUserText(ctx, s.Outbound, meta, text, extras)
}
```

（`channel.DeliverUserText` 内部 `resolveOutbound` 会按 `meta.Source` 在 Router 里选 weixin 子渠道；ui 会话自动跳过。）

- [ ] **步骤 6：更新 weixinTestServer 夹具**

`server_channel_weixin_test.go:19-44`：把

```go
	srv.WeixinILink = fake
	srv.WeixinChannel = ch
	srv.WeixinRuntime = rt
	srv.WeixinCredsDir = dir
```

改为：

```go
	ch := weixin.New(fake, rt, "", "")
	ch.SetCredsDir(dir)

	srv := api.NewServer(store.NewMemory(), nil, nil)
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"
	// 单渠道测试：Outbound 直接用 ch（resolveOutbound 非 Router 走 source 判定）。
	srv.Outbound = ch
	srv.RegisterChannel(&api.ChannelHandle{
		Name: "weixin", Channel: ch, Runtime: rt, CredsDir: dir,
	})
	return srv, fake, dir
```

（不再需要 `Ext` 字段；登录 handler 经 `ch.ILink()` 取 fake。`ChannelHandle.Ext` 字段本任务可不加，保持最小。）

- [ ] **步骤 7：跑绿**

运行：`go build ./... && go test ./internal/api/... -v`
预期：PASS。所有 weixin settings/login/logout 测试、run 相关测试通过（行为不变）。

- [ ] **步骤 8：Commit**

```bash
git add internal/api/
git commit -m "refactor(api): 用渠道句柄表替代 Weixin* 专用字段，出站走 Router"
```

---

## 任务 5：bootstrap 遍历 Descriptors 通用装配

**文件：**
- 创建：`internal/channel/weixin/settings.go`（把 settings DTO + load/save 从 api 移入）
- 创建：`internal/channel/bootstrap.go`（`BuildDeps` + `Bootstrapper` 可选接口）
- 修改：`internal/channel/weixin/channel.go`（`openFromConfig` 用 `DefaultCredsDir`；实现 `Bootstrap`）
- 修改：`internal/api/server_channel_weixin.go`（settings 类型改用 `weixin.Settings`/`weixin.LoadSettings`/`SaveSettings`）
- 修改：`internal/bootstrap/bootstrap.go:556-639`（`wireWeixinChannel` → `wireChannels` 通用循环）
- 测试：`internal/channel/weixin/channel_test.go`、`internal/bootstrap/bootstrap_test.go`

- [ ] **步骤 1：把微信 settings 移到 weixin 包**

创建 `internal/channel/weixin/settings.go`：把 `internal/api/server_channel_weixin.go` 里的 `WeixinChannelSettings` 结构体（`AgentID/Allowlist/Assignee/Enabled` + json tag）、`loadWeixinSettings`/`saveWeixinSettings`、默认 settings（文件缺失返回 `{Allowlist: []string{}, Enabled: true}`）整体移入，改为导出：

```go
package weixin

const DefaultCredsDir = "./data/channels/weixin"

type Settings struct {
	AgentID   string   `json:"agent_id"`
	Allowlist []string `json:"allowlist"`
	Assignee  string   `json:"assignee"`
	Enabled   bool     `json:"enabled"`
}

func LoadSettings(dir string) (Settings, error) { /* 原 loadWeixinSettings 逻辑，dir 空用 DefaultCredsDir */ }
func SaveSettings(dir string, s Settings) error { /* 原 saveWeixinSettings 逻辑 */ }
```

`api/server_channel_weixin.go`：删本地 DTO/load/save，改用 `weixin.Settings`、`weixin.LoadSettings`、`weixin.SaveSettings`、`weixin.DefaultCredsDir`；`api.DefaultWeixinCredsDir` 常量删除（grep 替换引用为 `weixin.DefaultCredsDir`）。handler 的请求/响应 JSON 字段名不变（`agent_id/allowlist/assignee/enabled/running/reason`）。

- [ ] **步骤 2：写失败测试 —— weixin.Channel 实现 Bootstrap**

在 `internal/channel/weixin/channel_test.go` 加：

```go
func TestChannelBootstrapBuildsRuntime(t *testing.T) {
	dir := t.TempDir()
	// 预置 settings：enabled=false 时不应要求 start
	if err := SaveSettings(dir, Settings{Assignee: "bob", AgentID: "ag1", Enabled: false, Allowlist: []string{"p1"}}); err != nil {
		t.Fatal(err)
	}
	ch := New(NewFake(), nil, "", "")
	ch.SetCredsDir(dir)
	deps := channel.BuildDeps{
		Store:          &fakeRuns{active: map[string]bool{}},
		Meta:           conversation.NewMemoryStore(),
		Messages:       conversation.NewMemoryStore(),
		DefaultAgentID: "default-ag",
	}
	rt, credsDir, start, err := ch.Bootstrap(deps)
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if rt.Source != "weixin" || rt.Assignee != "bob" || rt.DefaultAgentID != "ag1" {
		t.Fatalf("runtime=%+v", rt)
	}
	if credsDir != dir {
		t.Fatalf("credsDir=%q want %q", credsDir, dir)
	}
	if start {
		t.Fatal("start should be false (no creds + disabled)")
	}
}
```

- [ ] **步骤 3：跑红**

运行：`go test ./internal/channel/weixin/ -run TestChannelBootstrap -v`
预期：FAIL（`channel.BuildDeps`、`ch.Bootstrap`、`weixin.SaveSettings` 未定义）。

- [ ] **步骤 4：实现 BuildDeps/Bootstrapper 与 weixin.Bootstrap**

创建 `internal/channel/bootstrap.go`：

```go
package channel

import (
	"context"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// BuildDeps are the generic dependencies every wired channel shares. The
// channel builds its own Runtime (assignee/agent/source from its settings).
type BuildDeps struct {
	Store          RunStore
	Meta           conversation.MetaStore
	Messages       conversation.Store
	DefaultAgentID string
	SupportsVision bool
	AfterCreateRun func(ctx context.Context, run *store.Run, userParts []llm.ContentPart) error
	ResumeHITL     func(ctx context.Context, runID string, approve bool, comment string) error
}

// Bootstrapper is implemented by channels that build their Runtime from
// persisted per-channel settings. Channels not implementing it are registered
// for future use but not started/wired in v1.
type Bootstrapper interface {
	Bootstrap(deps BuildDeps) (rt *Runtime, credsDir string, start bool, err error)
}
```

`internal/channel/weixin/channel.go` 加方法：

```go
// Bootstrap builds the weixin Runtime from persisted settings, applies the
// allowlist, loads credentials, and reports whether polling should start.
func (c *Channel) Bootstrap(deps channel.BuildDeps) (*channel.Runtime, string, bool, error) {
	dir := strings.TrimSpace(c.credsDir)
	if dir == "" {
		dir = DefaultCredsDir
	}
	settings, err := LoadSettings(dir)
	if err != nil {
		return nil, "", false, err
	}
	assignee := strings.TrimSpace(settings.Assignee)
	if assignee == "" {
		assignee = "channel:weixin"
	}
	agentID := strings.TrimSpace(settings.AgentID)
	if agentID == "" {
		agentID = deps.DefaultAgentID
	}
	rt := &channel.Runtime{
		Runs:           deps.Store,
		Meta:           deps.Meta,
		Messages:       deps.Messages,
		Assignee:       assignee,
		DefaultAgentID: agentID,
		SupportsVision: deps.SupportsVision,
		Source:         "weixin",
		AfterCreateRun: deps.AfterCreateRun,
		ResumeHITL:     deps.ResumeHITL,
	}
	c.SetRuntime(rt)
	c.SetAllowlist(settings.Allowlist)

	accountID, token, credErr := LoadCreds(dir)
	if credErr == nil {
		c.SetCredentials(accountID, token)
	}
	start := credErr == nil && settings.Enabled
	return rt, dir, start, nil
}
```

- [ ] **步骤 5：写失败测试 —— wireChannels 通用装配（新渠道零改动 bootstrap）**

在 `internal/bootstrap/bootstrap_test.go` 加一个测试渠道，注册到 channel 注册表，断言 `wireChannels` 无需改 bootstrap 即装配它：

```go
func TestWireChannelsPicksUpRegisteredDescriptor(t *testing.T) {
	channel.Register(channel.Descriptor{
		Name: "stubch",
		Build: func(channel.Config) (channel.Channel, error) { return &stubBootChannel{name: "stubch"}, nil },
	})
	t.Cleanup(channel.ResetForTest) // 见步骤 6 需导出测试重置

	srv := api.NewServer(store.NewMemory(), nil, nil)
	// ... 用内存 conversation.Store、nil provider、最小 engine 调 wireChannels
	router, err := wireChannels(srv, channelDepsForTest(t))
	if err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := router.For("stubch"); !ok {
		t.Fatal("router should route stubch by its Source()")
	}
	if _, ok := srv.Channel("stubch"); !ok {
		t.Fatal("srv should have stubch handle")
	}
}
```
（`stubBootChannel` 实现 `channel.Channel` + `Source() string { return "stubch" }` + `Bootstrap(channel.BuildDeps) (*channel.Runtime, string, bool, error)` 返回 `&channel.Runtime{Source:"stubch", Assignee:"a", DefaultAgentID:"b", Runs:..., Meta:...}, "", false, nil`。`channelDepsForTest` 构造内存 store/messages/engine/runCtx。）

- [ ] **步骤 6：实现 wireChannels 通用循环**

在 `internal/channel/registry.go` 导出测试重置（现有 `resetRegistryForTest` 改为导出 `ResetForTest`，保留旧名内部调用或直接改名并更新测试）。

`internal/bootstrap/bootstrap.go`：用下面的通用函数替换 `wireWeixinChannel`（保留签名被调用处改为 `wireChannels(...)`；调用点原来传 `(srv, st, engine, messages, provider, closer)`，调整为新参数集）：

```go
type channelDeps struct {
	srv            *api.Server
	st             store.Store
	messages       conversation.Store
	engine         *run.Engine
	provider       llm.Provider
	defaultAgentID string
	runCtx         context.Context
	closer         *storeAndMCPCloser
}

func wireChannels(d channelDeps) (*channel.Router, error) {
	meta, ok := d.messages.(conversation.MetaStore)
	if !ok {
		return nil, fmt.Errorf("conversation store does not support meta")
	}
	supportsVision := d.provider != nil && d.provider.SupportsVision()
	router := channel.NewRouter()
	deps := channel.BuildDeps{
		Store:          d.st,
		Meta:           meta,
		Messages:       d.messages,
		DefaultAgentID: d.defaultAgentID,
		SupportsVision: supportsVision,
		AfterCreateRun: func(ctx context.Context, runRec *store.Run, userParts []llm.ContentPart) error {
			d.srv.Dispatch(context.Background(), middleware.Job{
				RunID: runRec.ID, Kind: middleware.KindRun, AgentID: runRec.AgentID,
				Input: runRec.Input, UserParts: api.PartsToMiddleware(userParts),
			})
			return nil
		},
		ResumeHITL: func(ctx context.Context, runID string, approve bool, comment string) error {
			return d.engine.ContinueFromHITL(ctx, runID, run.Decision{Approve: approve, Comment: comment})
		},
	}
	for _, desc := range channel.Descriptors() {
		ch, err := desc.Build(channel.Config{"creds_dir": desc.DefaultCredsDir})
		if err != nil {
			return nil, fmt.Errorf("open channel %s: %w", desc.Name, err)
		}
		handle := &api.ChannelHandle{Name: desc.Name, Channel: ch, CredsDir: desc.DefaultCredsDir, RunCtx: d.runCtx}
		bs, isBoot := ch.(channel.Bootstrapper)
		if isBoot {
			rt, dir, start, err := bs.Bootstrap(deps)
			if err != nil {
				return nil, fmt.Errorf("bootstrap channel %s: %w", desc.Name, err)
			}
			handle.Runtime = rt
			handle.CredsDir = dir
			router.Add(ch)
			router.BindRuntime(rt)
			ch := ch // capture
			d.closer.stops = append(d.closer.stops, func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = ch.Stop(stopCtx)
			})
			if start {
				if err := ch.Start(d.runCtx); err != nil {
					log.Printf("%s channel: start skipped: %v", desc.Name, err)
				} else {
					log.Printf("%s channel: started (creds_dir=%s)", desc.Name, dir)
				}
			}
		}
		d.srv.RegisterChannel(handle)
	}
	d.srv.Outbound = router
	d.srv.OutboundExtras = router.Extras
	d.engine.Meta = meta
	d.engine.Outbound = router
	d.engine.OutboundExtras = router.Extras
	return router, nil
}
```

删除旧 `wireWeixinChannel`；更新 `bootstrap.go` 中其调用点为 `wireChannels(channelDeps{...})`（`defaultAgentID` 用 `srv.DefaultAgentID`，`runCtx` 用原 `runCtx`/`context.WithCancel`）。

- [ ] **步骤 7：跑绿（全量）**

运行：`go build ./... && go test ./internal/channel/... ./internal/api/... ./internal/bootstrap/... ./tests/integration/... -v`
预期：PASS。weixin 登录/启停/白名单/出站/HITL/UI 镜像行为不变；stub 渠道测试证明"新增 in-tree 渠道只需 Register(Descriptor)+实现 Bootstrap，不碰 wireChannels"。

- [ ] **步骤 8：Commit**

```bash
git add internal/ internal/bootstrap/
git commit -m "refactor(bootstrap): 渠道改为 Descriptor/Bootstrapper 通用装配，weixin 成为注册表实例"
```

---

## 任务 6：（可选，建议）config 声明式启用 + 全量回归

**文件：**
- 修改：`internal/config/config.go`（新增 `Channels []ChannelConfig`）
- 修改：`internal/bootstrap/bootstrap.go`（`wireChannels` 按 `cfg.Channels` 过滤；缺省启用已注册渠道保持兼容）
- 修改：`configs/default.yaml`（注释示例，可选）
- 测试：`internal/config/*_test.go`、`internal/bootstrap/bootstrap_test.go`

- [ ] **步骤 1：config 类型 + 测试**

`config.go` 加：

```go
// ChannelConfig declaratively enables/configures a registered channel.
type ChannelConfig struct {
	Type    string            `yaml:"type"`    // channel type key, e.g. "weixin"
	Enabled bool              `yaml:"enabled"` // default: wired when registered
	Config  map[string]string `yaml:"config"`  // opaque overrides (creds_dir, base_url...)
}
```
并在 `Config` 加字段 `Channels []ChannelConfig \`yaml:"channels"\``。写测试：加载含 `channels:` 的最小 YAML，断言解析出 1 个 `{type:weixin, enabled:true}`。

- [ ] **步骤 2：跑红**：`go test ./internal/config/ -run Channel -v`（FAIL，字段未定义）。

- [ ] **步骤 3：wireChannels 按 config 过滤**

`wireChannels` 遍历 `channel.Descriptors()` 前，若 `len(cfg.Channels) > 0`，构造 `enabled map[string]bool` 与 `cfgByType map[string]map[string]string`；只装配 `enabled[type]`（或 config 未列但属于默认渠道 weixin 时启用，保持向后兼容）。把 `desc.DefaultCredsDir` 与 `cfg.Config["creds_dir"]` 合并后传入 `desc.Build(channel.Config{...})`。`cfg.Channels` 为空时行为与任务 5 完全一致（装配全部已注册渠道）。

- [ ] **步骤 4：跑绿**：`go build ./... && go test ./internal/config/... ./internal/bootstrap/... -v`。

- [ ] **步骤 5：全量回归（阶段一完成门槛）**

运行：

```powershell
$env:PATH = "C:\Users\Administrator\sdk\go\bin;" + $env:PATH
go build ./...
go test ./...
gofmt -l internal/   # 必须无输出
go vet ./internal/...
```

前端无改动；若 `internal/ui/dist` 不受影响则无需重建。全绿后 commit：

```bash
git add internal/ configs/
git commit -m "feat(config): channels 声明式启用配置（可选），阶段一全量回归通过"
```

---

## 完成标准（阶段一验收）

1. `grep -rn '"weixin"' internal/channel/runtime.go internal/channel/outbound.go internal/bootstrap/bootstrap.go` 不再有用于**路由/会话前缀/装配判定**的硬编码（仅 weixin 包自身 `Source()` 与 settings 默认值出现 "weixin"）。
2. `bootstrap.go` 不再出现 `weixin.New` / `weixin.LoadCreds` / `srv.Weixin`；改为遍历 `channel.Descriptors()`。
3. `api/server.go` 不再有 `WeixinILink/WeixinChannel/WeixinRuntime/WeixinCredsDir/WeixinRunCtx` 字段。
4. 新增一个渠道 = 新建 `internal/channel/<name>` 包（实现 `Channel` + `Source()` + `Bootstrap`，`init()` 调 `channel.Register(Descriptor{...})`）+ 在 `cmd/baize/main.go` 加一行 blank import；**不改** bootstrap/outbound/runtime/api 通用路径。
5. weixin 全量行为不变：登录/启停/白名单入站强制/`running`/`reason` 回显/出站【客服】【助手】前缀/HITL 审批续跑/UI 操作员镜像/context_token 出站，均由既有测试（`channel/weixin`、`api/server_channel_weixin_test`、`tests/integration`）锁定且全绿。
6. `go test ./...` 全绿、`gofmt`/`go vet` 干净。

## 自检清单（计划作者）

- 类型/方法名一致：`SourceSourced.Source()`、`ConvID/SourceFromConvID`、`Router.{Add,For,BindRuntime,Extras}`、`Descriptor.{Name,Build,DefaultCredsDir}`、`Register/Describe/Descriptors/ResetForTest`、`BuildDeps`、`Bootstrapper.Bootstrap`、`api.ChannelHandle`、`Server.{RegisterChannel,Channel,Outbound,OutboundExtras}`、`weixin.{Settings,LoadSettings,SaveSettings,DefaultCreds}`、`(*Channel).{Source,ILink,Bootstrap}`。
- 每个任务独立可测、以绿测试收尾；任务 1-5 必须按序（后任务依赖前任务的接口）。
- 历史会话 ID 兼容：`Runtime.Source` 空值默认 `"weixin"`，不改既有 `weixin:...` 会话 ID。
- 无占位符：所有代码块为可直接落地的最小实现；涉及"逐处替换"处给出了明确的 before/after 字段映射。





