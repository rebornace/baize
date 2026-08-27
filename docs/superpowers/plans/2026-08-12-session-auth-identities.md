# 会话身份库与可插拔鉴权解析 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 用会话级多 Identity + 可插拔 AuthResolver 替代写死 `bearer_env`；登录可捕获并复用；UI 可查看/退出/设默认。

**架构：** `conversation_id` 挂在 Run 上；凭证存进程内 `identity.Store`；OpenAPI tool 闭包在 invoke 前 `Resolve` 得到本次 Headers；登录成功按 Capture 配置 upsert；默认 `OpenAPISecurityResolver` 按 operation security scheme 选型。

**技术栈：** Go 1.22+、现有 kin-openapi、SQLite store、Vite + TypeScript（`web/chat`）

**规格：** `docs/superpowers/specs/2026-08-12-session-auth-identities-design.md`

---

## 文件结构（将创建/修改）

| 路径 | 职责 |
|------|------|
| `internal/identity/identity.go` | Identity 模型、PublicView（脱敏） |
| `internal/identity/store.go` | 进程内 Store：List/Upsert/Delete/SetDefault/ClearCaptured |
| `internal/identity/store_test.go` | 身份库单测 |
| `internal/identity/capture.go` | 从 tool 结果按 JSON path / glob 捕获 token |
| `internal/identity/capture_test.go` | 捕获与 JWT claims 摘要单测 |
| `internal/identity/context.go` | ctx 键：ConversationID、ForceIdentityID |
| `internal/authresolve/resolve.go` | `Resolver` 接口 + `ResolveInput` / `Result` |
| `internal/authresolve/openapi.go` | `OpenAPISecurityResolver` |
| `internal/authresolve/openapi_test.go` | 选型顺序单测 |
| `internal/connector/openapi/loader.go` | `ToolRoute.Security []string`（scheme 名） |
| `internal/connector/openapi/invoke.go` | `Invoke` 支持 per-call header 覆盖（不写回 `inv.Headers`） |
| `internal/connector/openapi/register_opts.go` | Capture 配置、Identities、Resolver |
| `internal/connector/openapi/register.go` | 闭包内 Resolve → Invoke → Capture |
| `internal/store/store.go` 等 | Run 增加 `ConversationID`、`IdentityID`；扩展 CreateRun |
| `internal/run/engine.go` | Execute/HITL 从 Run 注入 auth ctx |
| `internal/api/server.go` | runs 字段；identities CRUD API |
| `internal/config/config.go` + `internal/demo/demo.go` | capture yaml；把 Store/Resolver 注入注册 |
| `web/chat/src/api.ts` + `main.ts` + `style.css` | conversation + 账号面板 |
| `internal/ui/dist/**` | 重建嵌入 UI |
| `tests/integration/session_auth_test.go` | 捕获→复用、list 脱敏 |
| `README.md` | 短节：会话身份 |

---

### 任务 1：Identity 模型与内存 Store

**文件：**
- 创建：`internal/identity/identity.go`
- 创建：`internal/identity/store.go`
- 创建：`internal/identity/store_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
package identity_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/identity"
)

func TestStoreUpsertListDeleteDefault(t *testing.T) {
	s := identity.NewMemoryStore()
	now := time.Now().UTC()
	id, err := s.Upsert("conv1", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer SECRET_TOKEN"},
		Source:            identity.SourceLoginCapture,
		Subject:           "admin@x.com",
		IsDefault:         true,
		CreatedAt:         now,
		UpdatedAt:         now,
	})
	if err != nil || id == "" {
		t.Fatalf("upsert: id=%q err=%v", id, err)
	}
	views := s.ListPublic("conv1")
	if len(views) != 1 || views[0].Label != "admin@x.com" || views[0].IsDefault != true {
		t.Fatalf("views=%+v", views)
	}
	raw, _ := json.Marshal(views)
	if strings.Contains(string(raw), "SECRET_TOKEN") {
		t.Fatal("public list leaked token")
	}
	if err := s.SetDefault("conv1", id); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("conv1", id); err != nil {
		t.Fatal(err)
	}
	if len(s.ListPublic("conv1")) != 0 {
		t.Fatal("expected empty")
	}
}
```

