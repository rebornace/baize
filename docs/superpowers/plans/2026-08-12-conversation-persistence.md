# 对话上下文与身份持久化 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为同一 `conversation_id` 持久化多轮 Message（供 LLM 窗口注入），并将 IdentityStore 落到 SQLite，使重启后仍保持登录与对话记忆。

**架构：** 新建 Message Store（memory + sqlite）；Identity 增加 SQLite 实现并在 bootstrap 按配置装配；`POST /v0/runs` 写 user message，Run 终态写 assistant/失败短注；`Engine.Execute` 组装 `system + window(history) + user`；UI 启动拉历史，可选清空聊天。

**技术栈：** Go 1.22+、现有 SQLite store（modernc）、Vite + TypeScript（`web/chat`）

**规格：** `docs/superpowers/specs/2026-08-12-conversation-persistence-design.md`

---

## 文件结构（将创建/修改）

| 路径 | 职责 |
|------|------|
| `internal/conversation/message.go` | Message 模型与 role 常量 |
| `internal/conversation/store.go` | MessageStore 接口 + Memory 实现 |
| `internal/conversation/store_test.go` | 追加/列表/Clear/窗口裁剪单测 |
| `internal/conversation/sqlite.go` | Message SQLite 实现（可复用 `*sql.DB`） |
| `internal/conversation/sqlite_test.go` | 落盘与重启后仍可读 |
| `internal/identity/sqlite.go` | Identity Store 的 SQLite 实现（实现现有 `identity.Store`） |
| `internal/identity/sqlite_test.go` | Upsert/List/重启后 Resolve 用数据仍在 |
| `internal/store/sqlite.go` | 导出 DB 访问或提供 `OpenSQLiteDB` 共享连接；迁移建表钩子 |
| `internal/config/config.go` | `conversation.max_messages` / `persist_identities` |
| `configs/default.yaml` | 写入 conversation 段默认值 |
| `internal/run/engine.go` | 注入 MessageStore + MaxMessages；Execute 组装历史 |
| `internal/run/engine_test.go` | script LLM 断言第二轮含第一轮内容 |
| `internal/api/server.go` | messages GET/DELETE；PostRun 写 user；终态写 assistant |
| `internal/api/server_test.go` | messages API 与顺序 |
| `internal/bootstrap/bootstrap.go` | 装配 MessageStore + Identity（sqlite/memory） |
| `web/chat/src/api.ts` | listMessages / clearMessages |
| `web/chat/src/main.ts` | 启动加载历史；清空聊天按钮 |
| `web/chat/src/style.css` | 按钮样式（最小） |
| `internal/ui/dist/**` | `npm run build` 后嵌入 |
| `tests/integration/conversation_persist_test.go` | 两轮注入 + 身份重启（或 sqlite reopen） |
| `README.md` / `README.zh-CN.md` | 短节：对话记忆与 db 含凭证说明 |

---

### 任务 1：Message 模型与内存 Store

**文件：**
- 创建：`internal/conversation/message.go`
- 创建：`internal/conversation/store.go`
- 创建：`internal/conversation/store_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestMemoryStoreAppendListClearWindow(t *testing.T) {
	s := conversation.NewMemoryStore()
	_, err := s.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "你好"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "你好！", RunID: "run_1"})
	if err != nil {
		t.Fatal(err)
	}
	all := s.List("conv1")
	if len(all) != 2 || all[0].Role != conversation.RoleUser || all[1].Content != "你好！" {
		t.Fatalf("%+v", all)
	}
	win := s.ListWindow("conv1", 1)
	if len(win) != 1 || win[0].Role != conversation.RoleAssistant {
		t.Fatalf("window=%+v", win)
	}
	s.Clear("conv1")
	if len(s.List("conv1")) != 0 {
		t.Fatal("expected empty after clear")
	}
}
```

- [ ] **步骤 2：运行确认失败**

```powershell
go test ./internal/conversation/ -count=1
```

