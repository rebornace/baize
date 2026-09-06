# 阶段二 2A：通用进程外 webhook 渠道 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 新增一个通用 `webhook` 渠道类型，让 IM 适配器（独立进程、任意语言）经 JSON-over-HTTP 双向收发消息，能力与进程内微信对等；新增 IM 无需改 baize 核心。

**架构：** 新增 `internal/channel/webhook/` 包，实现 `channel.Channel` + `SourceSourced` + `Bootstrapper`，`init()` 注册 Descriptor（`EnabledByDefault:false`）。入站：适配器签名 POST `/v0/channels/{name}/inbound` → 验签/幂等/白名单 → `Runtime.HandleInbound`。出站：`SendText/SendMedia` 签名 POST 适配器 `outbound_url`。渠道经可选 `RouteRegistrar` 钩子在装配期自挂 HTTP 路由（方案 A）。`wireChannels` 声明式模式改为"按配置实例遍历"以支持同类型多实例。

**技术栈：** Go（标准库 net/http、crypto/hmac）、httptest、阶段一渠道框架（Descriptor/Router/Runtime/Bootstrapper）、`internal/inbox` 现有签名算法（抽取共享）。

**铁律（全程）：** 2A 阶段进程内 weixin 渠道**保持原样、行为不变**（新旧并存）。省略 `channels:` 段时装配结果与阶段一逐字节一致。每任务以 `go test ./...`（含 `tests/integration`）全绿、`gofmt`/`go vet` 干净收尾。Go 不在 PATH 时用 `C:\Users\Administrator\go-sdk\go\bin\go.exe`；Windows + PowerShell。

**参考规格：** `docs/superpowers/specs/2026-09-06-phase2-webhook-channel-design.md`（§5 协议、§3 配置、§6 媒体、§9 测试）。本计划只实现 **2A**；2B（微信适配器迁移）另出计划。

---

## 文件结构

**新建：**
- `internal/webhooksig/sig.go` — 共享 HMAC-SHA256 签名/验签/时间窗（从 inbox 抽出，inbox 改为引用它）。
- `internal/channel/webhook/protocol.go` — 入站/出站 DTO、协议头常量、JSON 编解码。
- `internal/channel/webhook/config.go` — 实例配置解析（source/account/secret/outbound_url/assignee/agent_id/supports_vision/allowlist 等）。
- `internal/channel/webhook/channel.go` — `Channel`（实现 Channel/SourceSourced/Bootstrapper），`openFromConfig` 工厂，`init()` 注册 Descriptor。
- `internal/channel/webhook/inbound.go` — 入站 http.Handler：验签、时间窗、幂等、URL 附件下载、白名单、组装 `channel.Inbound` 交 Runtime。
- `internal/channel/webhook/outbound.go` — 出站 HTTP 客户端：签名 POST、超时、指数退避重试、SendText/SendMedia。
- `internal/channel/webhook/*_test.go` — 各文件单测。
- `examples/im-adapter/{main.go,handler.go,hmac.go,README.md}` — 仅标准库的示例适配器。
- `tests/integration/webhook_channel_test.go` — 端到端双向闭环。

**修改：**
- `internal/inbox/verify.go` — 改为 re-export / 调用 `webhooksig`（行为不变）。
- `internal/channel/bootstrap.go` — `BuildDeps` 增加 `Routes RouteRegistrar`；`channel.go` 增加 `RouteRegistrar` 接口。
- `internal/api/server.go` — 实现 `RegisterRoute(pattern, http.Handler)`；`authorize()` 放行渠道入站路径（`/v0/channels/` 前缀下的 POST inbound）。
- `internal/controlplane/acl.go` — 为 `POST /v0/channels/{id}/inbound` 增加 `RoleNone` 规则。
- `internal/config/config.go` — `ChannelConfig` 增加 `Name string \`yaml:"name"\``；相应测试。
- `internal/bootstrap/bootstrap.go` — `wireChannels` 声明式模式按"配置实例"遍历（`channel.Open(type,cfg)` 建实例），省略段路径保持不变；把 `Routes` 传入 `BuildDeps`；重复 name/source 报错。
- `configs/minimal.yaml` — 加注释示例（可选）。

---

## 任务 1：抽取共享签名包 `internal/webhooksig`

**文件：**
- 创建：`internal/webhooksig/sig.go`
- 创建：`internal/webhooksig/sig_test.go`
- 修改：`internal/inbox/verify.go`（改为引用 webhooksig，保持导出 API 不变）

- [ ] **步骤 1：编写失败测试**

`internal/webhooksig/sig_test.go`：

```go
package webhooksig

import (
	"strings"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	secret := "topsecret"
	body := []byte(`{"hello":"world"}`)
	ts := "1700000000"
	sig := Sign(secret, ts, body)
	if !strings.HasPrefix(sig, SignaturePrefix) {
		t.Fatalf("sig missing prefix: %q", sig)
	}
	if err := Verify(secret, ts, body, sig, time.Unix(1700000000, 0), 300*time.Second); err != nil {
		t.Fatalf("verify round trip: %v", err)
	}
}

func TestVerifyRejectsWrongSecret(t *testing.T) {
	body := []byte("x")
	ts := "1700000000"
	sig := Sign("right", ts, body)
	if err := Verify("wrong", ts, body, sig, time.Unix(1700000000, 0), 300*time.Second); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestVerifyRejectsSkew(t *testing.T) {
	body := []byte("x")
	ts := "1700000000"
	sig := Sign("s", ts, body)
	// request time 1000s away from now, maxSkew 300s
	if err := Verify("s", ts, body, sig, time.Unix(1700001000, 0), 300*time.Second); err == nil {
		t.Fatal("expected skew error")
	}
}

func TestVerifyRejectsBadHeader(t *testing.T) {
	body := []byte("x")
	if err := Verify("s", "1700000000", body, "garbage", time.Unix(1700000000, 0), 300*time.Second); err == nil {
		t.Fatal("expected error for malformed header")
	}
	if err := Verify("s", "not-a-number", body, SignaturePrefix+"aa", time.Unix(1700000000, 0), 300*time.Second); err == nil {
		t.Fatal("expected error for bad timestamp")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/webhooksig/...`
预期：FAIL（包不存在 / undefined: Sign）。

- [ ] **步骤 3：编写最少实现**

`internal/webhooksig/sig.go`（逻辑逐字搬迁自 `internal/inbox/verify.go`）：

```go
// Package webhooksig provides HMAC-SHA256 request signing shared by inbound
// webhooks (inbox) and out-of-process channel adapters.
package webhooksig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

const SignaturePrefix = "v1="

var (
	ErrInvalidSignature = errors.New("webhooksig: invalid signature")
	ErrTimestampSkew    = errors.New("webhooksig: timestamp skew")
)

// Sign computes HMAC-SHA256 over "<timestamp>."+body, returned as "v1=<hex>".
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify checks timestamp skew and constant-time signature comparison.
func Verify(secret, timestamp string, body []byte, headerSig string, now time.Time, maxSkew time.Duration) error {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidSignature
	}
	requestTime := time.Unix(ts, 0)
	if now.Sub(requestTime) > maxSkew || requestTime.Sub(now) > maxSkew {
		return ErrTimestampSkew
	}
	if !strings.HasPrefix(headerSig, SignaturePrefix) {
		return ErrInvalidSignature
	}
	expectedSig, err := hex.DecodeString(strings.TrimPrefix(headerSig, SignaturePrefix))
	if err != nil {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	if !hmac.Equal(mac.Sum(nil), expectedSig) {
		return ErrInvalidSignature
	}
	return nil
}
```

修改 `internal/inbox/verify.go` 为薄封装（保持 `inbox.Sign/Verify` 及 `ErrInvalidSignature/ErrTimestampSkew` 导出符号可用，行为不变）：

