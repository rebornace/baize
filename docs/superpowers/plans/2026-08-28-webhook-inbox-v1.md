# HTTP Webhook Inbox v1 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 外部系统 HMAC 签名 POST → Baize Inbox 建 Run（幂等 + 续聊）；管理员 UI 管理 Channel；与出站 Webhook 组成生产集成闭环。

**架构：** 新建 `internal/inbox`（验签/Registry/限速）；`store` 增 `inbox_deliveries`/`inbox_threads`；Channel 配置走 settings KV `inbox_channels`；`server` 抽取 `startRun` 内核供 `POST /v0/runs` 与 `POST /v0/inbox/{id}` 共用；前端 `InboxSettings` 对称 Webhook 页。

**技术栈：** Go 1.25 / crypto/hmac / 既有 store+SQLite / React 19 + vitest。

**规格：** `docs/superpowers/specs/2026-08-28-webhook-inbox-v1-design.md`

**Git：** 分支 `feat/webhook-inbox-v1`

**环境（Windows）：**

```powershell
$env:Path = "C:\Users\Administrator\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.25.0.windows-amd64\bin;D:\Git\bin;" + $env:Path
$env:GOTOOLCHAIN = "local"; $env:GOPROXY = "https://goproxy.cn,direct"
cd C:\Users\Administrator\Desktop\baize
```

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/inbox/model.go` | Channel、Payload、Delivery 类型 + Validate |
| `internal/inbox/verify.go` | HMAC v1 签名/验签 + 时间戳 |
| `internal/inbox/registry.go` | 内存 Channel 表 + 热更新 |
| `internal/inbox/ratelimit.go` | 每 channel 120/min 滑动窗口 |
| `internal/inbox/sign_test.go` | 验签/签名测试辅助 |
| 各 `_test.go` | 单测 |
| `internal/store/store.go` | InboxDelivery/InboxThread 类型 + Store 接口扩展 |
| `internal/store/memory.go` / `sqlite.go` | 表/ map 实现 |
| `internal/config/config.go` | `inbox.channels` yaml |
| `internal/api/run_start.go` | 抽取 `startRun`（原 handlePostRun 内核） |
| `internal/api/server_inbox.go` | 入站 + settings handlers |
| `internal/api/server.go` | 路由注册；handlePostRun 改调 startRun |
| `internal/bootstrap/bootstrap.go` | seed inbox channels + 注入 Registry |
| `internal/controlplane/acl.go` | Inbox 路由 ACL |
| `internal/run/events.go` 或 `engine.go` | 常量 `EventInboxReceived = "inbox.received"`（若无则加） |
| `web/chat/src/pages/InboxSettings.tsx` + test | 设置 UI |
| `web/chat/src/api.ts` | inbox settings API |
| `web/chat/src/settingsNav.ts` + test | 导航项 |
| `web/chat/src/main.tsx` | 路由 |
| `tests/integration/inbox_test.go` | E2E |
| `examples/inbox-alert/` | 示例脚本 + README |
| README ×2、`docs/architecture-and-plugin-protocol.md` | 文档 |

### 计划级裁定（实现者必须遵守）

1. **幂等 body_hash**：使用 HTTP 请求的 **原始 body 字节**（验签前读到的 `[]byte`），不做 JSON 重序列化。
2. **幂等重放 HTTP 状态**：同 key 同 hash → **200 OK** + 相同 JSON；首次接受 → **202 Accepted**。
3. **PUT settings 保 secret**：客户端 PUT 的 channel 条目若 **省略 `secret` 字段**，合并保留 store 中已有 secret（同 login capture preserve 模式）；新建 channel 无 secret 则服务端生成。
4. **delivery_id 前缀**：`dlv_` + UUID（与 `run_` 一致风格）。
5. **Inbox 不走** `@skill` 解析、附件、session_token；skills 仅来自 Channel 配置非空数组，否则 Agent 默认。
6. **事件顺序**：`CreateRun` → `AppendEvent(inbox.received)` → `AppendEvent(run.started)` → 异步 Execute。
7. **Gate 开启时** Inbox 入站仍为 `RoleNone`；settings 路由为 `RoleAdmin`。

---

### 任务 0：分支

- [ ] **步骤 1：创建分支**

```powershell
git checkout -b feat/webhook-inbox-v1
```

---

### 任务 1：inbox 模型与 HMAC 验签

**文件：**
- 创建：`internal/inbox/model.go`、`internal/inbox/model_test.go`
- 创建：`internal/inbox/verify.go`、`internal/inbox/verify_test.go`

- [ ] **步骤 1：编写失败的测试**

`model_test.go`：

```go
package inbox_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/inbox"
)