预期：包不存在或 `NewMemoryStore` 未定义。

- [ ] **步骤 3：最少实现**

`message.go`：

```go
package conversation

import "time"

const (
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleSystemNote = "system_note"
)

type Message struct {
	ID             string
	ConversationID string
	Role           string
	Content        string
	RunID          string
	CreatedAt      time.Time
}
```

`store.go`：接口 `Append`（生成 `msg_`+uuid，填 CreatedAt）、`List`（正序）、`ListWindow(conv, n)`（n<=0 视为全部；否则最近 n 条仍正序返回）、`Clear`；`MemoryStore` 用 `sync.RWMutex` + `map[string][]Message`。

- [ ] **步骤 4：测试通过并 Commit**

```powershell
go test ./internal/conversation/ -count=1
git add internal/conversation/
git commit -m "feat(conversation): Message 内存 Store 与窗口列表"
```

---

### 任务 2：Message SQLite 实现

**文件：**
- 修改：`internal/store/sqlite.go` — 增加共享打开路径或 `DB() *sql.DB`（若不宜导出，则在 `conversation` 包接受 `*sql.DB` + 自建表）
- 创建：`internal/conversation/sqlite.go`
- 创建：`internal/conversation/sqlite_test.go`

推荐：`conversation.OpenSQLite(db *sql.DB) (*SQLiteStore, error)` 在传入的 db 上 `CREATE TABLE IF NOT EXISTS messages (...)`，不第二套连接。

- [ ] **步骤 1：失败测试**

```go
func TestSQLiteMessageRoundTrip(t *testing.T) {
	db := openTestDB(t) // 用 store.OpenSQLite 临时路径后取 db，或 sql.Open sqlite :memory: / temp file
	s, err := conversation.OpenSQLite(db)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: "ping"})
	if err != nil || id == "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	// close & reopen same file
	s2, err := conversation.OpenSQLite(reopen(t))
	got := s2.List("c1")
	if len(got) != 1 || got[0].Content != "ping" {
		t.Fatalf("%+v", got)
	}
}
```

- [ ] **步骤 2：实现表结构**

```sql
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  run_id TEXT,
  created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_conv_created ON messages(conversation_id, created_at);
```

- [ ] **步骤 3：测试通过并 Commit**

```powershell
go test ./internal/conversation/ -count=1
git add internal/conversation/ internal/store/sqlite.go
git commit -m "feat(conversation): Message SQLite 持久化"
```

---

### 任务 3：Identity SQLite Store

**文件：**
- 创建：`internal/identity/sqlite.go`
- 创建：`internal/identity/sqlite_test.go`
- 修改：必要时把 MemoryStore 的互斥默认逻辑抽成可复用的私有 helper（避免两套行为漂移）；**行为必须与 MemoryStore 一致**（scheme+subject 合并、IsDefault 互斥等）

- [ ] **步骤 1：失败测试**

复用 `store_test.go` 中的 Upsert/ListPublic/Delete/SetDefault 场景，改为：

```go
func TestSQLiteStorePersistsAcrossOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id.db")
	db1 := openSQL(t, path)
	s1, err := identity.OpenSQLite(db1)
	id, err := s1.Upsert("conv", identity.Identity{
		Label: "a@x.com", Scheme: "bearer",
		CredentialHeaders: map[string]string{"Authorization": "Bearer SECRET"},
		Source: identity.SourceLoginCapture, Subject: "a@x.com", IsDefault: true,
	})
	_ = db1.Close()
	db2 := openSQL(t, path)
	s2, _ := identity.OpenSQLite(db2)
	got, err := s2.Get("conv", id)
	if err != nil || got.CredentialHeaders["Authorization"] != "Bearer SECRET" {
		t.Fatalf("%+v err=%v", got, err)
	}
}
```

- [ ] **步骤 2：实现**