```go
package inbox

import (
	"errors"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

const signaturePrefix = webhooksig.SignaturePrefix

var (
	ErrInvalidSignature = webhooksig.ErrInvalidSignature
	ErrTimestampSkew    = webhooksig.ErrTimestampSkew
)

// Sign delegates to the shared webhooksig package.
func Sign(secret, timestamp string, body []byte) string {
	return webhooksig.Sign(secret, timestamp, body)
}

// Verify delegates to the shared webhooksig package.
func Verify(secret, timestamp string, body []byte, headerSig string, now time.Time, maxSkew time.Duration) error {
	if err := webhooksig.Verify(secret, timestamp, body, headerSig, now, maxSkew); err != nil {
		if errors.Is(err, webhooksig.ErrTimestampSkew) {
			return ErrTimestampSkew
		}
		return ErrInvalidSignature
	}
	return nil
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/webhooksig/... ./internal/inbox/...`
预期：PASS（webhooksig 新测试 + inbox 既有测试全绿，行为不变）。

- [ ] **步骤 5：Commit**

```bash
git add internal/webhooksig/ internal/inbox/verify.go
git commit -m "refactor(webhooksig): 抽取共享 HMAC 签名包，inbox 改为引用"
```

---

## 任务 2：渠道路由自注册接缝 + `ChannelConfig.Name` + 入站 ACL 放行

**文件：**
- 创建：`internal/channel/routes.go`（`RouteRegistrar` 接口）
- 修改：`internal/channel/bootstrap.go`（`BuildDeps` 加 `Routes`）
- 修改：`internal/api/server.go`（`Server.RegisterRoute`）
- 修改：`internal/controlplane/acl.go`（入站路径 `RoleNone`）
- 修改：`internal/config/config.go`（`ChannelConfig.Name`）
- 测试：`internal/channel/routes_test.go`、`internal/api/channels_route_test.go`、`internal/controlplane/acl_test.go`（或既有 acl 测试追加）、`internal/config/channels_test.go`

- [ ] **步骤 1：编写失败测试**

`internal/channel/routes_test.go`：

```go
package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRegistrar struct {
	patterns []string
}

func (f *fakeRegistrar) RegisterRoute(pattern string, h http.Handler) {
	f.patterns = append(f.patterns, pattern)
}

func TestRouteRegistrarInterface(t *testing.T) {
	var _ RouteRegistrar = (*fakeRegistrar)(nil)
	fr := &fakeRegistrar{}
	fr.RegisterRoute("POST /v0/channels/x/inbound", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	if len(fr.patterns) != 1 || fr.patterns[0] != "POST /v0/channels/x/inbound" {
		t.Fatalf("unexpected patterns: %v", fr.patterns)
	}
}

func TestBuildDepsCarriesRoutes(t *testing.T) {
	fr := &fakeRegistrar{}
	deps := BuildDeps{Routes: fr}
	if deps.Routes == nil {
		t.Fatal("Routes not carried")
	}
}
```

