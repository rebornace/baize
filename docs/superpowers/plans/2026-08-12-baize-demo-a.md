# Baize Demo A 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现可一键运行的 Go Runtime Demo A：OpenAPI Connector + mock LLM + ReAct，使 `baize demo` 后一条 curl 能创建模拟工单并回放 Run 事件。

**架构：** 单进程内 Runtime（`:8080`）与 `examples/mock-ticket`（`:18080`）；内存 store；OpenAPI 解析为 Tool；`llm.mock` 启发式选 tool；`run.Engine` 跑 ReAct；`api` 暴露 REST。不引入 LangGraph / SQLite / HITL。

**技术栈：** Go 1.22+、标准库 `net/http`、`encoding/json`、`gopkg.in/yaml.v3`、OpenAPI 用 `github.com/getkin/kin-openapi`（仅解析，保持依赖可控）。

**规格：** `docs/superpowers/specs/2026-08-12-baize-demo-a-design.md`  
**Git：** 过程提交进 `baize_real`；同步开源仓时排除 `docs/superpowers/**`。

---

## 文件结构（将创建）

| 路径 | 职责 |
|------|------|
| `go.mod` | module `github.com/rebornace/baize` |
| `internal/store/store.go` | 内存 Agent/Connector/Run/Event |
| `internal/store/store_test.go` | store 单测 |
| `internal/llm/provider.go` | Provider 接口与消息/工具类型 |
| `internal/llm/mock.go` | mock 启发式 |
| `internal/llm/mock_test.go` | mock 单测 |
| `internal/llm/openai.go` | openai_compatible（可选路径） |
| `internal/connector/openapi/loader.go` | OpenAPI → ToolSpec + Route |
| `internal/connector/openapi/invoke.go` | HTTP 执行 |
| `internal/connector/openapi/loader_test.go` | 映射单测 |
| `internal/tool/registry.go` | name → Invoker |
| `internal/tool/registry_test.go` | 注册/调用单测 |
| `internal/agent/agent.go` | Agent 定义结构 |
| `internal/run/engine.go` | ReAct 引擎 |
| `internal/run/engine_test.go` | 引擎单测（fake LLM + fake tool） |
| `internal/api/server.go` | REST 路由与 handler |
| `internal/api/server_test.go` | API 单测 |
| `internal/config/config.go` | YAML 配置 |
| `examples/mock-ticket/openapi.yaml` | 工单 OpenAPI |
| `examples/mock-ticket/server.go` | 可被 demo import 的 Handler |
| `examples/mock-ticket/server_test.go` | 工单 API 单测 |
| `configs/demo.yaml` | demo 配置 |
| `cmd/baize/main.go` | serve / demo 入口 |
| `internal/demo/demo.go` | 一键装配 |
| `tests/integration/demo_a_test.go` | 端到端集成测试 |
| `README.md` | 用户向 30 分钟路径 |

---

### 任务 1：Go module + 内存 Store

**文件：**
- 创建：`go.mod`
- 创建：`internal/store/store.go`
- 创建：`internal/store/store_test.go`

- [ ] **步骤 1：初始化 module**

```bash
cd C:/Users/Administrator/Desktop/baize
go mod init github.com/rebornace/baize
```

- [ ] **步骤 2：编写失败的 store 测试**

```go
package store_test

import (
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestCreateAndGetRun(t *testing.T) {
	s := store.New()
	r, err := s.CreateRun("ticket-agent", "创建工单")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID == "" || r.Status != store.StatusRunning {
		t.Fatalf("got %+v", r)
	}
	got, err := s.GetRun(r.ID)
	if err != nil || got.Input != "创建工单" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	s.AppendEvent(r.ID, store.Event{Type: "run.started"})
	evs, _ := s.ListEvents(r.ID)
	if len(evs) != 1 || evs[0].Type != "run.started" {
		t.Fatalf("events=%+v", evs)
	}
}
```