表 `identities`：`id, conversation_id, label, scheme, subject, source, is_default, headers_json, claims_json, last_used_at, created_at, updated_at`。  
`OpenSQLite(db)` 建表；方法签名实现 `identity.Store`。

- [ ] **步骤 3：跑现有 identity 单测 + 新测并 Commit**

```powershell
go test ./internal/identity/ -count=1
git add internal/identity/
git commit -m "feat(identity): SQLite 持久化 IdentityStore"
```

---

### 任务 4：配置与 bootstrap 装配

**文件：**
- 修改：`internal/config/config.go`
- 修改：`configs/default.yaml`
- 修改：`internal/bootstrap/bootstrap.go`
- 修改：`internal/api/server.go`（Server 增加 `Messages conversation.Store` 字段，可先占位）

- [ ] **步骤 1：扩展 Config**

```go
Conversation struct {
	MaxMessages       int  `yaml:"max_messages"`
	PersistIdentities bool `yaml:"persist_identities"`
} `yaml:"conversation"`
```

`Load` 默认：`MaxMessages` 若 `<=0` → `40`；若 yaml 未出现 `persist_identities`，在 sqlite 驱动下默认 `true`（可用指针区分「未设置」；若不想用指针，文档约定缺省 true，Load 里当 Store.Driver==sqlite 且未显式 false——最简单：默认值结构体里 `PersistIdentities` 用 `*bool`，或 Load 后若 driver=sqlite 则默认 true，配置显式 `false` 关闭）。

推荐实现：

```go
if cfg.Conversation.MaxMessages <= 0 {
	cfg.Conversation.MaxMessages = 40
}
// PersistIdentities：yaml 缺省时，sqlite → true；memory → false
// 用独立 bool + 在 Load 后根据 driver 设置：若 Unmarshal 后为 false 且文件未写该键会误判。
// 实用做法：默认 true；memory 驱动强制 false；配置 persist_identities: false 可关。
```

锁定：**`persist_identities` 默认 true；`store.driver=memory` 时忽略并使用 MemoryStore。**

- [ ] **步骤 2：bootstrap**

在 `newAPIServer`：

1. 打开 store 后，若 sqlite：从同一 `*sql.DB` 打开 `conversation.OpenSQLite` +（若 persist）`identity.OpenSQLite`；否则两者 Memory。  
2. 需要 `store.SQLite` 暴露 `DB() *sql.DB` 或 bootstrap 改为先 `sql.Open` 再注入——**优先给 `*store.SQLite` 加 `DB() *sql.DB`**。  
3. `srv.Identities = ...`；`srv.Messages = ...`；`engine` 注入 Messages + MaxMessages。

- [ ] **步骤 3：default.yaml 增加**

```yaml
conversation:
  max_messages: 40
  persist_identities: true
```

- [ ] **步骤 4：测试 Load 默认值 + Commit**

```powershell
go test ./internal/config/ ./internal/bootstrap/ -count=1
git add internal/config/ configs/default.yaml internal/bootstrap/ internal/store/sqlite.go internal/api/server.go
git commit -m "feat(config): conversation 窗口与身份持久化装配"
```

---

### 任务 5：Engine 注入历史窗口

**文件：**
- 修改：`internal/run/engine.go`
- 修改：`internal/run/engine_test.go`

- [ ] **步骤 1：失败测试（script LLM 记录收到的 messages）**

```go
func TestExecuteInjectsConversationHistory(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "sys"})
	msgStore := conversation.NewMemoryStore()
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleUser, Content: "上一轮问题"})
	_, _ = msgStore.Append("conv1", conversation.Message{Role: conversation.RoleAssistant, Content: "上一轮回答"})

	var saw []llm.Message
	llmStub := &captureLLM{onChat: func(msgs []llm.Message, _ []llm.ToolSpec) llm.Message {
		saw = append([]llm.Message(nil), msgs...)
		return llm.Message{Role: llm.RoleAssistant, Content: "本轮回答"}
	}}
	eng := &run.Engine{Store: st, LLM: llmStub, Tools: tool.NewRegistry(), MaxSteps: 4, Messages: msgStore, MaxMessages: 40}
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "本轮问题", ConversationID: "conv1"})
	if err := eng.Execute(context.Background(), r.ID, agent.Def{ID: "a", System: "sys"}, "本轮问题"); err != nil {
		t.Fatal(err)
	}
	// expect: system, 上一轮 user, 上一轮 assistant, 本轮 user
	if len(saw) < 4 || saw[1].Content != "上一轮问题" || saw[3].Content != "本轮问题" {
		t.Fatalf("%+v", saw)
	}
}
```