`internal/api/channels_route_test.go`：

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterRouteMountsHandler(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	srv.RegisterRoute("POST /v0/channels/demo/inbound", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("hit"))
	}))
	req := httptest.NewRequest("POST", "/v0/channels/demo/inbound", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req) // Handler() returns the gated mux; gate off with no tokens
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
}
```

> 注：`api.Server.Handler()` 返回包了门禁的 `http.Handler`（见 `internal/api/server.go:188`）。此处 `NewServer(nil,nil,nil)` 未配置任何 token，门禁关闭（`gateTokens().Enabled()==false`），请求直达 mux，故返回 202 而非 401。

ACL 测试（追加到既有 `internal/controlplane/acl_test.go`，若文件不存在则新建，沿用该包既有测试风格）：

```go
func TestChannelInboundIsRoleNone(t *testing.T) {
	if got := MinRole("POST", "/v0/channels/feishu/inbound"); got != RoleNone {
		t.Fatalf("inbound should be RoleNone (channel HMAC), got %v", got)
	}
}
```

config 测试（追加到 `internal/config/channels_test.go`，沿用该文件既有的临时文件 + `Load` 模式）：

```go
// TestChannelConfigNameParsed verifies the per-instance Name field parses and
// survives a Load round-trip.
func TestChannelConfigNameParsed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(p, []byte(`
llm:
  provider: mock
channels:
  - name: feishu
    type: webhook
    enabled: true
    config:
      source: feishu
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Channels) != 1 {
		t.Fatalf("channels len=%d want 1", len(cfg.Channels))
	}
	ch := cfg.Channels[0]
	if ch.Name != "feishu" || ch.Type != "webhook" || !ch.Enabled {
		t.Fatalf("unexpected: %+v", ch)
	}
	if ch.Config["source"] != "feishu" {
		t.Fatalf("source=%q", ch.Config["source"])
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/... ./internal/api/... ./internal/controlplane/... ./internal/config/...`
预期：FAIL（`undefined: RouteRegistrar`、`BuildDeps.Routes` 未定义、`srv.RegisterRoute` 未定义、`ChannelConfig.Name` 未定义、ACL 断言失败）。

- [ ] **步骤 3：编写最少实现**

`internal/channel/routes.go`：

```go
package channel

import "net/http"

// RouteRegistrar is implemented by the HTTP server (api.Server). Channels
// that expose their own HTTP endpoints (e.g. the webhook channel's inbound
// webhook) register them during Bootstrap instead of editing core routing.
// It depends only on net/http so the channel package never imports api.
type RouteRegistrar interface {
	RegisterRoute(pattern string, h http.Handler)
}
```

`internal/channel/bootstrap.go`：在 `BuildDeps` 结构增加字段（放在 `ResumeHITL` 之后）：

```go
	// Routes, when non-nil, lets a channel mount its own HTTP endpoints during
	// Bootstrap (e.g. inbound webhook). Nil in tests that do not exercise HTTP.
	Routes RouteRegistrar
```

`internal/api/server.go`：新增方法（放在 `RegisterChannel` 附近）：

```go
// RegisterRoute mounts an additional HTTP route on the core mux. Used by
// channels (via channel.RouteRegistrar) to expose their own endpoints without
// editing core routing. Must be called before the server starts serving.
func (s *Server) RegisterRoute(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}
```

`internal/controlplane/acl.go`：在规则表中（`v0/inbox/{id}` RoleNone 那条附近）增加：

```go
	{method: "POST", segments: []string{"v0", "channels", "{id}", "inbound"}, role: RoleNone},
```

`internal/config/config.go`：`ChannelConfig` 增加 `Name` 字段（放在 `Type` 之前）：

```go
type ChannelConfig struct {
	Name    string            `yaml:"name"`    // instance name; defaults to type. Required unique for multiple instances of one type.
	Type    string            `yaml:"type"`    // channel type key, e.g. "webhook"/"weixin"
	Enabled bool              `yaml:"enabled"` // wired only when true in declarative mode
	Config  map[string]string `yaml:"config"`  // opaque overrides (source, secret, outbound_url...)
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/... ./internal/api/... ./internal/controlplane/... ./internal/config/...`
预期：PASS。确认 inbox 路径仍 RoleNone（既有测试不回归）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/routes.go internal/channel/routes_test.go internal/channel/bootstrap.go internal/api/ internal/controlplane/ internal/config/
git commit -m "feat(channel): 渠道 HTTP 路由自注册接缝 RouteRegistrar + ChannelConfig.Name + 入站 ACL 放行"
```

---

## 任务 3：webhook 协议 DTO 与实例配置解析

**文件：**
- 创建：`internal/channel/webhook/protocol.go`
- 创建：`internal/channel/webhook/config.go`
- 创建：`internal/channel/webhook/config_test.go`

- [ ] **步骤 1：编写失败测试**

`internal/channel/webhook/config_test.go`：

```go
package webhook

import "testing"

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig("feishu", map[string]string{
		"source":          "feishu",
		"account":         "feishu-bot-1",
		"secret":          "s3cr3t",
		"outbound_url":    "http://adapter:8080/outbound",
		"assignee":        "u-admin",
		"agent_id":        "agent-x",
		"supports_vision": "true",
		"allowlist":       "u1, u2 ,,u3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "feishu" || cfg.Account != "feishu-bot-1" || cfg.Secret != "s3cr3t" {
		t.Fatalf("bad base fields: %+v", cfg)
	}
	if cfg.OutboundURL != "http://adapter:8080/outbound" || cfg.Assignee != "u-admin" || cfg.AgentID != "agent-x" {
		t.Fatalf("bad wiring fields: %+v", cfg)
	}
	if !cfg.SupportsVision {
		t.Fatal("supports_vision should parse true")
	}
	if len(cfg.Allowlist) != 3 || cfg.Allowlist["u1"] == false || cfg.Allowlist["u3"] == false {
		t.Fatalf("bad allowlist: %+v", cfg.Allowlist)
	}
}

func TestParseConfigDefaultsAndErrors(t *testing.T) {
	// source/account default to instance name (passed via name arg)
	cfg, err := parseConfig("feishu", map[string]string{"secret": "s", "outbound_url": "http://x/o", "assignee": "a"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source != "feishu" || cfg.Account != "feishu" {
		t.Fatalf("expected source/account default to instance name, got %+v", cfg)
	}
	// missing secret => error
	if _, err := parseConfig("n", map[string]string{"outbound_url": "http://x/o", "assignee": "a"}); err == nil {
		t.Fatal("expected error for missing secret")
	}
	// missing outbound_url => error
	if _, err := parseConfig("n", map[string]string{"secret": "s", "assignee": "a"}); err == nil {
		t.Fatal("expected error for missing outbound_url")
	}
	// missing assignee => error
	if _, err := parseConfig("n", map[string]string{"secret": "s", "outbound_url": "http://x/o"}); err == nil {
		t.Fatal("expected error for missing assignee")
	}
}
```

> 注：测试中 `ConfigMap` 用 `map[string]string` 别名或直接 `channel.Config`（底层即 `map[string]string`）；`parseConfig(name, cfg)` 是包内函数。以实际可编译为准。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/...`
预期：FAIL（包/函数未定义）。

- [ ] **步骤 3：编写最少实现**

`internal/channel/webhook/protocol.go`：

```go
package webhook

// Header names for the channel adapter wire protocol.
const (
	HeaderProtocol  = "X-Baize-Protocol"
	HeaderTimestamp = "X-Baize-Channel-Timestamp"
	HeaderSignature = "X-Baize-Channel-Signature"
	HeaderRunID     = "X-Baize-Run-Id"
	ProtocolVersion = "v0"
)

// Peer identifies a conversation party on the IM side.
type Peer struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Attachment is an inbound file, inline (base64) or a URL baize will fetch.
type Attachment struct {
	Name          string `json:"name"`
	MIME          string `json:"mime"`
	ContentBase64 string `json:"content_base64,omitempty"`
	URL           string `json:"url,omitempty"`
}

// InboundMessage is the adapter -> baize request body.
type InboundMessage struct {
	Event          string       `json:"event"` // "message"
	Account        string       `json:"account"`
	Peer           Peer         `json:"peer"`
	Text           string       `json:"text"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	ContextToken   string       `json:"context_token,omitempty"`
	IdempotencyKey string       `json:"idempotency_key,omitempty"`
}

// OutboundMedia is a file baize pushes to the adapter.
type OutboundMedia struct {
	Name          string `json:"name"`
	MIME          string `json:"mime"`
	ContentBase64 string `json:"content_base64"`
}

// OutboundMessage is the baize -> adapter request body.
type OutboundMessage struct {
	Kind           string         `json:"kind"` // assistant|operator|notify
	ConversationID string         `json:"conversation_id"`
	Account        string         `json:"account"`
	Peer           Peer           `json:"peer"`
	Text           string         `json:"text,omitempty"`
	Media          []OutboundMedia `json:"media,omitempty"`
	RunID          string         `json:"run_id,omitempty"`
	ContextToken   string         `json:"context_token,omitempty"`
}
```

`internal/channel/webhook/config.go`：

```go
package webhook

import (
	"fmt"
	"strconv"
	"strings"
)

// instanceConfig is a resolved webhook channel instance configuration.
type instanceConfig struct {
	Name           string
	Source         string
	Account        string
	Secret         string
	OutboundSecret string
	OutboundURL    string
	Assignee       string
	AgentID        string
	SupportsVision bool
	Allowlist      map[string]bool
}

// parseConfig resolves an instance from its channel.Config map. name is the
// instance name (used as default source/account).
func parseConfig(name string, m map[string]string) (instanceConfig, error) {
	get := func(k string) string { return strings.TrimSpace(m[k]) }
	c := instanceConfig{Name: name, Allowlist: map[string]bool{}}
	c.Source = orDefault(get("source"), name)
	c.Account = orDefault(get("account"), name)
	c.Secret = get("secret")
	c.OutboundSecret = orDefault(get("outbound_secret"), c.Secret)
	c.OutboundURL = get("outbound_url")
	c.Assignee = get("assignee")
	c.AgentID = get("agent_id")
	if v := get("supports_vision"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("webhook: invalid supports_vision %q: %w", v, err)
		}
		c.SupportsVision = b
	}
	if raw := get("allowlist"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				c.Allowlist[p] = true
			}
		}
	}
	if c.Secret == "" {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q", "secret", name)
	}
	if c.OutboundURL == "" {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q", "outbound_url", name)
	}
	if c.Assignee == "" {
		return c, fmt.Errorf("webhook: missing required config %q for instance %q", "assignee", name)
	}
	return c, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/webhook/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/webhook/protocol.go internal/channel/webhook/config.go internal/channel/webhook/config_test.go
git commit -m "feat(webhook): 渠道适配器协议 DTO 与实例配置解析"
```

---

## 任务 4：出站 HTTP 客户端（签名 POST + 指数退避重试）

**文件：**
- 创建：`internal/channel/webhook/outbound.go`
- 创建：`internal/channel/webhook/outbound_test.go`

- [ ] **步骤 1：编写失败测试**

`internal/channel/webhook/outbound_test.go`：

```go
package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

func testCfg(url string) instanceConfig {
	return instanceConfig{Name: "t", Source: "feishu", Account: "acc", Secret: "sec", OutboundSecret: "sec", OutboundURL: url, Assignee: "a"}
}

func TestOutboundPostSignsAndDelivers(t *testing.T) {
	var got OutboundMessage
	var gotSig, gotTS, gotProto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		gotSig = r.Header.Get(HeaderSignature)
		gotTS = r.Header.Get(HeaderTimestamp)
		gotProto = r.Header.Get(HeaderProtocol)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newSender(testCfg(srv.URL))
	err := s.post(context.Background(), OutboundMessage{Kind: "assistant", Account: "acc", Peer: Peer{ID: "u1"}, Text: "hi", RunID: "r1"})
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if got.Text != "hi" || got.Kind != "assistant" || got.Peer.ID != "u1" || got.Account != "acc" {
		t.Fatalf("bad body: %+v", got)
	}
	if gotProto != ProtocolVersion {
		t.Fatalf("bad protocol header: %q", gotProto)
	}
	if err := webhooksig.Verify("sec", gotTS, mustMarshal(t, got), gotSig, time.Now(), 300*time.Second); err != nil {
		t.Fatalf("signature verify: %v", err)
	}
}

func TestOutboundRetriesOn5xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	s := newSender(testCfg(srv.URL))
	if err := s.post(context.Background(), OutboundMessage{Kind: "assistant", Account: "acc", Peer: Peer{ID: "u"}}); err != nil {
		t.Fatalf("expected success after retries: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestOutboundNoRetryOn4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	s := newSender(testCfg(srv.URL))
	if err := s.post(context.Background(), OutboundMessage{Kind: "assistant", Account: "acc", Peer: Peer{ID: "u"}}); err == nil {
		t.Fatal("expected error on 4xx")
	}
	if calls != 1 {
		t.Fatalf("4xx must not retry, got %d calls", calls)
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
```

> 说明：签名验证测试用 `webhooksig.Verify` 重算；body 是服务端反序列化再重新 marshal 的 `OutboundMessage`，字段一致即可通过（JSON 键序由结构体定义固定）。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/...`
预期：FAIL（`undefined: newSender`）。

- [ ] **步骤 3：编写最少实现**

`internal/channel/webhook/outbound.go`：

```go
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/rebornace/baize/internal/webhooksig"
)

const (
	outboundTimeout   = 10 * time.Second
	outboundMaxTries  = 3
	maxOutboundBody   = 1 << 16 // 64KiB drain cap for error responses
)

// sender posts signed outbound messages to the adapter.
type sender struct {
	cfg instanceConfig
	hc  *http.Client
}

func newSender(cfg instanceConfig) *sender {
	return &sender{cfg: cfg, hc: &http.Client{Timeout: outboundTimeout}}
}

// post sends msg to the adapter outbound URL, signing with the outbound
// secret. Retries connection errors and 5xx with exponential backoff; 4xx
// (protocol/config error) is not retried.
func (s *sender) post(ctx context.Context, msg OutboundMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(s.cfg.OutboundSecret, ts, body)

	var lastErr error
	for attempt := 0; attempt < outboundMaxTries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.OutboundURL, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(HeaderProtocol, ProtocolVersion)
		req.Header.Set(HeaderTimestamp, ts)
		req.Header.Set(HeaderSignature, sig)
		if msg.RunID != "" {
			req.Header.Set(HeaderRunID, msg.RunID)
		}
		resp, err := s.hc.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxOutboundBody))
		_ = resp.Body.Close()
		code := resp.StatusCode
		if code >= 200 && code < 300 {
			return nil
		}
		if code >= 400 && code < 500 {
			return fmt.Errorf("webhook: outbound rejected with status %d (not retried)", code)
		}
		lastErr = fmt.Errorf("webhook: outbound status %d", code)
	}
	return lastErr
}

func backoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 1 * time.Second
	case 2:
		return 2 * time.Second
	default:
		return 4 * time.Second
	}
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/webhook/...`
预期：PASS（签名正确、5xx 重试 3 次成功、4xx 不重试）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/webhook/outbound.go internal/channel/webhook/outbound_test.go
git commit -m "feat(webhook): 出站签名 POST 客户端 + 指数退避重试"
```

---

## 任务 5：webhook 渠道（Channel + 工厂 + 注册 + Bootstrap + 出站）

**文件：**
- 创建：`internal/channel/webhook/channel.go`
- 创建：`internal/channel/webhook/channel_test.go`

- [ ] **步骤 1：编写失败测试**

`internal/channel/webhook/channel_test.go`：

```go
package webhook

import (
	"context"
	"testing"
)

func TestOpenFromConfigRequiresInstanceName(t *testing.T) {
	// empty instance name must error (multi-instance disambiguation)
	if _, err := openFromConfig("", map[string]string{"secret": "s", "outbound_url": "http://x/o", "assignee": "a"}); err == nil {
		t.Fatal("expected error for empty instance name")
	}
}

func TestOpenFromConfigBuildsChannel(t *testing.T) {
	ch, err := openFromConfig("feishu", map[string]string{
		"source":       "feishu",
		"account":      "acc-1",
		"secret":       "s",
		"outbound_url": "http://x/o",
		"assignee":     "u-admin",
		"agent_id":     "ag1",
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if ch.Name() != "feishu" {
		t.Fatalf("Name()=%q want feishu (instance name)", ch.Name())
	}
	if ch.Source() != "feishu" {
		t.Fatalf("Source()=%q want feishu", ch.Source())
	}
}

func TestSendTextPostsSignedOutbound(t *testing.T) {
	var got OutboundMessage
	srv := newCaptureServer(&got)
	defer srv.Close()
	ch, _ := openFromConfig("feishu", map[string]string{
		"source": "feishu", "account": "acc-1", "secret": "s",
		"outbound_url": srv.URL, "assignee": "u-admin",
	})
	if err := ch.SendText(context.Background(), "peer9", "你好", map[string]string{"context_token": "tok123"}); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if got.Peer.ID != "peer9" || got.Text != "你好" {
		t.Fatalf("bad peer/text: %+v", got)
	}
	if got.Account != "acc-1" {
		t.Fatalf("outbound account should be instance account, got %q", got.Account)
	}
	if got.ConversationID == "" {
		t.Fatal("ConversationID should be set")
	}
	if got.ContextToken != "tok123" {
		t.Fatalf("context token not propagated: %+v", got)
	}
}
```

在 `channel_test.go` 末尾补上 `newCaptureServer` 辅助：

```go
import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
)

func newCaptureServer(got *OutboundMessage) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, got)
		w.WriteHeader(http.StatusOK)
	}))
}
```

> （`context`、`testing` 与上面 import 合并到一个 import 块；`newCaptureServer` 读 body 到 `*OutboundMessage` 并回 200。）

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/...`
预期：FAIL（`undefined: openFromConfig`）。

