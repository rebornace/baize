# Chat UI 壳、轨迹卡片与 Run SSE 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** `/ui` 成为对话优先壳（左栏历史、工具状态卡片含审批、设置里只读 Tools/账号）；Run 事件走 SSE；前端改为 React + Vite，仍 embed。

**架构：** `conversation.Store.ListSummaries` 供 `GET /v0/conversations`。`internal/eventbus` 包装 `store.Store` 的 `AppendEvent`/`UpdateRun` 做进程内 fan-out。`GET /v0/runs/{id}/stream` 回放 + 增量。`web/chat` 换成 React Router（`basename=/ui`）。bootstrap 在 `*store.SQLite` 类型断言之后再包装 Store，以免打断 sqlite 消息库打开。

**技术栈：** Go 1.22+、net/http SSE、React 18、Vite 6、react-router-dom。无 assistant-ui、无 MUI。

**规格：** `docs/superpowers/specs/2026-08-15-chat-ui-shell-design.md`

**全局约束：**
- 不改 `configs/default.yaml`、Docker、ReAct 引擎选工具逻辑
- 不做 MCP 真连接、Connector 编辑器、LLM token 流、WebSocket
- 清空聊天后该对话从列表消失（消息没了）
- commit 中文：`type(scope): 说明`；PowerShell 不要 HEREDOC
- Go：`C:\Users\Administrator\sdk\go\bin`；`GOPROXY=https://goproxy.cn,direct`
- 实现不要在 `main` 上开始，用功能分支（如 `feat/chat-ui-shell`）

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/conversation/summary.go` | `Summary`、`TruncateTitle`（40 字 rune + `…`） |
| `internal/conversation/store.go` | 接口加 `ListSummaries() []Summary`；Memory 实现 |
| `internal/conversation/sqlite.go` | SQLite `ListSummaries` |
| `internal/conversation/store_test.go` | Memory 列表/截断/Clear |
| `internal/conversation/sqlite_test.go` | SQLite 同样用例 |
| `internal/api/server.go` | `GET /v0/conversations`、`GET /v0/runs/{id}/stream`、`Hub` 字段 |
| `internal/api/server_test.go` | 列表 API、SSE 回放/增量/after/终态 |
| `internal/eventbus/hub.go` | 按 run 订阅事件与终态 |
| `internal/eventbus/store.go` | `Notify(store.Store, *Hub) store.Store` |
| `internal/eventbus/hub_test.go` | 订阅、Publish、UpdateRun 终态 |
| `internal/bootstrap/bootstrap.go` | `newAPIServer` 在 engine 之前包装 Store 并注入 Hub |
| `docs/architecture-and-plugin-protocol.md` | §3 标明 stream 已实现 |
| `web/chat/package.json` | react、react-dom、react-router-dom、vite plugin、vitest |
| `web/chat/vite.config.ts` | `@vitejs/plugin-react`；vitest 若用同一 config |
| `web/chat/tsconfig.json` | `jsx: react-jsx` |
| `web/chat/index.html` | 入口改为 `/src/main.tsx` |
| `web/chat/src/main.tsx` | Router 挂载 |
| `web/chat/src/api.ts` | 保留并加 `listConversations`、`openRunStream` |
| `web/chat/src/foldEvents.ts` | 事件 → 气泡/卡片模型（可单测） |
| `web/chat/src/foldEvents.test.ts` | vitest |
| `web/chat/src/style.css` | ChatGPT 式浅色壳 |
| `web/chat/src/pages/ChatPage.tsx` | 左栏 + 消息 + composer |
| `web/chat/src/pages/SettingsLayout.tsx` | 设置左导航 |
| `web/chat/src/pages/ToolsSettings.tsx` | 只读 Tools |
| `web/chat/src/pages/IdentitiesSettings.tsx` | 账号 |
| `web/chat/src/pages/ComingSoon.tsx` | MCP/插件空状态 |
| `web/chat/src/components/ToolCard.tsx` | 状态条 + 展开 + HITL |
| `web/chat/src/components/Composer.tsx` | 输入；「+」提示去设置 |
| `internal/ui/dist/**` | `npm run build` 产物（须提交以便 embed） |
| 删除 | `web/chat/src/main.ts`（逻辑迁走后） |

---

### 任务 1：ListSummaries

**文件：**
- 创建：`internal/conversation/summary.go`
- 修改：`internal/conversation/store.go`（接口 + Memory）
- 修改：`internal/conversation/sqlite.go`
- 修改：`internal/conversation/store_test.go`、`sqlite_test.go`

- [ ] **步骤 1：写失败测试**（`store_test.go` 追加）

```go
func TestListSummariesTitleTruncateAndClear(t *testing.T) {
	s := conversation.NewMemoryStore()
	long := strings.Repeat("啊", 45)
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleUser, Content: long}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Append("c1", conversation.Message{Role: conversation.RoleAssistant, Content: "ok"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := s.Append("c2", conversation.Message{Role: conversation.RoleUser, Content: "短标题"}); err != nil {
		t.Fatal(err)
	}
	sum := s.ListSummaries()
	if len(sum) != 2 {
		t.Fatalf("len=%d", len(sum))
	}
	if sum[0].ID != "c2" || sum[0].Title != "短标题" {
		t.Fatalf("newest=%+v", sum[0])
	}
	r := []rune(sum[1].Title)
	if sum[1].ID != "c1" || len(r) != 41 || !strings.HasSuffix(sum[1].Title, "…") {
		t.Fatalf("truncated=%q len=%d", sum[1].Title, len(r))
	}
	s.Clear("c2")
	sum = s.ListSummaries()
	if len(sum) != 1 || sum[0].ID != "c1" {
		t.Fatalf("after clear %+v", sum)
	}
}
```

`sqlite_test.go` 用 `OpenSQLite` 做同一断言（不必 sleep：第二条对话后 `updated_at` 更大即可；若同秒冲突，先 Append c1 全套再 Append c2）。

- [ ] **步骤 2：跑测试确认失败**

```powershell
$env:PATH = "C:\Users\Administrator\sdk\go\bin;" + $env:PATH
$env:GOPROXY = "https://goproxy.cn,direct"
go test ./internal/conversation/ -count=1
```

预期：编译失败 `ListSummaries undefined`。

- [ ] **步骤 3：实现**

`summary.go`：

```go
package conversation

import "time"

type Summary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
}

func TruncateTitle(s string) string {
	r := []rune(s)
	if len(r) <= 40 {
		return s
	}
	return string(r[:40]) + "…"
}

func Summarize(id string, msgs []Message) Summary {
	sum := Summary{ID: id, Title: "新对话"}
	for _, m := range msgs {
		if m.CreatedAt.After(sum.UpdatedAt) {
			sum.UpdatedAt = m.CreatedAt
		}
	}
	for _, m := range msgs {
		if m.Role == RoleUser {
			sum.Title = TruncateTitle(m.Content)
			break
		}
	}
	return sum
}
```

Memory：`ListSummaries` 遍历 `s.msgs`，跳过空切片，`sort.Slice` 按 `UpdatedAt` 降序。

SQLite：

```sql
SELECT conversation_id, MAX(created_at) FROM messages GROUP BY conversation_id
```

再对每个 id `List` 后 `Summarize`（消息量按对话很小，YAGNI 单条 SQL 窗口函数）。按 `UpdatedAt` 降序。

接口：

```go
ListSummaries() []Summary
```

- [ ] **步骤 4：测试通过**（同步骤 2 命令）

- [ ] **步骤 5：Commit** `feat(conversation): 按消息聚合对话列表摘要`

---

### 任务 2：GET /v0/conversations

**文件：**
- 修改：`internal/api/server.go`（`routes` 增加 `GET /v0/conversations`，须写在 `{id}/messages` 旁；Go 1.22 mux 可同时注册）
- 修改：`internal/api/server_test.go`

- [ ] **步骤 1：失败测试**

```go
func TestListConversations(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	msgs := conversation.NewMemoryStore()
	srv.Messages = msgs
	_, _ = msgs.Append("conv_a", conversation.Message{Role: conversation.RoleUser, Content: "VPN 挂了"})

	req := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Conversations []conversation.Summary `json:"conversations"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) != 1 || body.Conversations[0].ID != "conv_a" || body.Conversations[0].Title != "VPN 挂了" {
		t.Fatalf("%+v", body)
	}
}

func TestListConversationsNilStore(t *testing.T) {
	st := store.NewMemory()
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	req := httptest.NewRequest(http.MethodGet, "/v0/conversations", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"conversations"`) {
		t.Fatalf("body=%s", rr.Body.String())
	}
}
```

- [ ] **步骤 2：** `go test ./internal/api/ -count=1 -run TestListConversations` 预期 404 或空路由。

- [ ] **步骤 3：实现**

```go
s.mux.HandleFunc("GET /v0/conversations", s.handleListConversations)

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	out := []conversation.Summary{}
	if s.Messages != nil {
		if sum := s.Messages.ListSummaries(); sum != nil {
			out = sum
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": out})
}
```

- [ ] **步骤 4：** 上述测试通过。

- [ ] **步骤 5：Commit** `feat(api): 列出对话 GET /v0/conversations`

---

### 任务 3：eventbus + SSE

**文件：**
- 创建：`internal/eventbus/hub.go`、`store.go`、`hub_test.go`
- 修改：`internal/api/server.go`（`Hub *eventbus.Hub`、`handleRunStream`）
- 修改：`internal/api/server_test.go`
- 修改：`internal/bootstrap/bootstrap.go`（包装时机见下）
- 修改：`docs/architecture-and-plugin-protocol.md` §3 事件推送一行改为已实现 stream

**包装时机：** `openConversationAndIdentities` 使用 `st.(*store.SQLite)`。必须在该调用和 `registerConnector` **之后**、创建 `run.Engine` **之前**：

```go
hub := eventbus.NewHub()
st = eventbus.Notify(st, hub)
engine := &run.Engine{Store: st, ...}
srv := api.NewServer(st, reg, engine)
srv.Hub = hub
```

`Notify` 用嵌入 `store.Store`，覆盖 `AppendEvent`（成功后 `ListEvents` 取下标 `len-1` 再 `Publish`）和 `UpdateRun`（status 为 `succeeded`/`failed` 时 `PublishEnd`）。

- [ ] **步骤 1：hub 测试**

```go
func TestNotifyAppendAndEnd(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	sub := hub.Subscribe(run.ID)
	defer sub.Cancel()
	if err := st.AppendEvent(run.ID, store.Event{Type: "run.started"}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-sub.Events:
		if ev.Index != 0 || ev.Event.Type != "run.started" {
			t.Fatalf("%+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	if err := st.UpdateRun(run.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	select {
	case stt := <-sub.Ended:
		if stt != store.StatusSucceeded {
			t.Fatalf("%s", stt)
		}
	case <-time.After(time.Second):
		t.Fatal("no end")
	}
}
```

`Subscribe` 通道缓冲至少 16。

- [ ] **步骤 2：** `go test ./internal/eventbus/ -count=1` 失败。

- [ ] **步骤 3：实现 Hub / Notify。**

`IndexedEvent`：`Index int`、`Event store.Event`。

- [ ] **步骤 4：** eventbus 测试通过。

- [ ] **步骤 5：SSE HTTP 测试**（`Hub` 未设置时 503 或退化为只回放不推增量——**规格要求 fan-out**，测试里必须注入 Hub + Notify 后的同一 Store）：

```go
func TestRunStreamReplayAndAfter(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Hub = hub
	run, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	_ = st.AppendEvent(run.ID, store.Event{Type: "run.started"})
	_ = st.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "hi"}})

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=0", nil)
	rr := httptest.NewRecorder()
	// 用可取消 ctx，避免测试挂死：另起 goroutine 写完首批后 UpdateRun
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	_ = st.UpdateRun(run.ID, store.StatusSucceeded, "x", "")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("stream did not end")
	}
	cancel()
	body := rr.Body.String()
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("ct=%s", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, "llm.message") || !strings.Contains(body, "run.ended") {
		t.Fatalf("body=%s", body)
	}
	if strings.Contains(body, "run.started") {
		t.Fatalf("after=0 should skip index 0: %s", body)
	}
}
```

`fakeRunner` 的 `Execute` 会自己 `AppendEvent`/`UpdateRun`；本测试不走 Execute，直接 Store。

404：`GET /v0/runs/nope/stream` → `run_not_found`。

Hub 为 nil：回放 `ListEvents` 后若 run 已终态则立刻 `run.ended` 并关闭；否则仍订阅不了增量——**bootstrap 必须设 Hub**。测试 `TestRunStreamRequiresHubForLive` 可省略，改为 bootstrap 单测太重；在 handler 里 `if s.Hub == nil { /* 仅回放 + 若已终态则 ended */ }`。

- [ ] **步骤 6：实现 `handleRunStream`**

头：`Content-Type: text/event-stream`、`Cache-Control: no-cache`、`Connection: keep-alive`。

`after` 查询参数；若有 `Last-Event-ID` 头则覆盖。解析失败当 `-1`（从头）。

流程：`GetRun` 404；`ListEvents` 回放 `index > after`；`Subscribe`；把订阅里已有缓冲与回放去重（只信下标）；循环 `select`：Events / Ended / ctx.Done() / 15s ping（写 `: ping\n\n`）。每条：

```
id: {index}

