# Baize Demo A 设计规格

> 状态：已批准  
> 日期：2026-08-12  
> 范围：第一个可运行里程碑（Demo A）  
> 依据：`docs/architecture-and-plugin-protocol.md` + grilling 共享理解

---

## 1. 目标与非目标

**目标：** 交付一个可本地一键运行的 Go Agent Runtime，证明「导入 OpenAPI → Agent 经 Tool 调用遗留 HTTP」闭环；默认无 API Key、无必选中间件。

**一句话成功标准：** `baize demo` 启动后，一条 `curl` 创建 Run，状态为 `succeeded`，模拟工单系统中出现新工单，且 Run 事件可回放。

### 做（Demo A）

- 独立进程 Runtime（`cmd/baize`：`serve` / `demo`）
- 五抽象中的最小子集：`Agent` / `Tool` / `Connector` / `Run`（`Runtime` 即进程本身）
- 内建 OpenAPI Connector（OpenAPI 3.x → Tool，HTTP 执行）
- 单 Agent ReAct 循环（`max_steps` 可配，默认 8）
- LLM：`mock`（默认）+ 可选 `openai_compatible`
- REST 控制面：agents / connectors / runs / events / healthz
- `examples/mock-ticket`：模拟工单 HTTP + `openapi.yaml`
- 内存 `store`（Run 与事件）
- 单元测试 + 集成测试（仅 mock 路径进默认 CI）

### 不做（Demo A）

- HITL / `waiting_human` / resume
- 工作流 DSL、多 Agent、Skill 包、Memory 插件
- 侧车插件协议服务端、MCP 桥、执行回调专用路径
- SSE / Webhook 事件推送
- SQLite / PG、OTel、企微 Channel
- TypeScript / Python SDK
- 桌面 App、学习闭环、可视化画布
- 基于 LangGraph 或其他 Python Agent 框架（内核坚持自研 Go）

---

## 2. 双仓与模块

| 远程 | 用途 |
|------|------|
| https://github.com/rebornace/baize.git | 开源发布：可运行代码、`examples/`、用户向 README；可含精简架构说明；**不含** `docs/superpowers/**`、grilling/计划过程、工具残留 |
| https://github.com/rebornace/baize_real.git | 开发主仓：上述全部 + `docs/superpowers/specs|plans`、内部笔记、实验脚本 |

**工作流：** 日常在 `baize_real` 开发与提交过程文档 → 发布前将干净树同步到 `baize`。

**Go module：** `github.com/rebornace/baize`

---

## 3. 仓库结构

```
baize/
├── cmd/baize/                 # serve | demo
├── internal/
│   ├── api/                   # REST 控制面
│   ├── agent/                 # Agent 定义
│   ├── run/                   # Run 状态机 + ReAct
│   ├── llm/                   # mock | openai_compatible
│   ├── tool/                  # Tool 注册与分发
│   ├── connector/openapi/     # OpenAPI → Tool + HTTP
│   ├── store/                 # 内存存储
│   └── config/                # 配置
├── examples/mock-ticket/      # 模拟工单服务 + openapi.yaml
├── configs/demo.yaml
├── docs/
│   ├── architecture-and-plugin-protocol.md   # 可同步开源（用户向）
│   └── superpowers/                          # 仅 baize_real
│       └── specs/2026-08-12-baize-demo-a-design.md
├── go.mod
├── README.md                  # 开源仓面向用户；30 分钟路径
└── .gitignore
```

---

## 4. 组件职责

| 组件 | 职责 | 依赖 |
|------|------|------|
| `config` | 加载 listen、LLM、agent、connector | 文件 YAML |
| `store` | 保存 Agent/Connector/Run/Events | 无 |
| `llm.Provider` | `Chat(ctx, messages, tools) → message｜tool_calls` | 无（mock）或 HTTP |
| `connector.openapi` | 解析 spec、注册 Tool、按 mapping 发 HTTP | `net/http` |
| `tool.Registry` | name → invoker | connectors |
| `run.Engine` | ReAct 循环、写事件、更新状态 | llm, tool, store |
| `api` | REST 接线 | engine, store |
| `demo` | 同进程内另起 HTTP Server 跑 mock-ticket + 自动注册 | 全部 |

---

## 5. 控制面 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | `{ "status": "ok" }` |
| PUT | `/v0/agents/{id}` | 注册/更新 Agent |
| PUT | `/v0/connectors/{id}` | 注册/更新 Connector（type=`openapi`） |
| POST | `/v0/runs` | body: `{ "agent_id", "input" }` → `{ "run_id", "status" }` |
| GET | `/v0/runs/{id}` | status、output、摘要 |
| GET | `/v0/runs/{id}/events` | 有序事件列表 |