- [ ] **步骤 3：编写最少实现**

`internal/channel/webhook/channel.go`：

```go
// Package webhook implements a generic out-of-process channel: IM adapters
// (independent processes, any language) exchange JSON over HTTP with baize.
// Each configured instance represents one IM account.
package webhook

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/channel"
)

func init() {
	channel.Register(channel.Descriptor{
		Name:             "webhook",
		Build:            func(c channel.Config) (channel.Channel, error) { return openFromConfig(c["name"], c) },
		DefaultCredsDir:  "",
		EnabledByDefault: false, // opt-in via config channels:; multiple instances allowed
	})
}

// Channel is one webhook channel instance (one IM account).
type Channel struct {
	cfg    instanceConfig
	sender *sender
	rt     *channel.Runtime
}

// openFromConfig builds a webhook instance. name is the instance name; it is
// required and used as the default source/account.
func openFromConfig(name string, m map[string]string) (*Channel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("webhook: instance name is required (use channels[].name)")
	}
	cfg, err := parseConfig(name, m)
	if err != nil {
		return nil, err
	}
	return &Channel{cfg: cfg, sender: newSender(cfg)}, nil
}

func (c *Channel) Name() string   { return c.cfg.Name }
func (c *Channel) Source() string { return c.cfg.Source }

// Start/Stop are no-ops: the webhook channel has no polling loop; inbound
// arrives over HTTP and outbound is request/response.
func (c *Channel) Start(ctx context.Context) error { return nil }
func (c *Channel) Stop(ctx context.Context) error  { return nil }

// Bootstrap assembles the channel Runtime from generic deps. Persisted
// per-channel settings are not used for webhook (it is purely config-driven);
// assignee/agent/vision come from instance config. It registers its inbound
// HTTP route via deps.Routes when available.
func (c *Channel) Bootstrap(deps channel.BuildDeps) (*channel.Runtime, string, bool, error) {
	rt := &channel.Runtime{
		Runs:           deps.Store,
		Meta:           deps.Meta,
		Messages:       deps.Messages,
		Assignee:       c.cfg.Assignee,
		DefaultAgentID: firstNonEmpty(c.cfg.AgentID, deps.DefaultAgentID),
		SupportsVision: deps.SupportsVision || c.cfg.SupportsVision,
		AfterCreateRun: deps.AfterCreateRun,
		ResumeHITL:     deps.ResumeHITL,
		Source:         c.cfg.Source,
	}
	c.rt = rt
	if deps.Routes != nil {
		deps.Routes.RegisterRoute(
			"POST /v0/channels/"+c.cfg.Name+"/inbound",
			c.inboundHandler(),
		)
	}
	// start=true keeps parity with other Bootstrappers; Start is a no-op.
	return rt, "", true, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// SendText pushes a text message to the adapter for peerID.
func (c *Channel) SendText(ctx context.Context, peerID, text string, extras map[string]string) error {
	msg := OutboundMessage{
		Kind:           kindFromExtras(extras),
		ConversationID: channel.ConvID(c.cfg.Source, c.cfg.Account, peerID),
		Account:        c.cfg.Account,
		Peer:           Peer{ID: peerID},
		Text:           text,
		ContextToken:   extras["context_token"],
	}
	return c.sender.post(ctx, msg)
}

// SendMedia pushes a file to the adapter (small files inline base64).
func (c *Channel) SendMedia(ctx context.Context, peerID, filename, mime string, data []byte, extras map[string]string) error {
	msg := OutboundMessage{
		Kind:           kindFromExtras(extras),
		ConversationID: channel.ConvID(c.cfg.Source, c.cfg.Account, peerID),
		Account:        c.cfg.Account,
		Peer:           Peer{ID: peerID},
		Media: []OutboundMedia{{
			Name:          filename,
			MIME:          mime,
			ContentBase64: base64.StdEncoding.EncodeToString(data),
		}},
		ContextToken: extras["context_token"],
	}
	return c.sender.post(ctx, msg)
}

func kindFromExtras(extras map[string]string) string {
	if extras != nil {
		if k := strings.TrimSpace(extras["kind"]); k != "" {
			return k
		}
	}
	return "assistant"
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/webhook/...`
预期：PASS（工厂、Name/Source、出站签名 POST、account/context_token 传递正确）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/webhook/channel.go internal/channel/webhook/channel_test.go
git commit -m "feat(webhook): webhook 渠道 Channel/工厂/注册/Bootstrap/出站"
```

---

## 任务 6：入站 HTTP handler（验签 / 幂等 / 白名单 / 附件 / 落 Runtime）

**文件：**
- 创建：`internal/channel/webhook/inbound.go`
- 创建：`internal/channel/webhook/inbound_test.go`

- [ ] **步骤 1：编写失败测试**

`internal/channel/webhook/inbound_test.go`：

```go
package webhook

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhooksig"
)

