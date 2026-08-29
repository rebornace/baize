# 微信 Channel（iLink）与会话归属 v0 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 具名操作员 + 会话 `owner_id` 隔离 `/ui` 列表；Channel 插件注册表 + `weixin`（iLink 扫码、长轮询、文本/媒体、与 `/ui` 权限内双向同步）。

**Architecture:** 先扩展 `controlplane` 与会话元数据表并过滤 API；再增加 `internal/channel` 注册表与 `weixin` 插件（`ilink.Client` 接口 + 假实现单测）；bootstrap 启动恢复轮询；设置页扫码与聊天页归属/出站挂钩。

**Tech Stack:** Go 1.25、既有 Gate/会话/附件、`database/sql`、React 设置页；iLink HTTP JSON（实现时以腾讯公开 iLink / OpenClaw 插件契约为准，测试只依赖接口）。

**规格：** `docs/superpowers/specs/2026-08-29-channel-weixin-v0-design.md`  
**Git：** 分支 `feat/channel-weixin-v0`（从最新 `main`）

## Global Constraints

- 中文 Conventional Commits；PowerShell here-string 提交，不用 bash heredoc。
- Gate 全空 = 开发态（`owner_id=local-dev`，等同 admin UI），文档必须写明无多运营隔离。
- 同 peer 有 active Run → 不建第二 Run，微信回复固定文案「请稍候，上一轮还在处理」。
- 群聊默认关闭；不做公众号/企微/SSO/CLI 扫码。
- 单实例；凭证目录默认 `./data/channels/weixin/`，gitignore。
- 假 iLink 单测必绿；真机扫码为手工验收。

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/config/config.go` | `operators[]`、渠道/微信相关配置字段 |
| `internal/controlplane/auth.go` | 多 operator token → `(Role, OperatorID)` |
| `internal/controlplane/principal.go` | `Principal{Role, OperatorID}`；`WithPrincipal` / `PrincipalFrom` |
| `internal/conversation/meta.go` | 会话元数据模型 + Store 方法 |
| `internal/conversation/sqlite_meta.go` / PG 路径 | `conversations` 表 |
| `internal/api/server.go` 等 | `/v0/me` 带 `operator_id`；会话列表过滤；消息/Run 鉴权 |
| `internal/channel/registry.go` | `RegisterChannel` / `Open` / `List` |
| `internal/channel/channel.go` | `Channel` 接口：`Start`/`Stop`/`SendText`/`SendMedia` |
| `internal/channel/runtime.go` | 入站：解析 peer → meta → CreateRun；出站钩子 |
| `internal/channel/weixin/` | iLink 客户端、登录、轮询、媒体、凭证落盘 |
| `internal/channel/weixin/fake_ilink_test.go` | 假 HTTP |
| `internal/api/server_channel_weixin.go` | 登录/状态/登出/设置 API |
| `web/chat/src/pages/WeixinChannelSettings.tsx` | 扫码与设置 |
| `web/chat/src/pages/ChatPage.tsx` | scope、标题、出站绑定 |
| `README.zh-CN.md` / `README.md` | 文档 |
| `.gitignore` | `data/channels/` |

---

### 任务 0：分支

- [ ] **步骤 1**

```powershell
cd C:\Users\Administrator\Desktop\baize
git checkout main
git pull real main
git checkout -b feat/channel-weixin-v0
```

---

### 任务 1：具名操作员认证（TDD）

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/controlplane/auth.go`
- Create: `internal/controlplane/principal.go`
- Create: `internal/controlplane/auth_operators_test.go`
- Modify: Gate 中间件，把 `OperatorID` 写入 context

**Interfaces:**
- Produces: `type Operator struct { ID, Token string }`；`Authenticate(header, Tokens) (Principal, bool)`；`type Principal struct { Role Role; OperatorID string }`；`Tokens` 增加 `Operators []Operator`；兼容旧 `Operator` 单字段 → `id=operator`

- [ ] **步骤 1：失败测试**

```go
func TestAuthenticateNamedOperators(t *testing.T) {
	tok := Tokens{
		Admin: "adm",
		Operators: []Operator{{ID: "alice", Token: "ta"}, {ID: "bob", Token: "tb"}},
	}
	p, ok := AuthenticatePrincipal("Bearer ta", tok)
	if !ok || p.Role != RoleOperator || p.OperatorID != "alice" {
		t.Fatalf("%+v ok=%v", p, ok)
	}
	p, ok = AuthenticatePrincipal("Bearer adm", tok)
	if !ok || p.Role != RoleAdmin {
		t.Fatalf("%+v", p)
	}
}

func TestAuthenticateLegacyOperatorToken(t *testing.T) {
	tok := Tokens{Operator: "op", Admin: "adm"}
	p, ok := AuthenticatePrincipal("Bearer op", tok)
	if !ok || p.OperatorID != "operator" {
		t.Fatalf("%+v", p)
	}
}
```

- [ ] **步骤 2：实现** — `Enabled()`：admin 非空或任一 operator token 非空；YAML `operators` 解析进 config，bootstrap 填入 `Tokens`。