func TestChannelValidateRejectsBadID(t *testing.T) {
	c := inbox.Channel{ID: "Bad", AgentID: "a", Secret: "s"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("err=%v", err)
	}
}

func TestPayloadValidateRequiresInput(t *testing.T) {
	p := inbox.Payload{}
	if err := p.Validate(); err == nil {
		t.Fatal("want input required")
	}
}
```

`verify_test.go`：

```go
package inbox_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/inbox"
)

func TestSignAndVerifyOK(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"input":"hi"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := inbox.Sign(secret, ts, body)
	if err := inbox.Verify(secret, ts, body, sig, time.Now(), 300*time.Second); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyRejectsSkew(t *testing.T) {
	secret := "s"
	body := []byte(`{}`)
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	sig := inbox.Sign(secret, ts, body)
	if err := inbox.Verify(secret, ts, body, sig, time.Now(), 300*time.Second); err != inbox.ErrTimestampSkew {
		t.Fatalf("err=%v want skew", err)
	}
}
```

- [ ] **步骤 2：运行验证失败**

```powershell
go test ./internal/inbox/ -count=1
```

- [ ] **步骤 3：实现**

`model.go`：`Channel`、`Payload`、`DeliveryRecord`；`Channel.Validate()` 校验 id 正则、agent_id、secret 非空；`Payload.Validate()` input trim 长度 1–8192。

`verify.go`：

```go
const signaturePrefix = "v1="

func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

func Verify(secret, timestamp string, body []byte, headerSig string, now time.Time, maxSkew time.Duration) error
```

导出 `ErrInvalidSignature`、`ErrTimestampSkew`；header 必须以 `v1=` 开头且 hex 解码后 `hmac.Equal`。

- [ ] **步骤 4：测试通过**

```powershell
go test ./internal/inbox/ -count=1
```

- [ ] **步骤 5：Commit**

```powershell
git add internal/inbox/
git commit -m "feat(inbox): Channel 模型与 HMAC v1 验签"
```

---

### 任务 2：Store 幂等表与会话映射

**文件：**
- 修改：`internal/store/store.go`
- 修改：`internal/store/memory.go`、`internal/store/sqlite.go`
- 修改：`internal/store/store_test.go`、`internal/store/sqlite_test.go`

- [ ] **步骤 1：编写失败的测试**

`store_test.go` 追加：

```go
func TestInboxDeliveryRoundTrip(t *testing.T) {
	s := store.NewMemory()
	d := store.InboxDelivery{
		ChannelID: "alerts", IdempotencyKey: "k1",
		DeliveryID: "dlv_x", RunID: "run_y", BodyHash: "abc",
	}
	if err := s.PutInboxDelivery(d); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetInboxDelivery("alerts", "k1")
	if err != nil || !ok || got.RunID != "run_y" {
		t.Fatalf("got=%+v ok=%v err=%v", got, ok, err)
	}
}

func TestInboxThreadRoundTrip(t *testing.T) {
	s := store.NewMemory()
	if err := s.PutInboxThread("alerts", "jira-1", "conv-1"); err != nil {
		t.Fatal(err)
	}
	conv, ok, err := s.GetInboxThread("alerts", "jira-1")
	if err != nil || !ok || conv != "conv-1" {
		t.Fatalf("conv=%q ok=%v", conv, ok)
	}
}
```

- [ ] **步骤 2：运行验证失败**

```powershell
go test ./internal/store/ -run TestInbox -count=1
```

- [ ] **步骤 3：实现**

`store.go` 增加：

```go
const SettingKeyInboxChannels = "inbox_channels"

type InboxDelivery struct {
	ChannelID, IdempotencyKey, DeliveryID, RunID, BodyHash string
	CreatedAt time.Time
}

type Store interface {
	// ...existing...
	GetInboxDelivery(channelID, idempotencyKey string) (InboxDelivery, bool, error)
	PutInboxDelivery(d InboxDelivery) error
	GetInboxThread(channelID, externalID string) (conversationID string, ok bool, err error)
	PutInboxThread(channelID, externalID, conversationID string) error
}
```

SQLite：`inbox_deliveries` / `inbox_threads` 表 + UNIQUE 约束；Memory：嵌套 map。  
`PutInboxDelivery` 同 key 已存在 → 返回 error 或依赖上层先 Get（上层处理 409）；实现 **Get-or-put 由 API 层做**，Store 仅 CRUD。

- [ ] **步骤 4：SQLite 测试同步**

`sqlite_test.go` 复制 Memory 两个测试。

- [ ] **步骤 5：Commit**

```powershell
git add internal/store/
git commit -m "feat(store): Inbox 幂等投递与会话线程表"
```

---

### 任务 3：Registry、限速、配置与 bootstrap

**文件：**
- 创建：`internal/inbox/registry.go`、`internal/inbox/registry_test.go`
- 创建：`internal/inbox/ratelimit.go`、`internal/inbox/ratelimit_test.go`
- 修改：`internal/config/config.go`
- 修改：`internal/bootstrap/bootstrap.go`
- 修改：`configs/minimal.yaml`、`configs/demo.yaml`（`inbox: { channels: [] }`）

- [ ] **步骤 1：编写失败的测试**

```go
func TestRegistryGetDisabledChannel(t *testing.T) {
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "a", AgentID: "x", Secret: "s", Enabled: false}})
	_, ok := reg.Get("a")
	if ok {
		t.Fatal("disabled must not be returned for inbound")
	}
}