（测试文件顶部补上 `"encoding/json"` 与 `"strings"` import。）

同文件增加：同 `Scheme`+`Subject` 二次 Upsert 更新同一 `id`、不新增。

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/identity/ -count=1
```

预期：FAIL（包不存在）

- [ ] **步骤 3：最少实现**

`identity.go`：

```go
package identity

import "time"

const (
	SourceEnv          = "env"
	SourceLoginCapture = "login_capture"
	SourceManual       = "manual"
)

type Identity struct {
	ID                string
	Label             string
	Scheme            string
	CredentialHeaders map[string]string
	Source            string
	Subject           string
	ClaimsSummary     map[string]any
	IsDefault         bool
	LastUsedAt        time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PublicView struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	Scheme        string         `json:"scheme,omitempty"`
	Source        string         `json:"source"`
	ClaimsSummary map[string]any `json:"claims_summary,omitempty"`
	IsDefault     bool           `json:"is_default"`
	LastUsedAt    *time.Time     `json:"last_used_at,omitempty"`
}
```

`store.go`：`MemoryStore` + `sync.RWMutex`；`Upsert` 生成 `idt_`+uuid；同 conversation 下 `scheme`+`subject` 命中则更新 headers/label/claims；若 `IsDefault` 则同 scheme 其它项取消默认；`List`/`Get`/`Delete`/`SetDefault`/`ClearCaptured`（只删 `login_capture`/`manual`）；`Touch(conversationID, id)` 更新 `LastUsedAt`；`ListPublic` 映射为 `PublicView`（不含 CredentialHeaders）。

- [ ] **步骤 4：测试通过**

```bash
go test ./internal/identity/ -count=1
```

- [ ] **步骤 5：Commit**

```bash
git add internal/identity/
git commit -m "feat(identity): 会话身份内存 Store 与脱敏视图"
```

---

### 任务 2：ctx 键、登录捕获、JWT 摘要

**文件：**
- 创建：`internal/identity/context.go`
- 创建：`internal/identity/capture.go`
- 创建：`internal/identity/capture_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestCaptureAccessToken(t *testing.T) {
	cfg := identity.CaptureConfig{
		ToolNameGlob:    "*login*",
		TokenJSONPaths:  []string{"accessToken", "data.token"},
		LabelJSONPaths:  []string{"email"},
		HeaderTemplate:  "Bearer {{token}}",
		DefaultScheme:   "bearer",
	}
	if !identity.MatchToolName(cfg.ToolNameGlob, "AdminAuthController_login") {
		t.Fatal("glob")
	}
	headers, label, subject, claims, ok := identity.ExtractCredential(cfg, map[string]any{
		"accessToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbkB4LmNvbSIsImVtYWlsIjoiYWRtaW5AeC5jb20iLCJyb2xlcyI6WyJhZG1pbiJdLCJleHAiOjk5OTk5OTk5OTl9.sig",
		"email":       "admin@x.com",
	})
	if !ok || headers["Authorization"] == "" || label == "" {
		t.Fatalf("extract: %+v %q %v", headers, label, ok)
	}
	_ = subject
	_ = claims
}
```

用真实可解析的三段 JWT（payload 含 `sub`/`email`/`roles`/`exp`）构造；断言 `claims["sub"]` 或 roles 存在。另测：`is_error` 路径由调用方跳过（本函数只负责 Extract）。

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/identity/ -run TestCapture -count=1
```

- [ ] **步骤 3：最少实现**

- `context.go`：`WithConversationID` / `ConversationIDFrom`；`WithForceIdentityID` / `ForceIdentityIDFrom`（unexported ctx key 类型）
- `CaptureConfig` 字段与规格一致；`MatchToolName` 用 `path.Match`（或简易 `*` 通配）
- `ExtractCredential`：按 path 取嵌套 map（`.` 分段）；token 已含 `Bearer ` 则 HeaderTemplate 可忽略前缀；`ParseJWTClaimsSummary` 只 base64url 解 payload，失败则空 map；`subject` 优先 `sub` 否则 label

- [ ] **步骤 4：测试通过并 commit**

```bash
go test ./internal/identity/ -count=1
git add internal/identity/
git commit -m "feat(identity): 登录结果捕获与 auth ctx"
```

---