- [ ] **步骤 3：`GET /v0/me`** 返回 `{"role":"...","operator_id":"...","gate_enabled":true}`（字段名与前端对齐）。

- [ ] **步骤 4：提交**

```powershell
go test ./internal/controlplane/ ./internal/config/ -count=1
git add internal/controlplane internal/config internal/api
git commit -m @"
feat(controlplane): 具名操作员 token 与 Principal
"@
```

---

### 任务 2：会话元数据与列表过滤（TDD）

**Files:**
- Create: `internal/conversation/meta.go`
- Modify: `internal/conversation/sqlite.go`（及 postgres/OpenSQL 路径若共用）
- Modify: `internal/api/server.go` — `handleListConversations`、消息读写、创建会话
- Test: `internal/conversation/meta_test.go`、`internal/api/server_conversation_owner_test.go`

**Interfaces:**
- Produces: `type Meta struct { ID, OwnerID, Source, Title string; UpdatedAt time.Time; ChannelPeer string }`；`EnsureMeta` / `ListMeta(filter)` / `GetMeta` / `CanAccess(principal, meta) bool`

- [ ] **步骤 1：表 DDL**

```sql
CREATE TABLE IF NOT EXISTS conversation_meta (
  id TEXT PRIMARY KEY,
  owner_id TEXT NOT NULL,
  source TEXT NOT NULL,
  title TEXT,
  channel_peer TEXT,
  updated_at TEXT NOT NULL
);
```

- [ ] **步骤 2：测试** — alice 只看到自己的；admin `scope=all` 看到全部；bob 读 alice 会话消息 → 403。

- [ ] **步骤 3：创建会话** — UI 新建时 `EnsureMeta(id, owner=principal.OperatorID或local-dev, source=ui)`。Gate 关：`owner_id=local-dev`，列表不过滤（或仅 local-dev，与「开发态等同 admin 看全」一致：**Gate 关时不过滤**）。

- [ ] **步骤 4：提交**

```powershell
go test ./internal/conversation/ ./internal/api/ -count=1
git add internal/conversation internal/api
git commit -m @"
feat(conversation): 会话 owner 元数据与列表隔离
"@
```

---

### 任务 3：Channel 注册表与 Runtime 钩子

**Files:**
- Create: `internal/channel/registry.go`、`channel.go`、`runtime.go`、`runtime_test.go`

**Interfaces:**
- Produces:

```go
type Channel interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    SendText(ctx context.Context, peerID, text string, extras map[string]string) error
    SendMedia(ctx context.Context, peerID string, filename string, mime string, data []byte, extras map[string]string) error
}

type Inbound struct {
    PeerID string
    Text string
    Files []InboundFile // name, mime, bytes
    Extras map[string]string // context_token 等
}

type Runtime struct { /* Store, Messages, Engine start CreateRun, Meta, Assignee */ }
func (r *Runtime) HandleInbound(ctx context.Context, ch Channel, in Inbound) error
```

- [ ] **步骤 1：单测假 Channel** — 入站建 meta（owner=assignee）、无 active Run 时 CreateRun 被调用；有 active Run 时调用 `SendText` 忙碌文案且不 CreateRun。

- [ ] **步骤 2：实现 `HandleInbound`** — conversation id = `weixin:` + account + `:` + peer（account 由 channel 注入 extras）；附件写入与现 Chat 附件相同存储约定（复用 `internal/attach` / run 入参路径，参照 `server` 发消息带 attachments）。

- [ ] **步骤 3：提交**

```powershell
go test ./internal/channel/ -count=1
git add internal/channel
git commit -m @"
feat(channel): 注册表与入站 Runtime 钩子
"@
```

---

### 任务 4：weixin iLink 客户端（接口 + Fake）

**Files:**
- Create: `internal/channel/weixin/ilink.go`（接口）
- Create: `internal/channel/weixin/client.go`（真 HTTP，base URL 可配置）
- Create: `internal/channel/weixin/fake.go`（测试）
- Create: `internal/channel/weixin/client_test.go`

**Interfaces:**
- Produces: `type ILink interface { GetQR(ctx) (ticket, qrURL string, err error); PollLogin(ctx, ticket) (status, accountID, token string, err error); GetUpdates(ctx, token, cursor) ([]Update, nextCursor, err error); SendMessage(...); DownloadMedia(...) }`

- [ ] **步骤 1：** 用 Fake 写登录：`start→pending→success` 与 `GetUpdates` 返回一条文本。

- [ ] **步骤 2：** 真客户端按腾讯 iLink / 社区已公开路径实现（常量集中；超时与 hold 轮询）；错误包装清晰。

- [ ] **步骤 3：提交**

```powershell
go test ./internal/channel/weixin/ -count=1
git add internal/channel/weixin
git commit -m @"
feat(weixin): iLink 客户端接口与 Fake
"@
```

---

### 任务 5：weixin Channel 插件（登录落盘、轮询、媒体）

**Files:**
- Create: `internal/channel/weixin/channel.go`、`creds.go`、`channel_test.go`
- Modify: `cmd/baize` / bootstrap 空白注册