func TestRateLimitBlocksBurst(t *testing.T) {
	rl := inbox.NewRateLimiter(2, time.Minute)
	if !rl.Allow("a") || !rl.Allow("a") {
		t.Fatal("first two should pass")
	}
	if rl.Allow("a") {
		t.Fatal("third should block")
	}
}
```

- [ ] **步骤 2：运行验证失败**

```powershell
go test ./internal/inbox/ -run "TestRegistry|TestRateLimit" -count=1
```

- [ ] **步骤 3：实现**

`registry.go`：`Replace([]Channel)`、`Get(id) (Channel, bool)`（enabled+secret 非空）、`SecretHint(secret string) string`（末 4 字符）。

`ratelimit.go`：每 channel 滑动窗口计数（mutex + map[channel]timestamps slice）。

`config.go`：

```go
type InboxConfig struct {
	Channels []inbox.Channel `yaml:"channels"`
}
```

bootstrap：`seedInboxChannels(cfg, st, reg)` — store 有 KV 则 JSON 载入 Registry；否则 yaml seed，secret 空则 `inbox.GenerateSecret()` 并 UpsertSetting + log 警告。

`Server` 增加字段 `Inbox *inbox.Registry`（或嵌入 Registry 指针）。

- [ ] **步骤 4：Commit**

```powershell
git add internal/inbox/registry.go internal/inbox/ratelimit.go internal/config/ internal/bootstrap/ configs/
git commit -m "feat(inbox): Channel Registry、限速与 bootstrap 种子"
```

---

### 任务 4：抽取 startRun + 入站 handler

**文件：**
- 创建：`internal/api/run_start.go`
- 创建：`internal/api/server_inbox.go`
- 修改：`internal/api/server.go`（路由、`handlePostRun` 瘦身）
- 修改：`internal/run/engine.go` 或新建 `events.go` — `EventInboxReceived = "inbox.received"`

- [ ] **步骤 1：编写失败的 API 测试**

`internal/api/server_inbox_test.go`：

```go
func TestInboxPostRequiresSignature(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "hi"})
	reg := inbox.NewRegistry()
	reg.Replace([]inbox.Channel{{ID: "alerts", AgentID: "a", Secret: "sec", Enabled: true}})
	srv := testServerWithInbox(t, st, reg)
	req := httptest.NewRequest(http.MethodPost, "/v0/inbox/alerts",
		strings.NewReader(`{"input":"hello"}`))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestInboxPostCreatesRun(t *testing.T) {
	// 签名正确 → 202 + run_id；events 含 inbox.received
}
```

- [ ] **步骤 2：运行验证失败**

```powershell
go test ./internal/api/ -run TestInbox -count=1
```

- [ ] **步骤 3：实现 run_start.go**

```go
type startRunInput struct {
	AgentID, Input, ConversationID, IdentityID string
	Skills []string
	Webhook *store.WebhookConfig
	Passthrough map[string]string
	UserParts []llm.ContentPart
	PreEvents []store.Event
}