- [ ] **步骤 3：运行测试确认失败**

```bash
go test ./internal/store/ -v
```

预期：FAIL（包不存在或缺类型）

- [ ] **步骤 4：实现最少 Store**

`internal/store/store.go` 需包含：

```go
package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type Agent struct {
	ID     string `json:"id"`
	System string `json:"system"`
}

type Connector struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // openapi
	Spec    string `json:"spec"`
	BaseURL string `json:"base_url"`
}

type Event struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

type Run struct {
	ID        string  `json:"id"`
	AgentID   string  `json:"agent_id"`
	Input     string  `json:"input"`
	Status    Status  `json:"status"`
	Output    string  `json:"output,omitempty"`
	Error     string  `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu         sync.RWMutex
	agents     map[string]Agent
	connectors map[string]Connector
	runs       map[string]*Run
	events     map[string][]Event
}

func New() *Store {
	return &Store{
		agents:     map[string]Agent{},
		connectors: map[string]Connector{},
		runs:       map[string]*Run{},
		events:     map[string][]Event{},
	}
}

func (s *Store) UpsertAgent(a Agent) { /* lock; save */ }
func (s *Store) GetAgent(id string) (Agent, error) { /* ... */ }
func (s *Store) UpsertConnector(c Connector) { /* ... */ }
func (s *Store) GetConnector(id string) (Connector, error) { /* ... */ }
func (s *Store) CreateRun(agentID, input string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "run_" + uuid.NewString()
	r := &Run{ID: id, AgentID: agentID, Input: input, Status: StatusRunning, CreatedAt: time.Now().UTC()}
	s.runs[id] = r
	s.events[id] = nil
	return r, nil
}
func (s *Store) GetRun(id string) (*Run, error) { /* copy out */ }
func (s *Store) UpdateRun(id string, status Status, output, errMsg string) error { /* ... */ }
func (s *Store) AppendEvent(runID string, ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.runs[runID]; !ok {
		return fmt.Errorf("run not found")
	}
	ev.Timestamp = time.Now().UTC()
	s.events[runID] = append(s.events[runID], ev)
	return nil
}
func (s *Store) ListEvents(runID string) ([]Event, error) { /* copy slice */ }
```

依赖：`go get github.com/google/uuid`

- [ ] **步骤 5：运行测试确认通过**

```bash
go test ./internal/store/ -v
```

预期：PASS

- [ ] **步骤 6：Commit（baize_real）**

```bash
git add go.mod go.sum internal/store/
git commit -m "feat(store): 内存 Store 与 Run 事件"
```

---

### 任务 2：mock-ticket 示例服务

**文件：**
- 创建：`examples/mock-ticket/openapi.yaml`
- 创建：`examples/mock-ticket/server.go`
- 创建：`examples/mock-ticket/server_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
package mockticket_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
)

func TestCreateAndListTickets(t *testing.T) {
	h := mockticket.NewHandler()
	srv := httptest.NewServer(h)
	defer srv.Close()

	body, _ := json.Marshal(map[string]string{"title": "VPN 挂了", "priority": "high"})
	res, err := http.Post(srv.URL+"/tickets", "application/json", bytes.NewReader(body))
	if err != nil || res.StatusCode != 201 {
		t.Fatalf("create: %v %v", err, res)
	}
	res, err = http.Get(srv.URL + "/tickets")
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	json.NewDecoder(res.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("list=%v", list)
	}
}
```

- [ ] **步骤 2：运行测试确认失败**

```bash
go test ./examples/mock-ticket/ -v
```

预期：FAIL

- [ ] **步骤 3：实现 Handler + openapi.yaml**

`openapi.yaml` 最小内容：

```yaml
openapi: 3.0.3
info:
  title: Mock Ticket API
  version: 0.1.0
paths:
  /tickets:
    get:
      operationId: list_tickets
      responses:
        "200":
          description: ok
    post:
      operationId: create_ticket
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [title]
              properties:
                title: { type: string }
                priority: { type: string }
      responses:
        "201":
          description: created