- [ ] **步骤 1：凭证** — `SaveCreds(dir, accountID, token)` / `LoadCreds`；目录权限 0700。

- [ ] **步骤 2：`Channel` 实现** — `Start` 循环 `GetUpdates` → `Runtime.HandleInbound`；媒体下载后填 `Inbound.Files`；出站 `SendText`/`SendMedia`。

- [ ] **步骤 3：测试** — Fake 推送两条不同 peer → 两个 meta；busy 路径；图片字节进入 Inbound.Files。

- [ ] **步骤 4：提交**

```powershell
go test ./internal/channel/... -count=1
git add internal/channel cmd/baize internal/bootstrap
git commit -m @"
feat(weixin): Channel 插件轮询与凭证落盘
"@
```

---

### 任务 6：API 设置页后端

**Files:**
- Create: `internal/api/server_channel_weixin.go`、`server_channel_weixin_test.go`
- Modify: `internal/controlplane/acl.go`（admin 路由）
- Modify: bootstrap 注入 Channel/Runtime

**API：**

| 方法 | 路径 |
|------|------|
| POST | `/v0/settings/channels/weixin/login/start` |
| GET | `/v0/settings/channels/weixin/login/status?ticket=` |
| POST | `/v0/settings/channels/weixin/logout` |
| GET/PUT | `/v0/settings/channels/weixin`（agent_id、allowlist、assignee、enabled） |

- [ ] **步骤 1：测试** — 非 admin 403；start 返回 qr；fake 登录 success 后 status=success。

- [ ] **步骤 2：实现并提交**

```powershell
go test ./internal/api/ -count=1
git add internal/api internal/bootstrap internal/controlplane
git commit -m @"
feat(api): 微信渠道登录与设置端点
"@
```

---

### 任务 7：UI — 渠道设置 + 会话 scope

**Files:**
- Create: `web/chat/src/pages/WeixinChannelSettings.tsx`
- Modify: `settingsNav.ts`、`main.tsx`、`api.ts`、`ChatPage.tsx`、`UnlockPage`/`getMe` 解析 `operator_id`
- Rebuild: `npm run build` → `internal/ui/dist`

- [ ] **步骤 1：** 导航增加「渠道」；页内二维码（img 或 URL）、状态轮询、登出、assignee/allowlist 表单。

- [ ] **步骤 2：** 聊天列表：admin 可切换「全部 / 我的」；标题显示渠道前缀；`operator_id` 存 context。

- [ ] **步骤 3：**

```powershell
cd web/chat; npm test -- --run; npx tsc --noEmit; npm run build
```

- [ ] **步骤 4：提交（含 dist）**

```powershell
git add web/chat internal/ui/dist
git commit -m @"
feat(ui): 微信渠道设置与会话归属展示
"@
```

---

### 任务 8：`/ui` → 微信出站（双向同步）

**Files:**
- Modify: run 完成 / 消息追加路径（API `POST` 带 conversation 发消息处或 engine 结束钩子）
- Test: `internal/channel/outbound_test.go`

- [ ] **步骤 1：** 若 `meta.Source=="weixin"` 且 `ChannelPeer` 非空，助手最终文本（及附件）调用 `weixin.Send*`。  
- [ ] **步骤 2：** 无 Channel 或未登录则跳过（UI 仍保存消息）。  
- [ ] **步骤 3：提交**

```powershell
go test ./internal/channel/ ./internal/api/ -count=1
git add internal/
git commit -m @"
feat(channel): UI 会话回复出站到微信 peer
"@
```

---

### 任务 9：文档与 gitignore

**Files:**
- Modify: `README.zh-CN.md`、`README.md`、`docs/architecture-and-plugin-protocol.md`
- Modify: `.gitignore` — `/data/channels/`
- Modify: `docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md` 状态 → 实现中/已交付
- Modify: `configs/demo.yaml` 注释示例 `operators`

文档要点：具名操作员配置、开发态无隔离、微信设置页步骤、与 Inbox 分工、群限制、受理人、媒体。

- [ ] **步骤 1：提交**

```powershell
git add README.md README.zh-CN.md docs .gitignore configs
git commit -m @"
docs: 微信 Channel 与具名操作员说明
"@
```

---

### 任务 10：全量验证

- [ ] `go test ./... -count=1`
- [ ] `web/chat` test + tsc
- [ ] 手工清单（规格 §9 A–G）记在 PR 描述
- [ ] finishing：合并推 `real`；`export-public.ps1` 同步 public（无 superpowers）

---

## Self-Review（对照规格）

| 规格 | 任务 |
|------|------|
| 具名操作员 /me | 1 |
| owner 列表/403 | 2 |
| Channel 注册表 + 入站 | 3 |
| iLink + Fake | 4 |
| 轮询/凭证/媒体 | 5 |
| 设置 API | 6 |
| 设置页 UI + dist | 7 |
| 双向同步出站 | 8 |
| 文档 Gate 开发态 | 9 |
| 非目标未纳入 | Global |

无 TBD；忙碌文案固定为规格用词。