### 任务 3：AuthResolver 接口与 OpenAPISecurityResolver

**文件：**
- 创建：`internal/authresolve/resolve.go`
- 创建：`internal/authresolve/openapi.go`
- 创建：`internal/authresolve/openapi_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
func TestOpenAPISecurityResolverOrder(t *testing.T) {
	r := authresolve.OpenAPISecurityResolver{}
	ids := []identity.Identity{
		{ID: "env", Source: identity.SourceEnv, CredentialHeaders: map[string]string{"Authorization": "Bearer ENV"}, Scheme: "bearer"},
		{ID: "a", Source: identity.SourceLoginCapture, Scheme: "bearer", IsDefault: true, CredentialHeaders: map[string]string{"Authorization": "Bearer A"}, Subject: "a"},
		{ID: "b", Source: identity.SourceLoginCapture, Scheme: "other", CredentialHeaders: map[string]string{"Authorization": "Bearer B"}, Subject: "b"},
	}
	res := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:       ids,
		SecuritySchemes:  []string{"bearer"},
		DefaultHeaders:   map[string]string{"Authorization": "Bearer ENV"},
	})
	if res.IdentityID != "a" || res.Headers["Authorization"] != "Bearer A" {
		t.Fatalf("%+v", res)
	}
	res2 := r.Resolve(context.Background(), authresolve.ResolveInput{
		Identities:      ids,
		ForceIdentityID: "b",
		SecuritySchemes: []string{"bearer"},
	})
	if res2.IdentityID != "b" {
		t.Fatalf("%+v", res2)
	}
}
```

再测：无匹配身份 → 回退 `DefaultHeaders`；`ClaimsSummary.exp` 已过期的身份跳过。

- [ ] **步骤 2–4：实现接口、跑通测试、commit**

```go
type Resolver interface {
	Resolve(ctx context.Context, in ResolveInput) Result
}
type ResolveInput struct {
	Identities      []identity.Identity
	SecuritySchemes []string
	DefaultHeaders  map[string]string
	ForceIdentityID string
}
type Result struct {
	Headers    map[string]string
	IdentityID string
	OK         bool
}
```

选型顺序严格按规格 §5.2。过期：`claims_summary["exp"]` 为数值且 `< now`。

```bash
go test ./internal/authresolve/ -count=1
git add internal/authresolve/
git commit -m "feat(authresolve): OpenAPI security 默认 Resolver"
```

---

### 任务 4：Loader Security + Invoke 覆盖 Headers

**文件：**
- 修改：`internal/connector/openapi/loader.go`
- 修改：`internal/connector/openapi/loader_test.go`（或新建小 fixture）
- 修改：`internal/connector/openapi/invoke.go`
- 修改：`internal/connector/openapi/invoke` 相关测试

- [ ] **步骤 1：失败测试 — Security 提取**

用临时 OpenAPI YAML（`t.TempDir()`）含：

```yaml
components:
  securitySchemes:
    bearer:
      type: http
      scheme: bearer
security:
  - bearer: []
paths:
  /me:
    get:
      operationId: getMe
      responses:
        "200": { description: ok }
```

断言 `LoadTools` 后 `getMe` 的 `Security` 为 `["bearer"]`。operation 级 security 覆盖全局。

- [ ] **步骤 2：失败测试 — per-call headers**

`Invoker.Headers` 为 `ENV`；调用 `InvokeWithHeaders(ctx, name, args, map[string]string{"Authorization":"Bearer CAPTURED"})`（或给 `Invoke` 增加可选 overlay——**选定一种 API 并在全计划统一**）：

推荐：

```go
func (inv *Invoker) Invoke(ctx context.Context, toolName string, args map[string]any) (InvokeResult, error) {
	return inv.invoke(ctx, toolName, args, nil)
}
func (inv *Invoker) InvokeWithHeaders(ctx context.Context, toolName string, args map[string]any, overlay map[string]string) (InvokeResult, error) {
	return inv.invoke(ctx, toolName, args, overlay)
}
```

`invoke` 合并：先 `inv.Headers`，再 `overlay`（同 key 覆盖）。httptest 断言请求头为 `CAPTURED`。

- [ ] **步骤 3：实现 loader security 解析**