type fakeRuns struct {
	mu      sync.Mutex
	created []string
	active  bool
}

func (f *fakeRuns) CreateRun(in store.CreateRunInput) (*store.Run, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, in.ConversationID)
	return &store.Run{ID: "run-1", AgentID: in.AgentID, ConversationID: in.ConversationID}, nil
}
func (f *fakeRuns) HasActiveRun(string) (bool, error) { return f.active, nil }
func (f *fakeRuns) WaitingHumanRun(string) (*store.Run, error) { return nil, nil }

type routeRec struct{ pattern string }

func (r *routeRec) RegisterRoute(p string, h http.Handler) { r.pattern = p }

func bootChannel(t *testing.T, m map[string]string, runs channel.RunStore, hook func(*store.Run, []llm.ContentPart)) (*Channel, http.Handler) {
	t.Helper()
	ch, err := openFromConfig("feishu", m)
	if err != nil {
		t.Fatal(err)
	}
	rec := &routeRec{}
	rt, _, _, err := ch.Bootstrap(channel.BuildDeps{
		Store:          runs,
		Meta:           conversation.NewMemoryStore(),
		DefaultAgentID: "ag-default",
		Routes:         rec,
		AfterCreateRun: func(ctx context.Context, run *store.Run, parts []llm.ContentPart) error {
			if hook != nil {
				hook(run, parts)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ch.rt = rt
	if rec.pattern != "POST /v0/channels/feishu/inbound" {
		t.Fatalf("unexpected route pattern: %q", rec.pattern)
	}
	return ch, ch.inboundHandler()
}

func signedBody(t *testing.T, secret string, msg InboundMessage) ([]byte, string, string) {
	t.Helper()
	body, _ := json.Marshal(msg)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	return body, ts, webhooksig.Sign(secret, ts, body)
}

func post(handler http.Handler, body []byte, ts, sig string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/v0/channels/feishu/inbound", bytes.NewReader(body))
	req.Header.Set(HeaderTimestamp, ts)
	req.Header.Set(HeaderSignature, sig)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func baseCfg(url string) map[string]string {
	return map[string]string{
		"source": "feishu", "account": "acc-1", "secret": "s3cr3t",
		"outbound_url": url, "assignee": "u-admin",
	}
}

func TestInboundCreatesRun(t *testing.T) {
	runs := &fakeRuns{}
	var gotParts []llm.ContentPart
	ch, handler := bootChannel(t, baseCfg("http://x/o"), runs, func(r *store.Run, p []llm.ContentPart) { gotParts = p })

	msg := InboundMessage{Event: "message", Peer: Peer{ID: "peer9"}, Text: "你好"}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	rec := post(handler, body, ts, sig)
	if rec.Code != http.StatusOK && rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(runs.created) != 1 {
		t.Fatalf("expected 1 run, got %v", runs.created)
	}
	if want := "feishu:acc-1:peer9"; runs.created[0] != want {
		t.Fatalf("convID=%q want %q", runs.created[0], want)
	}
	if len(gotParts) != 0 { // text-only => no multimodal parts
		t.Fatalf("expected no parts for text, got %+v", gotParts)
	}
}

func TestInboundRejectsBadSignature(t *testing.T) {
	runs := &fakeRuns{}
	_, handler := bootChannel(t, baseCfg("http://x/o"), runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x"}
	body, ts, _ := signedBody(t, "wrong-secret", msg)
	rec := post(handler, body, ts, webhooksig.Sign("wrong-secret", ts, body))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if len(runs.created) != 0 {
		t.Fatal("must not create run on bad signature")
	}
}

func TestInboundIdempotencyDedupes(t *testing.T) {
	runs := &fakeRuns{}
	ch, handler := bootChannel(t, baseCfg("http://x/o"), runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "x", IdempotencyKey: "k-1"}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	if r := post(handler, body, ts, sig); r.Code >= 300 {
		t.Fatalf("first: %d", r.Code)
	}
	if r := post(handler, body, ts, sig); r.Code >= 300 {
		t.Fatalf("second: %d", r.Code)
	}
	if len(runs.created) != 1 {
		t.Fatalf("idempotent key should yield 1 run, got %d", len(runs.created))
	}
}

func TestInboundAllowlistBlocksOutsider(t *testing.T) {
	runs := &fakeRuns{}
	cfg := baseCfg("http://x/o")
	cfg["allowlist"] = "allowed-peer"
	ch, handler := bootChannel(t, cfg, runs, nil)
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "stranger"}, Text: "x"}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	rec := post(handler, body, ts, sig)
	if rec.Code >= 300 {
		t.Fatalf("ignored messages still ack 2xx, got %d", rec.Code)
	}
	if len(runs.created) != 0 {
		t.Fatal("peer not in allowlist must not create run")
	}
}

func TestInboundInlineImageBecomesPart(t *testing.T) {
	runs := &fakeRuns{}
	var gotParts []llm.ContentPart
	cfg := baseCfg("http://x/o")
	cfg["supports_vision"] = "true"
	ch, handler := bootChannel(t, cfg, runs, func(r *store.Run, p []llm.ContentPart) { gotParts = p })

	// 1x1 transparent PNG
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	msg := InboundMessage{Event: "message", Peer: Peer{ID: "p"}, Text: "看图",
		Attachments: []Attachment{{Name: "a.png", MIME: "image/png", ContentBase64: base64.StdEncoding.EncodeToString(png)}}}
	body, ts, sig := signedBody(t, ch.cfg.Secret, msg)
	if rec := post(handler, body, ts, sig); rec.Code >= 300 {
		t.Fatalf("status=%d", rec.Code)
	}
	sawImage := false
	for _, p := range gotParts {
		if p.Type == "image" {
			sawImage = true
		}
	}
	if !sawImage {
		t.Fatalf("expected an image part, got %+v", gotParts)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/channel/webhook/...`
预期：FAIL（`undefined: (c *Channel).inboundHandler`）。

- [ ] **步骤 3：编写最少实现**

`internal/channel/webhook/inbound.go`：

```go
package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/webhooksig"
)

const (
	maxInboundBody = 10 << 20 // 10MiB (base64 media included)
	maxFetchBody   = 10 << 20
	signatureSkew  = 300 * time.Second
)

// inboundHandler returns the http.Handler for POST /v0/channels/{name}/inbound.
func (c *Channel) inboundHandler() http.Handler {
	var (
		mu      sync.Mutex
		seenKey = map[string]bool{}
	)
	fetch := &http.Client{Timeout: 15 * time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, maxInboundBody))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read body"})
			return
		}
		if err := webhooksig.Verify(c.cfg.Secret, r.Header.Get(HeaderTimestamp), body,
			r.Header.Get(HeaderSignature), time.Now(), signatureSkew); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
			return
		}
		var msg InboundMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
			return
		}
		if msg.Event != "" && msg.Event != "message" {
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "ignored"})
			return
		}
		peer := msg.Peer.ID
		if peer == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peer.id required"})
			return
		}
		// Mandatory inbound allowlist (when configured).
		if len(c.cfg.Allowlist) > 0 && !c.cfg.Allowlist[peer] {
			log.Printf("webhook %s: peer %q not in allowlist; ignored", c.cfg.Name, peer)
			writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
			return
		}
		// Best-effort idempotency (at-least-once delivery).
		if key := msg.IdempotencyKey; key != "" {
			mu.Lock()
			dup := seenKey[key]
			if !dup {
				seenKey[key] = true
			}
			mu.Unlock()
			if dup {
				writeJSON(w, http.StatusOK, map[string]string{"status": "duplicate"})
				return
			}
		}
		files, err := c.resolveAttachments(r.Context(), fetch, msg.Attachments)
		if err != nil {
			log.Printf("webhook %s: attachment error: %v", c.cfg.Name, err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "attachment"})
			return
		}
		extras := map[string]string{"account": c.cfg.Account}
		if msg.ContextToken != "" {
			extras["context_token"] = msg.ContextToken
		}
		in := channel.Inbound{PeerID: peer, Text: msg.Text, Files: files, Extras: extras}
		if err := c.rt.HandleInbound(r.Context(), c, in); err != nil {
			log.Printf("webhook %s: handle inbound: %v", c.cfg.Name, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "handle"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}

func (c *Channel) resolveAttachments(ctx context.Context, hc *http.Client, atts []Attachment) ([]channel.InboundFile, error) {
	files := make([]channel.InboundFile, 0, len(atts))
	for _, a := range atts {
		var data []byte
		switch {
		case a.ContentBase64 != "":
			b, err := base64.StdEncoding.DecodeString(a.ContentBase64)
			if err != nil {
				return nil, err
			}
			data = b
		case a.URL != "":
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := hc.Do(req)
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
			_ = resp.Body.Close()
			if err != nil {
				return nil, err
			}
			data = b
		default:
			continue
		}
		files = append(files, channel.InboundFile{Name: a.Name, MIME: a.MIME, Data: data})
	}
	return files, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/channel/webhook/...`
预期：PASS（建 run、convID=`feishu:acc-1:peer9`、坏签名 401、幂等去重、白名单忽略、内联图片→image part）。

- [ ] **步骤 5：Commit**

```bash
git add internal/channel/webhook/inbound.go internal/channel/webhook/inbound_test.go
git commit -m "feat(webhook): 入站 handler 验签/幂等/白名单/附件并落 Runtime"
```

---

## 任务 7：`wireChannels` 声明式多实例装配 + Routes 接线

**文件：**
- 修改：`internal/bootstrap/bootstrap.go`（`wireChannels` 声明式分支按配置实例装配；`BuildDeps.Routes` 注入；空导入 webhook 包）
- 测试：`internal/bootstrap/wire_channels_test.go`（追加多实例用例）

- [ ] **步骤 1：编写失败测试**

追加到 `internal/bootstrap/wire_channels_test.go`：

```go

// TestWireChannelsWebhookMultiInstance proves a webhook type configured with
// two named instances yields two separately-registered handles, two routable
// sources, and two distinct inbound routes.
func TestWireChannelsWebhookMultiInstance(t *testing.T) {
	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{
		{Name: "feishu", Type: "webhook", Enabled: true, Config: map[string]string{
			"source": "feishu", "account": "feishu-bot", "secret": "s",
			"outbound_url": "http://x/o", "assignee": "u",
		}},
		{Name: "dingtalk", Type: "webhook", Enabled: true, Config: map[string]string{
			"source": "dingtalk", "account": "dt-bot", "secret": "s2",
			"outbound_url": "http://y/o", "assignee": "u",
		}},
	}
	router, err := wireChannels(d)
	if err != nil {
		t.Fatalf("wireChannels: %v", err)
	}
	if _, ok := router.For("feishu"); !ok {
		t.Fatal("router should route feishu source")
	}
	if _, ok := router.For("dingtalk"); !ok {
		t.Fatal("router should route dingtalk source")
	}
	if _, ok := d.srv.Channel("feishu"); !ok {
		t.Fatal("handle feishu missing")
	}
	if _, ok := d.srv.Channel("dingtalk"); !ok {
		t.Fatal("handle dingtalk missing")
	}
	// Inbound routes must be mounted and open (RoleNone) through the gated handler.
	for _, p := range []string{"/v0/channels/feishu/inbound", "/v0/channels/dingtalk/inbound"} {
		req := httptest.NewRequest("POST", p, nil)
		rec := httptest.NewRecorder()
		d.srv.Handler().ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("route %s not mounted", p)
		}
		// No token configured => must NOT be 401/403 (channel HMAC path is public).
		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
			t.Fatalf("inbound route %s should be public, got %d", p, rec.Code)
		}
	}
}

