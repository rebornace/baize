**中文** | [English](./architecture.en.md)

# 架构与插件边界

Baize（白泽）是独立进程的 **Agent Runtime**（侧车 / 网关）：把 LLM、Tool、遗留 HTTP API 与人机卡点收成可审计的 `Run`。平台用 REST / HTTP 集成；不提供官方语言 SDK（集成方直接调控制面 HTTP）。

**产品叙事（为何值得旁挂）** 见根目录 [README 产品优势](../../README.zh-CN.md#产品优势) 与官网「多次升级后」一节；本页只写边界与实现落点。能力索引：[开发者文档入口](./README.md)。

| 做 | 不做 |
|----|------|
| OpenAPI → Tool、HTTP 插件 / MCP 连接器、执行回调、人工审批、可选线性 workflow；连接器分组与仅一层分组子任务；可选 Capacitor **控制台壳**与机上精简编排（`mobileagent`，见 [移动端](./mobile.md)） | 把完整 `baize serve`（HTTP/SSE/连接器农场）原样塞进手机；官方桌面 App；官方 TS/Python SDK；重画布 / 多层嵌套子任务树；绑死单一云或 Agent 框架 |
| SQLite 默认可跑；可选 Postgres / Redis / S3；连接器重导返回接口变更对照；**run.binding** 运行绑定；可选 OTLP（默认关，设置页可热更新）；**Admin 鉴权的** Prometheus **`GET /metrics`**（默认开）与设置页「运行指标」/ **`GET /v0/settings/logs`**（内存近期日志）；结构化 slog（`log_format` / `BAIZE_LOG_FORMAT`，可热更新）；连接器出站 mTLS（文件路径：OpenAPI / HTTP 插件 / MCP HTTP；设置页可配） | 把 OTel/APM 当作必选；进程内指标/日志库持久化；per-tenant cgroup/egress 沙箱；完整 IdP OBO；服务端强制对方 mTLS；Agent 侧 tool schema version pin；保证重跑结果一致 |

**五抽象：** `Runtime` · `Agent` · `Tool` · `Connector` · `Run`  
Skill / Memory / Channel / 线性 workflow 是配置或插件形态，不升格为第六抽象。

## 逻辑架构

```
企业平台 / 集成方
  REST 控制面 · SSE / Webhook 事件 · Chat UI
          │
          ▼
Baize Runtime (Go)
  Agent · LLM（OpenAI-compatible / 库内 model profile）
  Tool 目录 · Run 引擎（默认 ReAct · 可选线性 workflow）· HITL
  Connector 客户端
     │           │            │
     ▼           ▼            ▼
 OpenAPI 通配  HTTP 插件 v0   MCP 桥
 遗留 REST     侧车 Connector  MCP Server
```

- **MCP 桥（客户端）**：`PUT /v0/connectors/{id}` 注册 `type: mcp`；`tools/list` → 目录 `source=mcp`，Run 内 `tools/call`。
- **接口变更对照**：连接器重导（OpenAPI / 插件 / MCP）成功后，`PUT /v0/connectors/{id}` 响应含 `catalog_diff`（新增 / 删除 / method·path·schema 变更）；设置页保存成功提示展示摘要。Diff **非空**且配置了 `events.webhook` 时，异步 POST `type=catalog.diff`（同 URL / 重试 / 死信；空 Diff 或无 URL 跳过；fail-open）。目录以 Runtime 当前接线为准，不做客户端 version pinning。
- **运行绑定**：每个 Chat Run 在首次主模型调用前写入 `run.binding`（模型端点/profile、策略 knobs、记忆注入、收窄后工具契约指纹、连接器 `catalog_revision`）；写入失败则 Run 失败。工具调用事件带 `contract_hash` / `catalog_revision`，便于接口修复前后对比。`GET /v0/runs/{id}/binding`；Chat 助手消息旁「查看运行绑定」只读 Modal；连接器修订见 `GET /v0/connectors/{id}/catalog-revisions`。
- **可选可观测性**：`runtime.otel_otlp_endpoint` 非空时订阅 eventbus，将 Run / `tool.result`（及 MCP 导出调用）导出为 OTLP span；Chat `tool.result` span 含 `baize.contract_hash` / `baize.catalog_revision`（事件 Data 有时）。空 endpoint 则零开销关闭。事件可带非密钥审计字段（如 `connector_id`），永不写 token/密码。接口变更出站告警走 `events.webhook`，**不是**必选 OTel。**进程内**另提供 Admin 的 `GET /metrics` 与近期日志缓冲（设置 → 运行指标）；二者均不跨重启持久化。`log_format` / OTel 端点可在设置 → 运行参数 → **观测** 热更新（KV 覆盖，不写 YAML）。
- **连接器出站 mTLS**：OpenAPI / HTTP 插件 / **MCP HTTP** 连接器可配 `tls`（`cert_file` / `key_file` / 可选 `ca_file`，本机文件路径）；工具 invoke（及 MCP HTTP discover）使用客户端证书。与 `auth.mode` 正交。Store 只存路径。设置页连接器编辑器「出站 TLS / mTLS」可填路径。**stdio** MCP 不适用。
- **MCP 导出**：Runtime 作为 MCP 服务端（`/v0/mcp/export`），把工具目录的只读子集提供给 Cursor、Claude Desktop、豆包等支持 MCP 的 Agent 客户端；与「MCP 桥」方向相反，不经白泽自己的大语言模型。导出 Key 可配置 **`identity_slots`**（按连接器 / 可选角色绑定身份）——权限边界是**下游业务账号**，不是终端用户 IdP OBO。下游鉴权失败时优先 URL elicitation，客户端不支持 elicitation 时工具错误中**立即**带上公网托管登录 URL（`hosted_login_url`），勿把密码贴进 Agent 对话。`auth.mode: passthrough` 仅把该次 Chat Run 的白名单头带给下游，同样不是 OBO。
- **存储**：默认 SQLite；支持 `postgres` / `memory`。Blob：`file` / `s3` / `memory`。
- **控制面鉴权**：可选 operator / admin 口令；详见 [http-api](./http-api.md) 与 [configuration](./configuration.md)。

## 插件协议 v0（HTTP + JSON）

侧车与 Runtime **同网 HTTP**；内建 OpenAPI Connector 不走本协议。MCP 映射为内部 Tool，不替代本协议。

### 约定

- Base URL 在注册 Connector 时声明。
- 请求头：`Authorization`（透传或注入）、`X-Baize-Run-Id`、`X-Baize-Tenant-Id`（可空）、`X-Baize-Protocol: v0`。
- 错误体：`{ "error": { "code", "message", "retryable" } }`。

### 侧车必须实现

```http
GET  /healthz                     → 200 { "status": "ok" }
GET  /v0/tools                    → { "tools": [ ToolDesc, ... ] }
POST /v0/tools/{tool_name}/invoke → ToolResult
```

Invoke 请求含 `arguments` 与 `context`（`run_id`、`agent_id`、`tenant_id`，以及可选的 `callback_urls`）。配置了 `runtime.public_base_url`（及 HMAC）且带 `run_id` 时，Runtime 注入短期签名的 `callback_urls.event`；侧车可 POST 写入 Run 事件（`plugin.callback`）。未配置 `public_base_url` 则不注入。

### 企业执行回调

Connector 的 `execution_callback_url`：Runtime POST 工具名、参数、`run_id`、`idempotency_key` 与可选 `callback_urls`；企业无需实现 `/v0/tools` 发现。

### 内建 OpenAPI Connector

1. 导入 OpenAPI 3.x → 每 operation → 一个 Tool（名优先 `operationId`）。
2. 凭证优先级（摘要）：强制 `identity_id` → 会话内未过期 Identity → **仅无** `conversation_id` 时用 Connector 默认头 → 空头。有会话且工具 `require_login` 又无凭证时不发下游 HTTP。完整凭证不出现在 events / GET run。
3. **托管登录页**（`/v0/auth/hosted/...`）：按连接器 `hosted_login.methods` 渲染表单 / OAuth；提交后 invoke 登录工具并 capture。目标可为 MCP 导出身份槽或 Chat 工作区账号池（`conversation_id` + `workspace_id`）。凭证路径不经 LLM。在设置「对外登录」配置多种方式与公开基址。
4. 覆盖不了的遗留逻辑 → 侧车或执行回调。

协议主版本在路径与头中的 `v0`；未知主版本 → `400 protocol_unsupported`。

## 工具目录与 Registry

- 目录行在 store（`source`：`spec` / `plugin` / `mcp` / `extra`）。`spec`/`plugin`/`mcp` 可启停不可删；`extra` 可删。
- Registry **只**挂 `enabled=true` 的行；引擎与 HITL 读 Registry，`GET /v0/tools` 读目录（可见停用行）。
- Agent **不**绑定 Connector 子集：默认每次 Run 把全部启用工具交给模型（跨 Connector 工具名全局唯一）。开启工具收敛且工具数超过阈值时，会先经工具预筛（默认标准匹配：BM25；可选增强匹配：BM25 + Embedding 混合检索，见设置「匹配与决策」与 `internal/toolindex`）+ 决策层收敛，只把候选子集下发给主模型（总开关与各功能、TopK 在「运行参数 → 智能提速」；开总开关前需先配决策后端才有收益）。技能 `tools:` 默认是**最低保留集合**（除非 Agent 设置 `tool_binding: exclusive`）：声明过的工具在筛选时尽量保留，未声明的启用连接器工具仍是候选，不会被技能名单单独关掉。exclusive 模式下仅暴露当前已激活技能声明的工具（另加 `activate_skill`）。每条用户消息是新 Run，工具集会重新预筛；会话已有登录凭证后，预筛不再把登录建立类工具（及 managed `connector_login` 技能的 tools 最低保留）硬塞进 Top-K，避免登录成功后仍占满名额。System 会注入当前时间，便于解读工具结果里的时间字段。
- **工具匹配（增强）**：`internal/toolretrieval` 管理本机 Ollama 一键安装 / 模型拉取 / 路径与清理，或 OpenAI 兼容 Embedding API；状态经 `GET/POST…/tool-retrieval*` 见 [HTTP API](./http-api.md)。Windows 国内安装优先 [ModelScope `Lixiang/ollama-release`](https://www.modelscope.cn/models/Lixiang/ollama-release/files)（按稳定 tag 解析 `OllamaSetup.exe`），再回退 ghproxy / 官网等；体积与官方不符的陈旧镜像会跳过。未启用或探测失败时仍用标准匹配，失败则回退原路径。
- **决策模型**：设置「匹配与决策」一键启用；`internal/systemoneenable` 管理本机 Ollama 拉取默认模型 `tev1`（需 Ollama ≥0.35）或兼容 API；写入 `decide_systemone_*`；HTTP 前缀 `/v0/settings/systemone*`。判定顺序：决策模型 → 可选备用 `decide_profile_id`（空则不调主模型）→ 规则。
- **工作区**：Web 对话带 `workspace_id`（缺省 `default`）。同一工作区内的捕获登录身份共享（设置 → 账号）；不同工作区、渠道会话（`source:account:peer`）与 MCP 导出身份（设置 → 对外提供能力）隔离。v1 不迁移已有对话到其他工作区。
- **连接器分组**：YAML / 设置页「匹配与决策 → 连接器分组」维护命名集合（若干 `connector.id`）。工具收窄时，组内任一成员被选中或强制命中，同组其它连接器仍可进候选（事件可带 `groups_expanded`）。**不**自动起子任务。上一轮工具结果含 error 时清会话粘性，避免粘在错系统。
- **分组子任务（需开启）**：打开「允许分组内子任务」后，模型可调用 `spawn_group_run`（仅一层）：子任务工具面限于该组连接器，独立 `run.binding` 与审批；父任务进入 `waiting_child` 直至子回执。禁止再嵌套子任务。父聊天显示「子代理」卡，可展开看子事件与 binding、在子等待时直接审批、停止子 Run；展开且未终态时自动刷新，父事件漏收时用远端子状态兜底。子回执可带 `RECEIPT_JSON` 解析出的 `summary` / `facts`。`spawn_group_run` 可选 `async`：父立即返回并继续对话，子终态写回执与会话系统提示，但不自动唤醒父；同步模式仍是默认。工作流步骤内不允许启动子任务。
- **上下文卸出**：超大体量工具结果或用户粘贴可写入会话工作区 `offload/`；模型侧仅预览 + 路径，可用 `read_file` 分段回读。落库事件 / 气泡仍可保留全文策略。
- **给模型看的投影**（运行参数「长对话保留重点」，默认关）：只改模型输入侧的投影（钉住 / 摘要 / 丢掉过大工具结果），**不改写** `conversation.Messages`。失败则回退 Compactor。不承诺降低云端 API token。
- **前缀缓存**：system + 历史保持稳定；当前时间、记忆块与活动目标在历史之后、当前用户输入之前注入，避免每轮打穿前缀。中途切换模型 / 努力度仍可能损失缓存，长会话可先新建会话。卸出预览声明截断数量。
- **滚动摘要（Compactor）**：超预算时把较旧轮次压成结构化检查点（目标、进度、决定、必须保留的事实），不删原文、不把「先执行某步再探索」写成全局策略。
- **定时任务**：人同意后保存一次性 / 每天 / 每周指令；到点在原会话新开 Run，写工具仍走 HITL。账号隔离见设置页日程。上一轮 Run 未终态时跳过触发；若该轮在等审批，日程会一直跳过，设置页标出「暂停触发 · 等待审批」并给出回到会话的入口。WebUI 会话没有审批推送。到点 Run 带 `origin=scheduled`；模型可回 `NO_UPDATE` 静默收口，WebUI 留一行「无新增」，渠道不发。

## Agent 运行形态

| 模式 | 行为 |
|------|------|
| 默认 | 单 Agent ReAct：选 Tool → 执行 → 写轨迹 → 结束 |
| 可选 | Skill 包内线性 `workflow.yaml`（顺序步骤 + 可选 HITL；无 branch / 循环） |
| 可选 | 分组子任务：`spawn_group_run` → 子任务（仅一层）+ 父任务等待回执（`waiting_child`） |
| Skill | 配置形态：`SKILL.md`（可选 `SKILL.en.md`）+ tools；`agent.skills` 为新对话预激活（样板含 `baize-help`），未入默认仍可 `@`/`activate_skill`；`@id`/`/id` pin 写入用户消息 `pinned_skills`（历史可见）；Composer 可补全 `/reload`；`skills_reload`/`/reload` 或会话自动 dirty；可选 `tool_binding: exclusive` |
| Channel | 带签名的来信入口；即时消息等为进程外适配器 + 声明式 `channels`（见 [部署](./deployment.md)）。**正式适配器：个人微信**；飞书 / 钉钉见 `examples/im-adapter-*` 参考实现 |

## HITL

写操作等可标 `require_approval`：Run 进入 `waiting_human`，运营 / API `POST /v0/runs/{id}/resume` 后继续。Chat UI（`/ui`）提供运营入口。审批 / 登录等待默认 24 小时过期（`hitl.approval_ttl_sec`，`0` 关闭），过期后 Run 记为 `cancelled` 并释放会话；审批卡与日程列表显示已等待时长。工作流步骤等待时，工作流卡片标出「等待审批」或「等待登录」，审批卡提示重启后仍可继续。工作流步骤内的登录与工具自身审批都会持久化步骤状态，恢复后回到同一步；若进程在步骤执行中途退出，重新入队时判失败并提示核对，不自动重放可能已发生的写入。

下游需要登录时（`login_required`）：对话可暂停并展示托管登录入口——打开浏览器托管页、忽略该连接器并继续本轮，或结束本轮。MCP 导出侧见上文「鉴权失败 → elicitation / `hosted_login_url`」。

## 写操作诚实性

成功 Run 若整个 Run 没有一次 `llm.tool_call`，但最终回复命中“已创建 / 已复制 / 已更新 / 已删除”等写完成声明，会追加 `run.unverified_write` 事件与系统提示「本轮没有调用任何工具，但回复声称写操作已完成。请核实结果。」。Run 仍为 `succeeded`，助手原文与 `output` 不变；有任意工具调用时不检查（v1 边界）。

## 定时来源与静默

日程开出的 Run 持久化 `origin=scheduled`；系统提示要求“没有新变化只回 `NO_UPDATE`”。最终正文去掉首尾空白与句末标点后等于 `NO_UPDATE` 时：

- Run 仍 `succeeded`，`output` 为空，事件 `run.silent`
- 不写助手消息、不投递渠道、不触发记忆抽取
- WebUI 追加系统提示「定时检查：无新增。」

手动 Run 的 `NO_UPDATE` 按普通回复处理；`waiting_child`、失败/取消等状态不受影响。

## 功能搜索

WebUI 侧栏「搜索功能」/ `Ctrl/Cmd+K` / `/` 打开命令面板；设置页从 `settingsNav` 自动索引并直达，聊天动作（查看运行绑定、重新回答、审批/登录等）显示位置提示，不执行动作。

## 已知限制

- **工作流步骤审批不再占槽**：`workflow.yaml` 的 `approve: true` 步骤把 skill、下一步与结果树写入 HITL 载荷，Run 进入 `waiting_human` 并释放 worker；同意或重启后从暂停的步骤继续。旧等待（无状态）启动时仍失败并要求重跑；审批后的步骤执行若在结果落库前崩溃，可能重跑该步骤（at-least-once）。
- **HITL 过期是进程内协调**：`hitlMu` 只串行化单进程内的过期与同意；多副本部署时仍可能一边过期一边同意，最终以最后写入的终态为准。
- **多副本日程抢领窗口**：每个副本都会跑 15 秒一次的日程扫描；进程启动时会把全库 `running_run_id=pending` 的占位清空。若另一副本正好处在 `ClaimDue` 与 `CompleteOpen` 之间，可能出现重复开一轮。单副本没有这个问题；多副本部署时避免同时重启。
- **记忆不覆盖同名条目**：自动抽取遇到已存在的 `key` 会跳过，不会改写旧值。要纠正或更新，请用显式记忆（`remember_fact`）或「设置 → 记忆」编辑。
- **定时 Run 无来源标记**：到点 Run 的输入就是日程指令原文，会话历史里与人工输入不可区分，模型也不知道这一轮由定时唤醒，因此目前没有「无事则静默」。
- **并发守卫是进程内的**：UI、inbox、渠道与日程共用一把进程内会话锁；多副本部署时两个副本仍可能各自开出一轮 Run（数据库层不做唯一约束）。