data: {json event}

```

终态：

```
event: run.ended

data: {"status":"succeeded"}

```

然后 return。`Flush` 用 `http.NewResponseController(w).Flush()`。

若回放时 run 已是 succeeded/failed，回放完事件后发 `run.ended` 关闭，不必挂起。

- [ ] **步骤 7：** `go test ./internal/eventbus/ ./internal/api/ ./internal/bootstrap/ -count=1`

- [ ] **步骤 8：Commit** `feat(api): Run 事件 SSE stream 与进程内 fan-out`

---

### 任务 4：foldEvents + React 脚手架

**文件：** 见结构表 `web/chat/**`、`index.html`、`vite.config.ts`、`tsconfig.json`、`package.json`

- [ ] **步骤 1：** `web/chat` 安装依赖（国内可用 `https://registry.npmmirror.com`）：

```powershell
cd web/chat
npm install react react-dom react-router-dom
npm install -D @types/react @types/react-dom @vitejs/plugin-react vitest jsdom
```

`package.json` scripts 加 `"test": "vitest run"`。`vite.config.ts`：

```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  base: '/ui/',
  plugins: [react()],
  server: { proxy: { '/v0': 'http://127.0.0.1:8080' } },
  build: { outDir: '../../internal/ui/dist', emptyOutDir: true },
  test: { environment: 'node' },
})
```

`tsconfig.json` 加 `"jsx": "react-jsx"`。

- [ ] **步骤 2：写 `foldEvents.ts` 与失败测试 `foldEvents.test.ts`**

```ts
export type ChatBlock =
  | { kind: 'user'; text: string }
  | { kind: 'assistant'; text: string }
  | { kind: 'system'; text: string }
  | {
      kind: 'tool'
      name: string
      status: 'running' | 'waiting_human' | 'succeeded' | 'failed' | 'approved' | 'rejected'
      arguments?: unknown
      result?: unknown
      isError?: boolean
      runId: string
    }

export function foldEvents(runId: string, events: Event[]): ChatBlock[]
```

`Event` 从 `api.ts` 导入。规则：`llm.tool_call` 开一张 `running` 卡片；随后同名 `tool.result` 改该卡片（栈顶未完成的同名 tool）；`hitl.waiting` 把对应卡片改为 `waiting_human`；`hitl.resumed`/`rejected` 改 `approved`/`rejected`；`llm.message` 助手；`llm.error` system。测试至少：call+result 一张卡；waiting 带按钮所需 status。

- [ ] **步骤 3：** `npx vitest run` 先红后实现 `foldEvents` 再绿。

- [ ] **步骤 4：** `main.tsx` + 最小 Router：`/` Chat 占位、`/settings/tools` 占位。`index.html` 脚本改 `/src/main.tsx`。删除 `main.ts`。

- [ ] **步骤 5：** `npm run build`；`go test ./internal/api/ -run TestUIIndex -count=1` 仍含 `Baize Chat`。

- [ ] **步骤 6：Commit** `feat(ui): React 脚手架与事件折叠为工具卡片`

（`internal/ui/dist` 若被 git 跟踪则一并 add。）

---

### 任务 5：Chat 页（列表、composer、SSE、ToolCard）

**文件：** `ChatPage.tsx`、`ToolCard.tsx`、`Composer.tsx`、`style.css`、`api.ts`

- [ ] **步骤 1：** `api.ts` 增加：

```ts
export async function listConversations(): Promise<{ id: string; title: string; updated_at: string }[]> {
  const res = await fetch('/v0/conversations')
  const body = await parseJSON<{ conversations: { id: string; title: string; updated_at: string }[] }>(res)
  return body.conversations ?? []
}

export function openRunStream(
  runId: string,
  after: number,
  onEvent: (e: Event, index: number) => void,
  onEnded: (status: string) => void,
  onFatal: () => void,
): () => void
```

`openRunStream` 用 `EventSource('/v0/runs/' + id + '/stream?after=' + after)`。解析 `message` 的 `data` 为 Event，`e.lastEventId` 为 index。监听 `run.ended`。`onerror` 调用 `onFatal` 并 `close`（ChatPage 再改 700ms 轮询 `getRun`+`listEvents`）。返回 cancel。

- [ ] **步骤 2：** 实现壳 CSS：左栏 240px `#f9f9fa`，主区白底，底栏圆角输入。左下 `Link to=/settings/tools` 设置。左栏 `listConversations`，点击切换 `conversation_id`（`localStorage` 键仍 `baize.conversation_id`）。新对话生成 `conv_${crypto.randomUUID()}`。主区：`listMessages` 画 user/assistant；当前 Run 用 `foldEvents` 追加卡片与助手增量（避免与已持久 messages 重复：Run 进行中以 events 为准画工具卡；历史 messages 只画已结束轮次）。

简化历史：加载 messages 为气泡；**进行中的 Run** 另用 foldEvents 画 tool 卡 + 未写入的 assistant 流。HITL 只出现在卡片。

- [ ] **步骤 3：** `ToolCard`：显示 name + 中文状态；`waiting_human` 黄边 + 批准/驳回 → `resumeRun`。展开 JSON。resume 失败在卡片内显示错误。

- [ ] **步骤 4：** Composer：Enter 发送（Shift+Enter 换行）；`createRun` 后 `openRunStream`。`+` 按钮 `title="连接器在设置中配置"`。

- [ ] **步骤 5：** `npm run build`；`go test ./...`（缓存即可）。Commit `feat(ui): 对话壳、工具卡片与 SSE 客户端`

---

### 任务 6：设置页

**文件：** `SettingsLayout.tsx`、`ToolsSettings.tsx`、`IdentitiesSettings.tsx`、`ComingSoon.tsx`、`main.tsx` 路由

- [ ] **步骤 1：** 路由：

```
/settings/tools → ToolsSettings
/settings/identities → IdentitiesSettings（使用当前 conversation_id）
/settings/mcp → ComingSoon title=MCP
/settings/plugins → ComingSoon title=插件
```

设置布局左侧链到上述路径；返回聊天 `Link to=/`。

- [ ] **步骤 2：** Tools：`listTools()` 表格/列表 `name · method path · 需审批`。空：尚未注册 Connector。

- [ ] **步骤 3：** Identities：从现 `main.ts` 搬渲染与 `setDefaultIdentity` / `deleteIdentity` / `clearIdentities`（脱敏逻辑一并搬到 `web/chat/src/sensitive.ts` 以免丢失）。

- [ ] **步骤 4：** ComingSoon：文案「即将接入，不会在此填写假配置。」

- [ ] **步骤 5：** `npm run build`；`go test ./internal/api/ -run TestUIIndex`；`go test ./...`

- [ ] **步骤 6：Commit** `feat(ui): 设置页只读 Tools、账号与 MCP/插件空状态`

---

## 自检

| 规格 | 任务 |
|------|------|
| 壳 / 左栏 / 无主区 Tools 面板 | 5 |
| 工具卡片 B + HITL | 4 foldEvents、5 ToolCard |
| GET conversations + 标题 + Clear 消失 | 1–2、5 |
| 设置 Tools/identities/mcp/plugins | 6 |
| SSE + 回退轮询 | 3、5 `onFatal` |
| React embed | 4–6 |
| 空态 | 5 欢迎句、6 空 Tools |
| 不改 yaml/引擎/MCP 真连接 | 全局 |

无 TODO/待定占位。类型名统一：`conversation.Summary`、`eventbus.Hub`、`eventbus.Notify`、`foldEvents`、`ChatBlock`。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-15-chat-ui-shell.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 每个任务新子代理，任务间审查  

**2. 内联执行** — 本会话按任务做，批量检查点  

选哪种方式？