// TestWireChannelsWebhookDuplicateInstanceNameErrors proves two instances with
// the same name fail fast.
func TestWireChannelsWebhookDuplicateInstanceNameErrors(t *testing.T) {
	d := channelDepsForTest(t)
	d.cfg.Channels = []config.ChannelConfig{
		{Name: "dup", Type: "webhook", Enabled: true, Config: map[string]string{"secret": "s", "outbound_url": "http://x/o", "assignee": "u"}},
		{Name: "dup", Type: "webhook", Enabled: true, Config: map[string]string{"secret": "s", "outbound_url": "http://y/o", "assignee": "u"}},
	}
	if _, err := wireChannels(d); err == nil {
		t.Fatal("expected error for duplicate instance name")
	}
}
```

> 需在该测试文件 import 块加入：`net/http`、`net/http/httptest`。**无需**导入 webhook 包——任务 7 步骤 3a 在 `bootstrap.go` 里加了 webhook 空导入，测试编译 `bootstrap` 包时其 `init()` 自动注册 `webhook` 描述符。

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/bootstrap/... -run Webhook`
预期：FAIL（多实例只建出一个/被覆盖、路由未挂载、`BuildDeps.Routes` 未接）。

- [ ] **步骤 3：编写最少实现**

`internal/bootstrap/bootstrap.go`：

(a) 在第 29 行 weixin 空导入旁加：

```go
	_ "github.com/rebornace/baize/internal/channel/webhook"
```

(b) 构造 `deps` 时（`deps := channel.BuildDeps{...}` 块内，`ResumeHITL` 字段之后）加：

```go
		Routes: d.srv, // api.Server implements channel.RouteRegistrar
```

(c) 重写装配循环为「描述符遍历（空配置）+ 配置实例遍历（声明式）」两路。把现有 `for _, desc := range channel.Descriptors() { ... }` 循环体抽取为一个闭包，再按两种模式驱动。最小改动做法：在现有 `wireChannels` 中，把 `deps` 构造保留，替换其后的循环逻辑为：