- [ ] **步骤 2：实现**

`Engine` 增加：

```go
Messages    conversation.Store // 可选；nil 则行为与现在一致
MaxMessages int
```

`Execute`：

```go
messages := []llm.Message{{Role: llm.RoleSystem, Content: ag.System}}
if e.Messages != nil && runRec.ConversationID != "" {
	for _, m := range e.Messages.ListWindow(runRec.ConversationID, e.MaxMessages) {
		switch m.Role {
		case conversation.RoleUser:
			messages = append(messages, llm.Message{Role: llm.RoleUser, Content: m.Content})
		case conversation.RoleAssistant, conversation.RoleSystemNote:
			messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: m.Content})
		}
	}
}
// 若末条已是相同 Content 的 user（API 已先 Append），则不再 append input；否则 append 当前 input
```

注意：`injectAuthCtx` / `GetRun` 需在拼历史前拿到 `ConversationID`（已有）。

- [ ] **步骤 3：测试通过并 Commit**

```powershell
go test ./internal/run/ -count=1
git add internal/run/
git commit -m "feat(run): Execute 注入会话历史窗口"
```

---

### 任务 6：API 写 Message + messages 端点

**文件：**
- 修改：`internal/api/server.go`
- 修改：`internal/api/server_test.go`

- [ ] **步骤 1：路由**

```go
s.mux.HandleFunc("GET /v0/conversations/{id}/messages", s.handleListMessages)
s.mux.HandleFunc("DELETE /v0/conversations/{id}/messages", s.handleClearMessages)
```

- [ ] **步骤 2：PostRun**

在 `CreateRun` 成功后、`go Execute` 前：

```go
if s.Messages != nil && conv != "" {
	_, _ = s.Messages.Append(conv, conversation.Message{Role: conversation.RoleUser, Content: body.Input, RunID: runRec.ID})
}
```

- [ ] **步骤 3：终态写入**

在 `Execute` 返回后的 goroutine 包装（已有失败标 failed 逻辑）中：根据最终 `GetRun`：

- `succeeded` + output → Append assistant  
- `failed` + error → Append `system_note`（`运行失败：`+error）  
- `waiting_human` → **先不写** assistant（resume 完成后再写）

HITL：在 `ContinueFromHITL` / Gate 恢复后的最终成功/失败路径同样写一条；若实现困难，可在 Engine 的 `UpdateRun(succeeded/failed)` 处增加可选 `OnTerminal` 回调，或 API 轮询外由 Engine 直接依赖 Messages——**推荐 Engine 在 `UpdateRun` 到终态时写 Message**（集中、含 HITL），API 只负责创建时的 user message。

锁定本任务：**user 在 API；assistant/失败短注在 Engine**（Engine 已有 Messages）。

调整任务 5/6：Engine 在 `StatusSucceeded` / `StatusFailed` 的 `UpdateRun` 之后 Append。HITL reject → failed 短注。waiting_human 不写。

- [ ] **步骤 4：List/Clear handler**

```go
func (s *Server) handleListMessages(...) {
	if s.Messages == nil { writeJSON(w, 200, []any{}); return }
	writeJSON(w, 200, s.Messages.List(r.PathValue("id")))
}
func (s *Server) handleClearMessages(...) {
	if s.Messages != nil { s.Messages.Clear(r.PathValue("id")) }
	writeJSON(w, 200, map[string]string{"status":"ok"})
}
```