```

`server.go`：内存 slice 存工单；`GET/POST /tickets`；`GET /healthz` → `{"status":"ok"}`；导出 `NewHandler() http.Handler`。

- [ ] **步骤 4：测试通过并 Commit**

```bash
go test ./examples/mock-ticket/ -v
git add examples/mock-ticket/
git commit -m "feat(examples): mock-ticket 服务与 OpenAPI"
```

---

### 任务 3：OpenAPI Connector（加载 + 调用）

**文件：**
- 创建：`internal/connector/openapi/loader.go`
- 创建：`internal/connector/openapi/invoke.go`
- 创建：`internal/connector/openapi/loader_test.go`
- 创建：测试用 fixture 可复用 `examples/mock-ticket/openapi.yaml`

- [ ] **步骤 1：编写失败的映射测试**

```go
func TestLoadToolsFromSpec(t *testing.T) {
	tools, err := openapi.LoadTools("../../examples/mock-ticket/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
	}
	if !names["create_ticket"] || !names["list_tickets"] {
		t.Fatalf("tools=%v", names)
	}
	var create openapi.ToolRoute
	for _, tl := range tools {
		if tl.Name == "create_ticket" {
			create = tl
		}
	}
	if create.Method != http.MethodPost || create.Path != "/tickets" {
		t.Fatalf("route=%+v", create)
	}
}
```

类型约定：

```go
type ToolRoute struct {
	Name        string
	Description string
	Method      string
	Path        string
	InputSchema map[string]any
}
```

- [ ] **步骤 2：实现 LoadTools（kin-openapi）**

```bash
go get github.com/getkin/kin-openapi/openapi3
```

遍历 `Paths`/`Operations`，`operationId` 为 Name；无 operationId 则用 `get_tickets` 形式规范化。Demo A：`create_ticket` 的 InputSchema 至少含 `title`（string）与可选 `priority`。

- [ ] **步骤 3：编写 Invoke 集成片段测试**

用 `httptest` 起 `mockticket.NewHandler()`，`LoadTools` + `Invoker{BaseURL: srv.URL}.Invoke(ctx, "create_ticket", args)`，断言返回非 error 且含 id/title。

- [ ] **步骤 4：实现 Invoker.Invoke**

按 ToolRoute 拼 URL、JSON body（POST）、读响应 body 为 `map[string]any` 或 raw string；非 2xx 返回 `InvokeResult{IsError: true, Content: ...}`，不 panic。

```go
type InvokeResult struct {
	Content map[string]any
	IsError bool
}
```

- [ ] **步骤 5：测试通过并 Commit**

```bash
go test ./internal/connector/openapi/ -v
git add internal/connector/openapi/ go.mod go.sum
git commit -m "feat(connector): OpenAPI 加载与 HTTP 调用"
```

---

### 任务 4：LLM Provider（mock + 接口 + openai 骨架）

**文件：**
- 创建：`internal/llm/provider.go`
- 创建：`internal/llm/mock.go`
- 创建：`internal/llm/mock_test.go`
- 创建：`internal/llm/openai.go`

- [ ] **步骤 1：定义接口**

```go
package llm

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Provider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error)
}
```

- [ ] **步骤 2：mock 失败测试**

```go
func TestMockCreateTicket(t *testing.T) {
	p := llm.NewMock()
	msg, err := p.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "创建一个紧急工单：VPN 挂了"},
	}, []llm.ToolSpec{{Name: "create_ticket"}, {Name: "list_tickets"}})
	if err != nil || len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Name != "create_ticket" {
		t.Fatalf("%+v %v", msg, err)
	}
}