从 `doc.Components.SecuritySchemes` 与 `op.Security` / `doc.Security` 提取 scheme 名列表写入 `ToolRoute.Security []string`。

- [ ] **步骤 4：测试通过并 commit**

```bash
go test ./internal/connector/openapi/ -count=1
git add internal/connector/openapi/
git commit -m "feat(openapi): operation security 与 per-call Headers"
```

---

### 任务 5：RegisterWithOpts 接入 Store/Resolver/Capture

**文件：**
- 修改：`internal/connector/openapi/register_opts.go`
- 修改：`internal/connector/openapi/register.go`
- 修改：`internal/connector/openapi/register_test.go`

- [ ] **步骤 1：编写集成式单测（httptest）**

注册带 login + getMe 的 mini spec；`Identities: mem`；`Resolver: OpenAPISecurityResolver{}`；`Capture: *login*`。

1. 无会话 ctx：getMe 带 RegisterOpts.Headers（env）  
2. `ctx = WithConversationID`；先调 login 工具（mock 返回 accessToken）；再调 getMe → 请求头为捕获 token  
3. `ListPublic` 含一条且无明文 token

- [ ] **步骤 2：扩展 RegisterOpts**

```go
type RegisterOpts struct {
	// ...existing...
	Headers     map[string]string
	Identities  identity.Store          // 可为 nil → 行为与现网一致
	Resolver    authresolve.Resolver    // nil → 只用 Headers
	Capture     identity.CaptureConfig  // ToolNameGlob 空则关闭捕获
}
```

工具闭包伪代码：

```go
func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
	overlay := opts.Headers
	var usedID string
	if opts.Identities != nil && opts.Resolver != nil {
		conv := identity.ConversationIDFrom(ctx)
		force := identity.ForceIdentityIDFrom(ctx)
		in := authresolve.ResolveInput{
			Identities:      opts.Identities.List(conv),
			SecuritySchemes: route.Security,
			DefaultHeaders:  opts.Headers,
			ForceIdentityID: force,
		}
		res := opts.Resolver.Resolve(ctx, in)
		if res.OK {
			overlay = res.Headers
			usedID = res.IdentityID
		}
	}
	out, err := inv.InvokeWithHeaders(ctx, name, args, overlay)
	if err != nil {
		return nil, true, err
	}
	if usedID != "" && opts.Identities != nil {
		_ = opts.Identities.Touch(identity.ConversationIDFrom(ctx), usedID)
	}
	if !out.IsError && opts.Identities != nil && identity.MatchToolName(opts.Capture.ToolNameGlob, name) {
		if h, label, sub, claims, ok := identity.ExtractCredential(opts.Capture, out.Content); ok {
			_, _ = opts.Identities.Upsert(identity.ConversationIDFrom(ctx), identity.Identity{
				Label: label, Scheme: opts.Capture.DefaultScheme, Subject: sub,
				CredentialHeaders: h, Source: identity.SourceLoginCapture,
				ClaimsSummary: claims, IsDefault: true,
			})
		}
	}
	return out.Content, out.IsError, nil
}
```

注意：`conversation_id` 为空时跳过 Upsert（避免污染空 key）；Resolve 仍可用 DefaultHeaders。

- [ ] **步骤 3：测试通过并 commit**

```bash
go test ./internal/connector/openapi/ -count=1
git add internal/connector/openapi/
git commit -m "feat(openapi): 注册路径接入身份解析与登录捕获"
```

---

### 任务 6：Run 持久化 conversation_id / identity_id

**文件：**
- 修改：`internal/store/store.go`
- 修改：`internal/store/memory.go`、`sqlite.go`、对应 `*_test.go`

- [ ] **步骤 1：失败测试**

```go
r, err := s.CreateRun(store.CreateRunInput{
	AgentID: "a", Input: "hi", ConversationID: "conv_1", IdentityID: "idt_1",
})
got, _ := s.GetRun(r.ID)
if got.ConversationID != "conv_1" || got.IdentityID != "idt_1" {
	t.Fatalf("%+v", got)
}
```

- [ ] **步骤 2：改接口**