JSON 字段用 snake_case：`id, conversation_id, role, content, run_id, created_at`（给 Message 加 json tag）。

- [ ] **步骤 5：API 测试 + Commit**

```powershell
go test ./internal/api/ ./internal/run/ -count=1
git add internal/api/ internal/run/
git commit -m "feat(api): conversations messages 与终态落库"
```

---

### 任务 7：Chat UI 历史回放与清空

**文件：**
- 修改：`web/chat/src/api.ts`
- 修改：`web/chat/src/main.ts`
- 修改：`web/chat/src/style.css`（如需）
- 重建：`internal/ui/dist/**`

- [ ] **步骤 1：api.ts**

```ts
export interface ChatMessage {
  id: string
  conversation_id: string
  role: 'user' | 'assistant' | 'system_note'
  content: string
  run_id?: string
  created_at: string
}
export async function listMessages(conversationId: string): Promise<ChatMessage[]>
export async function clearMessages(conversationId: string): Promise<void>
```

- [ ] **步骤 2：main.ts**

- 启动 `loadConversationId` 后调用 `loadHistory()`：按 role 填入 `items` 并 `render()`  
- 侧栏增加「清空聊天」按钮 → `clearMessages` → 清空本地 `items`（保留账号面板）  
- 「新对话」仍换 id，并清空 items  

- [ ] **步骤 3：构建嵌入**

```powershell
cd web/chat; npm ci; npm run build
```

- [ ] **步骤 4：Commit**

```powershell
git add web/chat/ internal/ui/dist/
git commit -m "feat(ui): 会话历史回放与清空聊天"
```

---

### 任务 8：集成测试与文档

**文件：**
- 创建：`tests/integration/conversation_persist_test.go`
- 修改：`README.md`、`README.zh-CN.md`

- [ ] **步骤 1：集成测试**

1. memory/sqlite bootstrap 最小栈：两轮 `POST /v0/runs`（mock LLM 回固定文案），第二轮前用 test hook 或检查 MessageStore；若全链路 LLM 难断言，可对 Engine 单测已覆盖注入——集成侧重：  
   - GET messages 在一轮成功后长度 >= 2  
   - DELETE messages 后 identities 仍在（先 login capture 再 clear messages）  
2. Identity：sqlite 文件 Upsert → Close → Open → ListPublic 仍有条目  

- [ ] **步骤 2：README 短节**

说明：对话记忆与身份默认写入 `data/baize.db`；重启可恢复；勿提交 db；`conversation.max_messages`；清空聊天 ≠ 退出登录。

- [ ] **步骤 3：全量测试与 Commit**

```powershell
go test ./... -count=1
git add tests/integration/ README.md README.zh-CN.md
git commit -m "test+docs(conversation): 持久化集成验证与说明"
```

---

## 规格覆盖对照

| 规格章节 | 任务 |
|----------|------|
| §2 Message Store | 1–2 |
| §2 Identity SQLite | 3–4 |
| §5 Engine 窗口 | 5–6 |
| §5 API messages | 6 |
| §6 UI | 7 |
| §7 配置 | 4 |
| §9 迁移建表 | 2–3 |
| §10 测试 | 1–3、5–6、8 |
| §1 成功标准 1–5 | 8 验收 |

---

## 自检

- 无 TBD/占位实现步骤  
- `ListWindow` / `MaxMessages` / `persist_identities` 命名前后一致  
- user 消息 API 写、assistant Engine 写，避免重复 user：Engine 检测末条相同 content 则跳过  
- HITL 等待不写 assistant，终态再写  

---

计划已完成并保存到 `docs/superpowers/plans/2026-08-12-conversation-persistence.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 任务间审查（`subagent-driven-development`）  
2. **内联执行** — 本会话按 `executing-plans` 批量推进并设检查点  

选哪种方式？