func (s *Server) startRun(ctx context.Context, in startRunInput) (*store.Run, error)
```

逻辑从 `handlePostRun` 抽出：`CreateRun` → 可选 Messages.Append → 依次 `PreEvents` → `run.started` → go `runExecute`。

`handlePostRun` 改为解析 body 后构造 `startRunInput` 调用 `startRun`，响应不变。

- [ ] **步骤 4：实现 server_inbox.go `handlePostInbox`**

流程：

1. `io.LimitReader` 64KiB 读 body  
2. Registry.Get(channel_id) → 404  
3. RateLimiter.Allow → 429 + Retry-After  
4. Verify HMAC  
5. JSON decode Payload + Validate  
6. 幂等：有 idempotency_key → GetInboxDelivery；命中且 hash 同 → 200 返回缓存；hash 异 → 409  
7. 会话：`resolveConversation(store, channel, payload)`  
8. delivery_id = `dlv_`+uuid；构造 startRunInput（webhook 来自 channel）；PreEvents = inbox.received  
9. startRun → PutInboxDelivery + 可选 PutInboxThread  
10. 202 JSON

辅助：`readSignedInboxBody(r, maxBytes) ([]byte, timestamp, sigHeader, error)`

- [ ] **步骤 5：server.go 注册**

```go
s.mux.HandleFunc("POST /v0/inbox/{channel_id}", s.handlePostInbox)
```

- [ ] **步骤 6：测试通过**

```powershell
go test ./internal/api/ -run TestInbox -count=1
```

- [ ] **步骤 7：Commit**

```powershell
git add internal/api/run_start.go internal/api/server_inbox.go internal/api/server.go internal/run/
git commit -m "feat(api): Webhook Inbox 入站 POST 与 startRun 抽取"
```

---

### 任务 5：Settings API + ACL

**文件：**
- 修改：`internal/api/server_inbox.go`（settings handlers）
- 修改：`internal/controlplane/acl.go`
- 测试：`internal/api/server_inbox_test.go` 追加

- [ ] **步骤 1：编写失败的测试**

```go
func TestPutInboxChannelsPreservesSecret(t *testing.T) {
	// PUT 不带 secret 字段 → GET secret_hint 不变，验签仍可用旧 secret
}

func TestRotateInboxSecret(t *testing.T) {
	// POST rotate → 响应含明文 secret 一次；旧签名失效
}
```

- [ ] **步骤 2：实现 handlers**

- `handleGetInboxChannels` — 列表，secret → `secret_hint` only  
- `handlePutInboxChannels` — 校验 agent 存在、id 唯一、Validate；merge secret；UpsertSetting + Registry.Replace  
- `handlePostInboxRotateSecret` — 新 secret，返回 `{ "secret": "..." }`  
- `handlePostInboxTest` — 内部调用 Sign + handlePostInbox 逻辑或 httptest 自 POST

ACL 追加：

```go
{method: "POST", segments: []string{"v0", "inbox", "{id}"}, role: RoleNone},
{method: "GET", segments: []string{"v0", "settings", "inbox-channels"}, role: RoleAdmin},
// PUT, rotate, test ...
```

- [ ] **步骤 3：Commit**

```powershell
git add internal/api/ internal/controlplane/
git commit -m "feat(api): Inbox Channel 设置 API 与 ACL"
```

---

### 任务 6：集成测试

**文件：**
- 创建：`tests/integration/inbox_test.go`

- [ ] **步骤 1：编写集成测试**

```go
func TestInboxE2ECreatesRun(t *testing.T) {
	// bootstrap.StartForTest；PUT channels（或 yaml）；签名 POST；poll succeeded
}

func TestInboxIdempotencyReplay(t *testing.T) {
	// 同 key 两次 → 同 run_id；第二次 200
}