**Run 状态（Demo A）：** `queued` → `running` → `succeeded` | `failed`  
（无 `waiting_human` / `cancelled` 实现，枚举可预留但不暴露。）

**错误体：** `{ "error": { "code": "string", "message": "string" } }`

---

## 6. 数据流

1. `POST /v0/runs` → store 创建 Run(`running`)，event `run.started`
2. ReAct：`messages` + tool schemas → `llm.Provider`
3. 若返回 `tool_calls`：event `llm.tool_call` → `tool.Registry` → OpenAPI HTTP → event `tool.result`（失败则 `is_error: true` 并回灌）
4. 若返回最终文本：event `llm.message` → Run `succeeded`，写入 `output`
5. `max_steps` 耗尽或 LLM 错误 → Run `failed`

**`baize demo`：**

1. 在同一进程内启动 mock-ticket HTTP Server（默认 `:18080`；代码位于 `examples/mock-ticket`，由 demo 引用，不进入 `internal` 内核包）
2. 启动 Runtime（默认 `:8080`）
3. 按 `configs/demo.yaml` 注册 Agent + OpenAPI Connector
4. 健康检查 mock-ticket；失败则非 0 退出
5. 打印示例 `curl`

---

## 7. LLM

### mock（默认）

启发式（实现可微调，测试锁定行为）：

- 输入含「创建」或 `create` → `create_ticket`（从文本抽取 title，缺省用原文截断）
- 输入含「查询」「列表」或 `list` → `list_tickets`
- 已有成功的 tool 结果 → 返回简短中文总结，不再调 tool
- 无法匹配 → 返回纯文本说明可用能力（Run 仍 `succeeded`）

### openai_compatible（可选）

- 配置：`base_url`、`api_key`（环境变量）、`model`
- 与 mock 同一 `Provider` 接口；**不进默认 CI**

---

## 8. OpenAPI Connector

1. 加载 OpenAPI 3.x（文件路径）；每个 operation → 一个 Tool
2. Tool `name`：优先 `operationId`，否则 `method_path` 规范化
3. `input_schema`：合并 path/query/header/body 参数为 JSON Schema 对象（Demo A 允许简化：仅 body + 必要 path 参数）
4. Invoke：用 `base_url` + path 发 HTTP；`auth` Demo A 仅支持 `none` 与 `passthrough`（请求头透传可选，demo 默认 `none`）
5. 响应：非 2xx → tool 错误结果，不直接崩溃进程

**mock-ticket 最小 API：**

- `GET /tickets` → 列表
- `POST /tickets` → 创建（`title` 必填，`priority` 可选）
- `GET /openapi.yaml` 或仓库内静态 `openapi.yaml` 供 Connector 读取
- `GET /healthz`

---

## 9. 配置示例

```yaml
listen: ":8080"
llm:
  provider: mock
  # provider: openai_compatible
  # base_url: https://api.openai.com/v1
  # model: gpt-4o-mini
agent:
  id: ticket-agent
  system: "你是企业工单助手，只能通过工具访问工单系统。"
connector:
  id: ticket-api
  type: openapi
  spec: examples/mock-ticket/openapi.yaml
  base_url: http://127.0.0.1:18080
run:
  max_steps: 8
```

---

## 10. 错误处理

| 情况 | 行为 |
|------|------|
| API 校验失败 | HTTP 400 + error body |
| Tool/HTTP 失败 | `tool.result` 带错误；可继续循环；超步或不可恢复 → Run `failed` |
| LLM 失败 | event `llm.error`，Run `failed` |
| demo 依赖未就绪 | 重试健康检查后退出非 0 |

不做：跨 Run 自动重试、分布式补偿。

---

## 11. 测试

1. **单元：** OpenAPI `operationId` → Tool 名与 method/path 映射
2. **单元：** `llm.mock` 对「创建…」「查询…」选出预期 tool
3. **集成：** 启动 mock-ticket + Runtime，创建 Run → `succeeded`，且存在至少一张工单
4. **CI：** 仅 mock；无外网、无真实 LLM Key

---

## 12. 与完整架构的关系

本规格是 `architecture-and-plugin-protocol.md` 的 **垂直切片**：只兑现 Demo A 验收故事第 1 条。HITL、插件协议 v0 服务端、SQLite、DSL、SDK 等留待后续规格，不在本实现计划内膨胀。

---

## 13. 决议摘要

- 里程碑：Demo A（非 HITL、非骨架空仓）
- LLM：mock 默认 + openai_compatible 可选
- 形态：单二进制方案，`examples/mock-ticket` 隔离
- 内核：自研 Go，不基于 LangGraph
- 双仓：过程文档仅 `baize_real`；开源仓保持干净