```go
	// assemble builds and wires one instance. instName is the api handle key
	// (descriptor name for legacy; configured instance name for declarative).
	assemble := func(desc channel.Descriptor, instName string, overrides map[string]string) error {
		chCfg := channel.Config{"name": instName, "creds_dir": desc.DefaultCredsDir}
		for k, v := range overrides {
			chCfg[k] = v
		}
		ch, err := desc.Build(chCfg)
		if err != nil {
			return fmt.Errorf("open channel %s: %w", instName, err)
		}
		handle := &api.ChannelHandle{
			Name:     instName,
			Channel:  ch,
			CredsDir: chCfg["creds_dir"],
			RunCtx:   d.runCtx,
		}
		if bs, isBoot := ch.(channel.Bootstrapper); isBoot {
			rt, dir, start, err := bs.Bootstrap(deps)
			if err != nil {
				return fmt.Errorf("bootstrap channel %s: %w", instName, err)
			}
			handle.Runtime = rt
			if dir != "" {
				handle.CredsDir = dir
			}
			router.Add(ch)
			router.BindRuntime(rt)
			stopCh := ch
			d.closer.stops = append(d.closer.stops, func() {
				stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = stopCh.Stop(stopCtx)
			})
			if start {
				if err := ch.Start(d.runCtx); err != nil {
					log.Printf("%s channel: start skipped: %v", instName, err)
				} else {
					log.Printf("%s channel: started (creds_dir=%s)", instName, handle.CredsDir)
				}
			}
		}
		d.srv.RegisterChannel(handle)
		return nil
	}

	if !declarative {
		// Legacy/back-compat: wire every registered descriptor once by its type name.
		for _, desc := range channel.Descriptors() {
			if err := assemble(desc, desc.Name, nil); err != nil {
				return nil, err
			}
		}
	} else {
		seenName := map[string]bool{}
		listedType := map[string]bool{}
		for _, cc := range d.cfg.Channels {
			listedType[strings.TrimSpace(cc.Type)] = true
		}
		for _, cc := range d.cfg.Channels {
			typ := strings.TrimSpace(cc.Type)
			if typ == "" {
				continue
			}
			desc, ok := channel.Describe(typ)
			if !ok {
				return nil, fmt.Errorf("unknown channel type %q", typ)
			}
			instName := strings.TrimSpace(cc.Name)
			if instName == "" {
				instName = typ
			}
			if seenName[instName] {
				return nil, fmt.Errorf("duplicate channel instance name %q", instName)
			}
			seenName[instName] = true
			if !cc.Enabled {
				log.Printf("%s channel: disabled by config; skipped", instName)
				continue
			}
			if err := assemble(desc, instName, cc.Config); err != nil {
				return nil, err
			}
		}
		// Built-in defaults not explicitly listed stay wired (back-compat).
		for _, desc := range channel.Descriptors() {
			if desc.EnabledByDefault && !listedType[desc.Name] {
				if err := assemble(desc, desc.Name, nil); err != nil {
					return nil, err
				}
			}
		}
	}
```

> 旧的 `enabled/overrides/channelEnabled` 局部变量与原 `for _, desc := range channel.Descriptors()` 循环整体删除，由上面取代。保留 `router/deps` 构造与函数尾部 `d.srv.Outbound = router` 等接线不变。注意 `assemble` 里 webhook 多次实例的 `router.Add` 依赖 `Source()` 唯一；重复 source 会在 `router.Add` 静默覆盖——因此实例配置 `source` 必须唯一（默认=唯一实例名，天然唯一）。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/bootstrap/...`
预期：PASS（新多实例/重名用例 + 既有空配置/声明式/禁用/creds_dir 覆盖用例全部不回归）。

- [ ] **步骤 5：全量回归 + Commit**

运行：`go build ./... && go test ./...`
预期：全绿（weixin 既有行为不变）。

```bash
git add internal/bootstrap/bootstrap.go internal/bootstrap/wire_channels_test.go
git commit -m "feat(bootstrap): wireChannels 声明式按实例装配支持 webhook 多实例 + 注入 Routes"
```

---

## 任务 8：端到端集成测试（适配器↔baize 双向闭环）

**文件：**
- 创建：`tests/integration/webhook_channel_test.go`

这个测试用 `bootstrap.StartForTest` 起真实 baize，用 `httptest` 起一个假适配器，验证：签名入站 → 建 run → mock 引擎产出 → 出站签名 POST 回适配器。

- [ ] **步骤 1：编写失败测试**

`tests/integration/webhook_channel_test.go`：

```go
package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/webhooksig"
)

// outboundCapture is a fake adapter: it records baize->adapter posts.
type outboundCapture struct {
	mu   sync.Mutex
	got  []map[string]any
}

func (o *outboundCapture) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		o.mu.Lock()
		o.got = append(o.got, m)
		o.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})
}

func (o *outboundCapture) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.got)
}

func TestWebhookChannelBidirectionalE2E(t *testing.T) {
	adapter := &outboundCapture{}
	adapterSrv := httptest.NewServer(adapter.handler())
	defer adapterSrv.Close()

	const secret = "e2e-secret"
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "wh-agent"
	cfg.Agent.System = "你是测试助手。"
	cfg.Channels = []config.ChannelConfig{{
		Name:    "feishu",
		Type:    "webhook",
		Enabled: true,
		Config: map[string]string{
			"source":       "feishu",
			"account":      "feishu-bot",
			"secret":       secret,
			"outbound_url": adapterSrv.URL,
			"assignee":     "u-admin",
			"agent_id":     "wh-agent",
		},
	}}

	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	// Adapter -> baize: signed inbound message.
	inbound := map[string]any{
		"event":   "message",
		"peer":    map[string]any{"id": "peer-42", "name": "Alice"},
		"text":    "你好",
		"account": "feishu-bot",
	}
	body, _ := json.Marshal(inbound)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign(secret, ts, body)

	req, _ := http.NewRequest("POST", runtimeURL+"/v0/channels/feishu/inbound", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("inbound post: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inbound status=%d body=%s", resp.StatusCode, raw)
	}

	// Wait for the engine to produce an assistant reply pushed to the adapter.
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.count() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if adapter.count() == 0 {
		t.Fatal("adapter never received an outbound message")
	}
	adapter.mu.Lock()
	first := adapter.got[0]
	adapter.mu.Unlock()
	if peer, _ := first["peer"].(map[string]any); peer["id"] != "peer-42" {
		t.Fatalf("outbound peer mismatch: %+v", first["peer"])
	}
	if first["account"] != "feishu-bot" {
		t.Fatalf("outbound account mismatch: %+v", first["account"])
	}
	if _, ok := first["text"].(string); !ok {
		t.Fatalf("outbound missing text: %+v", first)
	}
}

func TestWebhookChannelRejectsBadSignatureE2E(t *testing.T) {
	adapter := &outboundCapture{}
	adapterSrv := httptest.NewServer(adapter.handler())
	defer adapterSrv.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "wh-agent"
	cfg.Channels = []config.ChannelConfig{{
		Name: "feishu", Type: "webhook", Enabled: true,
		Config: map[string]string{"source": "feishu", "account": "b", "secret": "right",
			"outbound_url": adapterSrv.URL, "assignee": "u", "agent_id": "wh-agent"},
	}}
	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	body, _ := json.Marshal(map[string]any{"event": "message", "peer": map[string]any{"id": "p"}, "text": "x"})
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhooksig.Sign("wrong", ts, body) // signed with wrong secret
	req, _ := http.NewRequest("POST", runtimeURL+"/v0/channels/feishu/inbound", bytes.NewReader(body))
	req.Header.Set("X-Baize-Channel-Timestamp", ts)
	req.Header.Set("X-Baize-Channel-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if adapter.count() != 0 {
		t.Fatal("no outbound should happen on rejected inbound")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./tests/integration/... -run WebhookChannel -v`
预期：FAIL（路由 404 / 渠道未装配，因为任务 7 前 webhook 不进装配；任务 7 完成后此测试应转绿）。

> 依赖：本测试依赖任务 7 的装配改造。若任务 7 已合入，此处应直接 PASS；若先写本测试会在路由处 404，属预期失败。

- [ ] **步骤 3：验证通过**

运行：`go test ./tests/integration/... -run WebhookChannel -v`
预期：PASS（双向闭环：入站建 run → mock 回复 → 适配器收到出站；坏签名 401 且无出站）。

- [ ] **步骤 4：Commit**

```bash
git add tests/integration/webhook_channel_test.go
git commit -m "test(integration): webhook 渠道适配器双向闭环 + 坏签名拒绝"
```

---

## 任务 9：参考适配器 `examples/im-adapter`（仅标准库）

**文件：**
- 创建：`examples/im-adapter/main.go`
- 创建：`examples/im-adapter/README.md`

这是一个可运行的最小适配器：接收 baize 出站（验签并打印），并提供一个 `/send` 端点模拟 IM 用户发来消息（签名后 POST 回 baize 入站）。纯标准库，可作为任意语言实现的协议参照。

- [ ] **步骤 1：编写实现**

`examples/im-adapter/main.go`：

```go
// Command im-adapter is a minimal, stdlib-only reference adapter for the baize
// out-of-process webhook channel protocol. It:
//   - serves POST /outbound to receive messages FROM baize (verifies HMAC),
//   - serves POST /send?peer=<id>&text=<msg> to simulate an inbound IM message,
//     signing it and POSTing to baize's inbound webhook.
//
// Configure via flags: -baize (inbound url), -secret (shared HMAC), -addr.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	hTimestamp = "X-Baize-Channel-Timestamp"
	hSignature = "X-Baize-Channel-Signature"
	hProtocol  = "X-Baize-Protocol"
)

func sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "v1=" + hex.EncodeToString(mac.Sum(nil))
}