func TestInboxExternalIDThreadsConversation(t *testing.T) {
	// 同 external_id 两次 → 同 conversation_id
}
```

复用 `tests/integration` 的 `pollRun`；新增 `signInboxPOST(secret, body)` 辅助。

- [ ] **步骤 2：运行**

```powershell
go test ./tests/integration/ -run TestInbox -count=1 -v
```

- [ ] **步骤 3：Commit**

```powershell
git add tests/integration/inbox_test.go
git commit -m "test(integration): Webhook Inbox v1 E2E"
```

---

### 任务 7：前端 InboxSettings

**文件：**
- 创建：`web/chat/src/pages/InboxSettings.tsx`、`InboxSettings.test.ts`
- 修改：`web/chat/src/api.ts`、`settingsNav.ts`、`settingsNav.test.ts`、`main.tsx`

- [ ] **步骤 1：编写失败的测试**

`InboxSettings.test.ts`：

```ts
import { describe, expect, it } from 'vitest'
import { channelsToForm, validateChannelsForm } from './InboxSettings'

describe('validateChannelsForm', () => {
	it('requires id and agent_id', () => {
		const r = validateChannelsForm([{ id: '', agent_id: 'a', enabled: true }])
		expect(r.ok).toBe(false)
	})
})
```

- [ ] **步骤 2：api.ts 增加**

```ts
export type InboxChannel = { id: string; agent_id: string; enabled: boolean; skills?: string[]; description?: string; webhook_url?: string; secret_hint?: string }
export async function getInboxChannels(): Promise<InboxChannel[]>
export async function putInboxChannels(channels: InboxChannel[]): Promise<void>
export async function rotateInboxSecret(id: string): Promise<{ secret: string }>
export async function testInboxChannel(id: string): Promise<{ delivery_id: string; run_id: string }>
```

- [ ] **步骤 3：实现 InboxSettings.tsx**

- Channel 列表编辑（增删行）  
- 字段：id、agent_id（下拉 agents）、enabled、skills 多选、description、webhook_url/headers  
- 按钮：保存、轮换 Secret（modal 展示一次）、复制入站 URL（`window.location.origin + '/v0/inbox/' + id`）、发送测试  
- 顶部说明块（规格 §7 文案）

- [ ] **步骤 4：settingsNav + main.tsx 路由 `/settings/inbox`**

- [ ] **步骤 5：前端测试 + build**

```powershell
cd web/chat
npm test -- InboxSettings
npm run build
```

- [ ] **步骤 6：Commit dist**

```powershell
git add web/chat/
git commit -m "feat(web): Inbox Channel 设置页"
```

---

### 任务 8：示例与文档

**文件：**
- 创建：`examples/inbox-alert/README.md`、`examples/inbox-alert/post.ps1`（或 `.sh`）
- 修改：`README.md`、`README.zh-CN.md`、`docs/architecture-and-plugin-protocol.md`

- [ ] **步骤 1：examples/inbox-alert**

PowerShell 脚本演示：读取 `INBOX_SECRET` + `RUNTIME_URL`，构造 body，`X-Baize-Inbox-Timestamp` + `Sign`，POST `/v0/inbox/alerts`。

- [ ] **步骤 2：README「生产集成：Webhook Inbox」**

含：架构一句话、Channel 配置步骤、签名 curl/PowerShell 片段、幂等/续聊说明、安全清单（规格 §11）、与出站 Webhook 配对图（ascii）。

- [ ] **步骤 3：架构 §5 Channel 行更新为已实现**

- [ ] **步骤 4：Commit**

```powershell
git add examples/inbox-alert/ README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md
git commit -m "docs(examples): Webhook Inbox v1 集成指南与告警示例"
```

---

### 任务 9：全量验证

- [ ] **步骤 1**

```powershell
go build ./...
go vet ./...
go test ./... -count=1
cd web/chat; npm test; npx tsc --noEmit
```

- [ ] **步骤 2：合并前自检**

对照规格 §1–§12 逐条勾选；确认 defer 项未意外实现。

---

## 规格覆盖自检

| 规格需求 | 任务 |
|---------|------|
| §3 Channel 配置 + KV 持久化 | 2, 3, 5 |
| §3.3 幂等/线程表 | 2 |
| §4 入站 API + 验签 + 限速 | 1, 4 |
| §4.5 会话解析 | 4 |
| §5 Run 创建 + inbox.received | 4 |
| §6 管理 API | 5 |
| §7 UI | 7 |
| §9 测试 | 1–6, 7 |
| §10 文档/示例 | 8 |
| §11 安全清单 | 8 (README) |
| defer 项 | 无任务 |

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-28-webhook-inbox-v1.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 必需子技能：`subagent-driven-development`。

**2. 内联执行** — 必需子技能：`executing-plans`。

**选哪种方式？**