```go
type Run struct {
	// existing fields...
	ConversationID string `json:"conversation_id,omitempty"`
	IdentityID     string `json:"identity_id,omitempty"`
}

type CreateRunInput struct {
	AgentID        string
	Input          string
	ConversationID string
	IdentityID     string
}

CreateRun(in CreateRunInput) (*Run, error)
```

SQLite：`ALTER` 不方便时对新建库改 `CREATE TABLE`；若已有库，启动时 `ALTER TABLE runs ADD COLUMN conversation_id TEXT` / `identity_id TEXT`（忽略 duplicate column 错误）。更新所有 `CreateRun("x","y")` 调用点为 `CreateRunInput{...}`。

- [ ] **步骤 3：全仓编译测试相关包并 commit**

```bash
go test ./internal/store/ ./internal/run/ ./internal/api/ ./tests/integration/ -count=1
git add internal/store/ internal/run/ internal/api/ tests/
git commit -m "feat(store): Run 持久化 conversation_id 与 identity_id"
```

（本任务允许为编译通过先改调用点，Engine/API 语义在任务 7–8 补全。）

---

### 任务 7：Engine 注入 auth ctx

**文件：**
- 修改：`internal/run/engine.go`
- 修改：`internal/run/engine_test.go`

- [ ] **步骤 1：失败测试**

注册一个 tool，记录 Invoke 时 `identity.ConversationIDFrom(ctx)`。`CreateRun` 带 `ConversationID: "c1"` 后 `Execute`；断言 tool 见到 `"c1"`。HITL 冷恢复路径同样注入（从 `GetRun` 读 ConversationID）。

- [ ] **步骤 2：实现**

在 `Execute` / `ContinueFromHITL` 开头：

```go
runRec, _ := e.Store.GetRun(runID)
ctx = identity.WithConversationID(ctx, runRec.ConversationID)
if runRec.IdentityID != "" {
	ctx = identity.WithForceIdentityID(ctx, runRec.IdentityID)
}
```

`runLoop` 与 HITL approve 后的 `Tools.Invoke` 都使用该 ctx。

- [ ] **步骤 3：测试通过并 commit**

```bash
go test ./internal/run/ -count=1
git add internal/run/
git commit -m "feat(run): 将 conversation 身份上下文注入 tool Invoke"
```

---

### 任务 8：API — runs 字段 + identities 端点

**文件：**
- 修改：`internal/api/server.go`
- 修改：`internal/api/server_test.go`
- 修改：`NewServer` 或 Server 字段增加 `Identities identity.Store`

- [ ] **步骤 1：失败测试**

- `POST /v0/runs` body 含 `conversation_id`；响应含同一 id；省略时服务端生成并返回  
- `GET /v0/conversations/{id}/identities` 返回数组（可先空）  
- 手动 `Upsert` 后 GET 无 token；`POST .../default`；`DELETE` 单条与清空  

- [ ] **步骤 2：实现 handlers**

```text
GET    /v0/conversations/{id}/identities
POST   /v0/conversations/{id}/identities/{iid}/default
DELETE /v0/conversations/{id}/identities/{iid}
DELETE /v0/conversations/{id}/identities
```

`handlePostRun`：

```go
conv := body.ConversationID
if conv == "" {
	conv = "conv_" + uuid
}
runRec, err := s.Store.CreateRun(store.CreateRunInput{
	AgentID: body.AgentID, Input: body.Input,
	ConversationID: conv, IdentityID: body.IdentityID,
})
// Execute with context.Background() — Engine 会从 Run 再注入
writeJSON(..., map[string]any{"run_id":..., "status":..., "conversation_id": conv})
```

- [ ] **步骤 3：测试通过并 commit**

```bash
go test ./internal/api/ -count=1
git add internal/api/
git commit -m "feat(api): conversation 身份 API 与 runs 会话字段"
```

---

### 任务 9：Config / Demo 接线

**文件：**
- 修改：`internal/config/config.go`
- 修改：`internal/demo/demo.go`
- 修改：`configs/demo.yaml`（capture 示例注释或默认 `*login*`）
- 修改：`.env.example` 如需说明

- [ ] **步骤 1：扩展 Auth 配置**