func verify(secret, timestamp string, body []byte, sig string) bool {
	want := sign(secret, timestamp, body)
	return hmac.Equal([]byte(want), []byte(sig))
}

func main() {
	baize := flag.String("baize", env("BAIZE_INBOUND_URL", "http://127.0.0.1:8080/v0/channels/demo/inbound"), "baize inbound webhook url")
	secret := flag.String("secret", env("CHANNEL_SECRET", "dev-secret"), "shared HMAC secret")
	addr := flag.String("addr", ":9100", "listen address")
	flag.Parse()

	http.HandleFunc("/outbound", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if !verify(*secret, r.Header.Get(hTimestamp), body, r.Header.Get(hSignature)) {
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		var msg map[string]any
		_ = json.Unmarshal(body, &msg)
		log.Printf("[baize->adapter] %v", msg)
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		peer := r.URL.Query().Get("peer")
		text := r.URL.Query().Get("text")
		if peer == "" || text == "" {
			http.Error(w, "peer and text required", http.StatusBadRequest)
			return
		}
		inbound := map[string]any{
			"event": "message",
			"peer":  map[string]any{"id": peer, "name": peer},
			"text":  text,
		}
		body, _ := json.Marshal(inbound)
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req, _ := http.NewRequest(http.MethodPost, *baize, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(hProtocol, "v0")
		req.Header.Set(hTimestamp, ts)
		req.Header.Set(hSignature, sign(*secret, ts, body))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		fmt.Fprintf(w, "baize responded %d\n", resp.StatusCode)
	})

	log.Printf("im-adapter listening on %s (baize=%s)", *addr, *baize)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
```

`examples/im-adapter/README.md`：

```markdown
# im-adapter（参考适配器）

baize 进程外 webhook 渠道的最小参考实现（Go 标准库，无第三方依赖），演示
适配器↔baize 的 JSON-over-HTTP + HMAC 协议。任意语言可照此实现。

## 运行

1. baize 配置一个 webhook 渠道实例（config.yaml）：

   ```yaml
   channels:
     - name: demo
       type: webhook
       enabled: true
       config:
         source: demo
         secret: dev-secret
         outbound_url: http://127.0.0.1:9100/outbound
         assignee: u-admin
   ```

2. 启动适配器：

   ```bash
   go run ./examples/im-adapter -baize http://127.0.0.1:8080/v0/channels/demo/inbound -secret dev-secret
   ```

3. 模拟 IM 用户发来消息：

   ```bash
   curl "http://127.0.0.1:9100/send?peer=alice&text=你好"
   ```

   baize 建 run、引擎产出后，适配器日志会打印 `[baize->adapter] ...` 出站消息。

## 协议要点

- 入站（适配器→baize）：`POST {baize}/v0/channels/{name}/inbound`，头
  `X-Baize-Channel-Timestamp`（unix 秒）、`X-Baize-Channel-Signature`
  （`v1=` + HMAC-SHA256(secret, timestamp+"."+body)）。
- 出站（baize→适配器）：`POST outbound_url`，同样的签名头；适配器必须验签。
- 时间窗 ±300s；重放/过期请求被拒。
```

- [ ] **步骤 2：构建验证**

运行：`go build ./examples/im-adapter/ && go vet ./examples/im-adapter/`
预期：通过（记得用 `bytes.NewReader` 后能编译）。

- [ ] **步骤 3：Commit**

```bash
git add examples/im-adapter/
git commit -m "docs(examples): 新增 im-adapter 标准库参考适配器"
```

---

## 任务 10：配置示例 + 全量回归收尾

**文件：**
- 修改：`configs/minimal.yaml`（加 webhook 渠道注释示例）

- [ ] **步骤 1：在 `configs/minimal.yaml` 追加注释示例**

```yaml
# 进程外 IM 渠道（webhook 适配器）。一个实例 = 一个 IM 账号；多账号写多条。
# channels:
#   - name: feishu            # 实例名（缺省=type）；同 type 多实例时必填且唯一
#     type: webhook
#     enabled: true
#     config:
#       source: feishu        # convID 前缀/meta.Source；缺省=实例名，需唯一
#       account: feishu-bot   # IM 账号标识；缺省=实例名
#       secret: ""            # 入站验签 HMAC 密钥（必填）
#       outbound_url: ""      # 适配器出站接收地址（必填）
#       outbound_secret: ""   # 出站签名密钥（缺省=secret）
#       assignee: ""          # 会话归属操作员（必填）
#       agent_id: ""          # 指定 agent（缺省用默认 agent）
#       supports_vision: "false"
#       allowlist: ""         # 逗号分隔；配置后强制入站白名单
```

- [ ] **步骤 2：全量回归**

运行：

```bash
gofmt -l . 
go vet ./...
go build ./...
go test ./...
```

预期：`gofmt -l` 无输出；vet 干净；build 通过；全部测试 PASS（含 `tests/integration`，weixin 既有行为零回归）。

- [ ] **步骤 3：Commit**

```bash
git add configs/minimal.yaml
git commit -m "docs(config): minimal.yaml 增加 webhook 渠道配置示例"
```

---

## 2A 范围之外（明确不做）

- **2B 微信适配器迁移**：把进程内 weixin 逻辑移植为外部 `weixin-adapter`、登录/状态管理面协议、`admin_url` 反向管理调用、子进程托管、UI 渠道管理页——均属 2B，2A 落地后另出计划。规格 §11 的微信功能对等回归清单在 2B 执行。
- **webhook 实例的 UI 增删改**：2A 用 config.yaml 声明式配置；运行时 UI 管理渠道实例留待后续。
- **出站大文件 URL 化 / blob 存储**：2A 出站小文件内联 base64；大文件外链在需要时再做。

## 自检结论

- **规格覆盖度（2A）：** §3 配置→任务 3/7；§4 路由自注册→任务 2/7；§5 协议 DTO→任务 3、入站任务 6、出站任务 4/5；§6 媒体（内联 base64 + URL 下载、图片→多模态 part）→任务 6；HMAC/时间窗→任务 1/4/6；多实例→任务 7；E2E→任务 8；参考适配器→任务 9。§7 管理面/admin_url/§8 子进程/微信迁移=2B，已在"范围之外"标注。
- **类型一致性：** `instanceConfig` 字段（Source/Account/Secret/OutboundSecret/OutboundURL/Assignee/AgentID/SupportsVision/Allowlist）在任务 3 定义，任务 4/5/6 引用一致；`openFromConfig(name, map)` 任务 5 定义、init 工厂与任务 7 调用一致；`inboundHandler()` 任务 5 注册、任务 6 实现；`newSender(cfg)` 任务 4 定义、任务 5 使用；协议头常量/`OutboundMessage`/`InboundMessage`/`Peer`/`Attachment`/`OutboundMedia` 任务 3 定义、任务 4/5/6/8 引用一致；`BuildDeps.Routes` 任务 2 定义、任务 5/7 使用一致。
- **占位符：** 无 TODO/待定；每个代码步骤均含完整代码。测试辅助（`newCaptureServer`、fake runs、httptest 适配器）均给出实现。