func TestMockSummarizeAfterTool(t *testing.T) {
	p := llm.NewMock()
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "创建工单"},
		{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{{ID: "1", Name: "create_ticket"}}},
		{Role: llm.RoleTool, ToolCallID: "1", Content: `{"id":"t1"}`},
	}
	msg, err := p.Chat(context.Background(), msgs, nil)
	if err != nil || msg.Content == "" || len(msg.ToolCalls) != 0 {
		t.Fatalf("%+v %v", msg, err)
	}
}
```

- [ ] **步骤 3：实现 mock 启发式（与规格一致）**

- 历史中已有 `RoleTool` → 返回中文总结，无 tool_calls  
- 否则用户文本含「创建」或 `create` → `create_ticket`，`title` 取「：」后或全文截断至 80 字，`priority` 若含「紧急」则为 `high`  
- 含「查询」「列表」或 `list` → `list_tickets`  
- 否则纯文本说明  

- [ ] **步骤 4：`openai.go` 实现同一接口**

读取 `base_url`/`api_key`/`model`；POST `{base}/chat/completions`；解析 `tool_calls`。无 Key 时返回明确 error。不要求默认测试覆盖。

- [ ] **步骤 5：测试通过并 Commit**

```bash
go test ./internal/llm/ -v
git add internal/llm/
git commit -m "feat(llm): Provider 接口与 mock/openai_compatible"
```

---

### 任务 5：Tool Registry + Agent 类型

**文件：**
- 创建：`internal/tool/registry.go`
- 创建：`internal/tool/registry_test.go`
- 创建：`internal/agent/agent.go`

- [ ] **步骤 1：Registry 测试**

```go
func TestRegistryInvoke(t *testing.T) {
	r := tool.NewRegistry()
	r.Register("echo", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"ok": true, "args": args}, false, nil
	})
	out, isErr, err := r.Invoke(context.Background(), "echo", map[string]any{"a": 1})
	if err != nil || isErr || out["ok"] != true {
		t.Fatalf("%v %v %v", out, isErr, err)
	}
}
```

- [ ] **步骤 2：实现 Registry**

```go
type Invoker func(ctx context.Context, args map[string]any) (content map[string]any, isError bool, err error)

type Registry struct { /* map[string]Invoker + ToolSpec list */ }
func (r *Registry) Register(name string, inv Invoker)
func (r *Registry) Specs() []llm.ToolSpec
func (r *Registry) Invoke(...) (map[string]any, bool, error)
```

注册 OpenAPI 工具时同时 `Register` 名称与对应 `Invoker` 闭包，并保存 Description/Schema 供 `Specs()`。

- [ ] **步骤 3：`agent.go`**

```go
package agent

type Def struct {
	ID           string
	System       string
	ConnectorIDs []string // demo 可用单一 connector
}
```

- [ ] **步骤 4：测试通过并 Commit**

```bash
go test ./internal/tool/ -v
git add internal/tool/ internal/agent/
git commit -m "feat(tool): Registry 与 Agent 定义"
```

---

### 任务 6：ReAct Run Engine

**文件：**
- 创建：`internal/run/engine.go`
- 创建：`internal/run/engine_test.go`

- [ ] **步骤 1：编写失败的引擎测试（fake LLM）**

```go
type scriptLLM struct{ calls int }

func (s *scriptLLM) Chat(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Message, error) {
	s.calls++
	if s.calls == 1 {
		return llm.Message{Role: llm.RoleAssistant, ToolCalls: []llm.ToolCall{
			{ID: "c1", Name: "create_ticket", Arguments: map[string]any{"title": "x"}},
		}}, nil
	}
	return llm.Message{Role: llm.RoleAssistant, Content: "已创建"}, nil
}
```

装配：`store` + `scriptLLM` + registry 中 `create_ticket` 返回 `{"id":"1"}`；`Engine.Start(ctx, agent, input)` 后 `GetRun` 为 `succeeded`，events 含 `llm.tool_call` 与 `tool.result`。

- [ ] **步骤 2：实现 Engine**

```go
type Engine struct {
	Store     *store.Store
	LLM       llm.Provider
	Tools     *tool.Registry
	MaxSteps  int // default 8
}