```yaml
auth:
  bearer_env: BAIZE_CONNECTOR_TOKEN
  capture:
    tool_name_glob: "*login*"
    token_json_paths: ["accessToken", "data.accessToken", "data.token"]
    label_json_paths: ["email", "data.email"]
    header_template: "Bearer {{token}}"
    default_scheme: ""   # 空则注册时若 spec 仅一个 http bearer scheme 则填入
```

- [ ] **步骤 2：demo 启动**

`identity.NewMemoryStore()` 挂到 `api.Server`；`RegisterWithOpts` 传入 Identities、Resolver、Capture；若 `default_scheme` 空，从 `LoadTools` 结果统计唯一 security scheme 名填入。

- [ ] **步骤 3：`go test ./...` 相关包通过后 commit**

```bash
git add internal/config/ internal/demo/ configs/ .env.example
git commit -m "feat(demo): 启动时接入会话身份与登录捕获配置"
```

---

### 任务 10：Chat UI — 会话与账号面板

**文件：**
- 修改：`web/chat/src/api.ts`
- 修改：`web/chat/src/main.ts`
- 修改：`web/chat/src/style.css`
- 重建：`internal/ui/dist/**`

- [ ] **步骤 1：api.ts**

```ts
export async function createRun(agentId: string, input: string, conversationId: string, identityId?: string)
export async function listIdentities(conversationId: string): Promise<IdentityView[]>
export async function setDefaultIdentity(conversationId: string, id: string)
export async function deleteIdentity(conversationId: string, id: string)
export async function clearIdentities(conversationId: string)
```

`CreateRunResponse` 增加 `conversation_id?: string`。

- [ ] **步骤 2：main.ts**

- `localStorage` key `baize.conversation_id`；无则 `crypto.randomUUID()` 前缀 `conv_`  
- 「新对话」：新 id、resetChat、刷新身份面板  
- 发送带 `conversation_id`  
- 侧栏「账号」：list、默认标记、退出按钮；脱敏展示 label/scheme/source  
- `tool.result` 渲染时若 JSON 含 `accessToken` 等键，显示为 `[redacted]`

- [ ] **步骤 3：build 并 commit**

```bash
cd web/chat && npm run build
git add web/chat/ internal/ui/dist/
git commit -m "feat(ui): 会话 conversation 与已登录账号面板"
```

---

### 任务 11：集成测试 + README

**文件：**
- 创建：`tests/integration/session_auth_test.go`
- 修改：`README.md`

- [ ] **步骤 1：集成测试**

冷启动最小 Server（memory store + mock LLM 可选：直接 Registry.Invoke 路径也可）：更稳妥是 httptest 上游 API + RegisterWithOpts + 两次带同一 conversation 的 tool 调用（若全链路 Run 过重，本任务允许「connector 级集成」+ 「API identities 脱敏」两条）。

必覆盖规格 §10：

1. 登录捕获后同 conversation 第二次调用带新 Bearer  
2. list identities 响应无 JWT 原文  
3. DELETE 后回退 env header  
4. `identity_id` 强制选用  

- [ ] **步骤 2：README 短节「会话身份」**

说明：`conversation_id`、登录自动记住、账号面板、env 仅兜底。

- [ ] **步骤 3：全量测试与 commit**

```bash
go test ./... -count=1
git add tests/integration/README.md
git commit -m "test+docs(auth): 会话身份集成验证与说明"
```

- [ ] **步骤 4：规格状态**

将规格文首状态改为「已批准并已有实现计划」。

```bash
git add docs/superpowers/specs/2026-08-12-session-auth-identities-design.md
git commit -m "docs(auth): 标记会话身份规格已批准"
```

---

## 规格覆盖自检

| 规格章节 | 任务 |
|----------|------|
| §1 成功标准 1–5 | 5、7、8、10、11 |
| §4 Identity / Capture | 1、2、9 |
| §5 Resolver | 3、5 |
| §6 Run/HITL/Invoke | 4、5、6、7 |
| §7 API | 8 |
| §8 UI | 10 |
| §9 安全脱敏 | 1、8、10、11 |
| §10 测试 | 各任务单测 + 11 |
| §11 兼容 env / 缺 conversation_id | 5、8、9 |

**占位符：** 无 TODO/待定。  
**类型名锁定：** `identity.Store`、`identity.CaptureConfig`、`authresolve.Resolver`、`InvokeWithHeaders`、`store.CreateRunInput`。