func (e *Engine) Execute(ctx context.Context, runID string, ag agent.Def, input string) error
```

逻辑：

1. `AppendEvent run.started`（若尚未）
2. messages = system + user(input)
3. loop ≤ MaxSteps：`Chat` → 若 tool_calls：对每个 Invoke，append assistant+tool messages 与 events；若无 tool_calls：写 output，`UpdateRun succeeded`，return
4. 超步或 Chat error → `UpdateRun failed`

事件类型字符串固定：`run.started` | `llm.tool_call` | `tool.result` | `llm.message` | `llm.error`

- [ ] **步骤 3：测试通过并 Commit**

```bash
go test ./internal/run/ -v
git add internal/run/
git commit -m "feat(run): ReAct 引擎与事件轨迹"
```

---

### 任务 7：REST API

**文件：**
- 创建：`internal/api/server.go`
- 创建：`internal/api/server_test.go`

- [ ] **步骤 1：API 测试**

使用 `httptest`：`NewServer(store, engine)`；`PUT /v0/agents/ticket-agent`；注册 connector 后由测试直接向 store 注入已绑定 tools 的 engine（或测试里手动 `Upsert` + 预建 registry）。最小路径：

1. PUT agent  
2. 测试用 engine 已能跑（可在 `Server` 构造时注入 `Execute` 函数或完整 Engine）  
3. `POST /v0/runs` `{"agent_id":"ticket-agent","input":"创建工单：测试"}`  
4. `GET /v0/runs/{id}` status succeeded（若用 mock LLM + 真 registry；单元测试可用任务 6 的 fake 通过接口注入）

为可测性：

```go
type Runner interface {
	Execute(ctx context.Context, runID string, ag agent.Def, input string) error
}
```

`Engine` 实现该接口。

- [ ] **步骤 2：实现路由**

| 方法 | 路径 | 行为 |
|------|------|------|
| GET | `/healthz` | 200 `{"status":"ok"}` |
| PUT | `/v0/agents/{id}` | UpsertAgent |
| PUT | `/v0/connectors/{id}` | UpsertConnector；**并**加载 OpenAPI、注册到共享 Registry（Server 持有） |
| POST | `/v0/runs` | CreateRun + 异步或同步 `Execute`（Demo A **同步**执行，降低复杂度） |
| GET | `/v0/runs/{id}` | JSON Run |
| GET | `/v0/runs/{id}/events` | JSON events |

错误：未知 agent → 400/404；统一 `{"error":{"code","message"}}`。

- [ ] **步骤 3：测试通过并 Commit**

```bash
go test ./internal/api/ -v
git add internal/api/
git commit -m "feat(api): REST 控制面 v0"
```

---

### 任务 8：config + demo 装配 + cmd

**文件：**
- 创建：`internal/config/config.go`
- 创建：`configs/demo.yaml`
- 创建：`internal/demo/demo.go`
- 创建：`cmd/baize/main.go`

- [ ] **步骤 1：config 加载**

```go
type Config struct {
	Listen string `yaml:"listen"`
	LLM    struct {
		Provider string `yaml:"provider"`
		BaseURL  string `yaml:"base_url"`
		Model    string `yaml:"model"`
		APIKeyEnv string `yaml:"api_key_env"` // 默认 BAIZE_API_KEY
	} `yaml:"llm"`
	Agent struct {
		ID     string `yaml:"id"`
		System string `yaml:"system"`
	} `yaml:"agent"`
	Connector struct {
		ID      string `yaml:"id"`
		Type    string `yaml:"type"`
		Spec    string `yaml:"spec"`
		BaseURL string `yaml:"base_url"`
	} `yaml:"connector"`
	Run struct {
		MaxSteps int `yaml:"max_steps"`
	} `yaml:"run"`
	Demo struct {
		TicketListen string `yaml:"ticket_listen"` // :18080
	} `yaml:"demo"`
}
```

`go get gopkg.in/yaml.v3`；`configs/demo.yaml` 内容与规格 §9 一致。

- [ ] **步骤 2：`demo.Run(cfg)`**

1. `http.ListenAndServe` mock-ticket 在 goroutine（`examples/mock-ticket`）  
2. 轮询 `GET http://127.0.0.1:18080/healthz` 最多 ~5s  
3. 按 provider 构造 LLM；LoadTools + Registry；Store Upsert agent/connector  
4. 启动 `api.Server` 于 `cfg.Listen`  
5. `log` 打印示例 curl  

- [ ] **步骤 3：`main.go`**

```go
switch os.Args[1] {
case "demo":
  cfg := config.Load("configs/demo.yaml")
  demo.Run(cfg)
case "serve":
  // 只起 Runtime，不启 mock-ticket；配置路径 flag
default:
  fmt.Println("usage: baize <demo|serve>")
}
```

- [ ] **步骤 4：手工冒烟**

```bash
go run ./cmd/baize demo
# 另一终端
curl -s -X POST http://127.0.0.1:8080/v0/runs -H "Content-Type: application/json" -d "{\"agent_id\":\"ticket-agent\",\"input\":\"创建一个紧急工单：VPN 挂了\"}"
curl -s http://127.0.0.1:18080/tickets
```

预期：Run succeeded；tickets 非空。

- [ ] **步骤 5：Commit**

```bash
git add internal/config/ internal/demo/ cmd/baize/ configs/demo.yaml
git commit -m "feat(cmd): baize demo 一键闭环"
```

---

### 任务 9：集成测试 + README

**文件：**
- 创建：`tests/integration/demo_a_test.go`
- 创建：`README.md`

- [ ] **步骤 1：集成测试**

在测试内调用与 `demo.Run` 相同的装配函数但传入 `httptest` 或动态端口（`demo` 支持 `TicketListen`/`Listen` 为 `:0` 并回传实际 URL——若过重，则测试内手动：`NewHandler` + `api.Server` + mock LLM + openapi invoke，不强制起真实 demo 进程）。

推荐可测入口：

```go
// internal/demo/demo.go
func StartForTest(t testing.TB, cfg config.Config) (runtimeURL, ticketURL string, shutdown func())
```

断言：POST run → succeeded；GET tickets len≥1；events 含 `tool.result`。

- [ ] **步骤 2：运行全部测试**

```bash
go test ./... -count=1
```

预期：全部 PASS

- [ ] **步骤 3：README（开源向，中文）**

含：一句话定位、`go run ./cmd/baize demo`、示例 curl、配置改 `openai_compatible` 的说明、指向 `docs/architecture-and-plugin-protocol.md`、声明 MIT、**不**提及 `docs/superpowers` 或双仓内部流程。

- [ ] **步骤 4：Commit**

```bash
git add tests/integration/ README.md
git commit -m "test: Demo A 集成测试与 README"
```

---

## 自检对照规格

| 规格项 | 任务 |
|--------|------|
| Go Runtime serve/demo | 8 |
| OpenAPI Connector | 3、7、8 |
| ReAct + max_steps | 6 |
| mock + openai_compatible | 4 |
| REST agents/connectors/runs/events/healthz | 7 |
| mock-ticket + openapi.yaml | 2 |
| 内存 store | 1 |
| 集成测试 mock CI | 9 |
| 双仓 / 无 HITL/DSL/插件 | 范围外，未列入任务 |
| 自研非 LangGraph | 全程 Go |

无占位符步骤；类型名统一：`store.Run`、`llm.Provider`、`openapi.ToolRoute`、`tool.Registry`、`run.Engine`、`agent.Def`。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-12-baize-demo-a.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每个任务调度新子代理，任务间审查，快速迭代（技能：subagent-driven-development）  
2. **内联执行** — 当前会话按 executing-plans 批量推进并设检查点  

**选哪种方式？**
