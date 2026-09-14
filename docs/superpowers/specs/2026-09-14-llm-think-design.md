# LLM-THINK：思考级别、方言映射与流式思考展示

> 状态：**已交付**（计划：[`plans/2026-09-14-llm-think.md`](../plans/2026-09-14-llm-think.md)）  
> 日期：2026-09-14  
> 史诗：LLM-THINK  
> 前置：多模型档案（`disable_thinking`）、P1 聊天（模型芯片，明确 defer 本能力）、Run SSE（`GET /v0/runs/{id}/stream`）、`openai_compatible` Chat Completions  
> 备注：**不**与 BLOB-CS / LOGIN-SKILL / MCP OAuth / UI-I18N / P6 Memory 合并；承接 P1 非目标「模型自身思考级别」

---

## 1. 背景与动机

今日只有档案布尔 `disable_thinking`，且仅在为真时向 DeepSeek 风格网关发送 `thinking: {type:"disabled"}`。模型调用是**整段返回**再发一条 `llm.message`，Chat 无法调低/中/高，也无法看见思考过程。

开源第一版要把思考做成一等能力：可调级别、能打到各家兼容协议、真流式打字机展示思考与正文，历史可回看。目的是让 Agent 过程可见、更有吸引力，而不是只多一个静默开关。

## 2. 已确认产品决策

| 议题 | 决策 |
|------|------|
| 控件位置 | **档案默认 + 聊天覆盖** |
| 聊天档位 | **默认（跟模型） / 关 / 低 / 中 / 高** |
| 覆盖寿命 | **本会话 sticky**（前端按 `conversationId` 记）；换对话回到「默认」 |
| 协议 | **内部统一档位**；按模型名 / Base URL **只发一种方言**；认不出则不发；档案可选手动覆盖 |
| 展示 | **真流式**：思考边到边打，再打正文 |
| 思考落库 | **落库**；历史可展开回看 |
| 渠道 | **不做**打字机；微信等仍收完整回复，只用档案默认级别 |
| 合并 | **独立史诗**，一次交付级别 + 流式 + 展示（不分期「先级别后展示」） |

## 3. 成功标准

1. 管理员可在「设置 → AI 模型」为每个档案设默认思考级别；运营在 Chat 可不改设置、按会话覆盖本轮级别。  
2. 对 DeepSeek / OpenAI 推理系 / 通义混合思考等常见网关，Baize 只发该网关认识的字段；普通非思考模型不因多余字段 400。  
3. 支持 stream 的上游：Chat 先打字机展示思考，再打字机展示正文；工具多轮时每轮 LLM 各自一块思考。  
4. 刷新或重进对话后，助手消息仍可展开当时的思考全文。  
5. 上游不支持 stream、不返回思考、或思考加密不可见时，有明确降级，不打断出字。

## 4. 范围

### 做

- 档案：`thinking_level`、`thinking_dialect`；迁移并逐步替代 `disable_thinking`。  
- `POST /v0/runs` 可选 `thinking_level`；省略则用档案默认。  
- `openai_compatible`：`stream: true`；方言映射；解析思考 / 正文 delta。  
- Run 事件：限频累计快照 `llm.thinking.delta` / `llm.content.delta`；每轮完整 `llm.thinking`；终态 `llm.message` 仍一条，带 `content` 与拼接后的 `thinking`。  
- 会话 assistant 消息可选 `thinking` 列；组装下一轮 prompt 时**不**把思考喂回模型。  
- Chat：思考芯片、实时打字机、历史折叠「查看思考」。  
- 设置页：级别四选一 + 高级「思考协议」。

### 不做

- 渠道侧流式 / 打字机。  
- 假流式（整段返回后再前端装打字机）作为主路径（仅作 stream 失败降级时无打字机）。  
- 完整 i18n。  
- 把思考块回灌下一轮模型上下文（含 Anthropic 式 thinking block 续写；后继）。  
- 每 token 落库。  
- 新 Provider 类型（仍只 `openai_compatible`）。  
- OpenAI Responses API；本史诗仍走现有 `/chat/completions`。

## 5. 架构草图

```
聊天选级别 → POST /v0/runs (thinking_level?)
                │
                ▼
         解析有效级别（覆盖 > 档案）
                │
                ▼
         方言映射 → stream chat/completions
                │
     ┌──────────┼──────────┐
     ▼          ▼          ▼
 llm.thinking  llm.content llm.message
 delta + 整轮     .delta    (最终一条；thinking 拼接)
     │            │          │
     └──── SSE ───┴──► 打字机 UI；thinking 写入会话消息
```

Engine 在每一轮 `Chat` 改为优先 `ChatStream`；失败则回退现有整段 `Chat()`，只发终态 `llm.message`。

## 6. 数据模型

### 6.1 级别与协议枚举

`thinking_level`：`off | low | medium | high`。

`thinking_dialect`：`auto | openai | deepseek | qwen | omit`。默认 `auto`。

聊天「默认」**不是**一个存库枚举值：前端不传 `thinking_level`。

### 6.2 模型档案

- 新增 `thinking_level`、`thinking_dialect`（API JSON 与 SQL 列同名）。  
- 迁移：`disable_thinking = true` → `thinking_level = off`；`false` → `medium`；`thinking_dialect = auto`。  
- 种子 YAML：若仍写 `disable_thinking`，按上表映射；新配置可写 `thinking_level` / `thinking_dialect`。  
- 读 API 继续返回 `disable_thinking`（`thinking_level == off`）直到实现计划标明的弃用提交；写 API 两者都收，同时出现时以 `thinking_level` 为准。

### 6.3 Run

`POST /v0/runs` 增加可选 `"thinking_level": "off"|"low"|"medium"|"high"`。非法值 400 `invalid_request`。

有效级别：请求有值则用之，否则档案 `thinking_level`。无档案（不应发 Run）走现有无模型错误。

Auto 路由：覆盖打在**实际选中的档案**上，再做方言映射。

### 6.4 会话消息

assistant 增加可选 `thinking`（TEXT，可空）与 `thinking_redacted`（布尔，默认 false）。列表/详情 JSON 带出。无 `thinking` 且非 redacted 则 UI 不显示入口。

历史重建优先用消息上的 `thinking`；若旧数据仅有事件，实现计划可用终态 `llm.message.data.thinking` 回填，不要求双写两套真相长期分叉——**权威是会话消息 + 终态事件**（二者在 Run 成功时应一致）。

### 6.5 事件

| type | data | 何时 |
|------|------|------|
| `llm.thinking.delta` | `{ "turn": N, "text": "<累计思考>" }` | 该轮思考累计变化；限频 |
| `llm.content.delta` | `{ "turn": N, "text": "<累计正文>" }` | 该轮正文累计变化；限频 |
| `llm.thinking` | `{ "turn": N, "text": "<该轮完整思考>", "thinking_redacted": false }` | 该轮思考阶段结束（进入正文、tool_call 或整轮结束前必须有一帧，可与最后一帧 delta 文本相同） |
| `llm.message` | `{ "content": "...", "thinking": "..." }` | **仍只在 Run 最终助手正文时发一条**（保持今日 fold 语义；`thinking` 为各 turn 按序拼接，供消息落库） |

`turn` 为本次 Run 内 0-based LLM 调用序号。Live UI 按 turn 把 `llm.thinking` 放在对应工具卡之上，不把多轮拼进同一块。工具轮**不**额外发 `llm.message`。

**限频：** 每种 delta 每种 turn 最多约 10 次/秒；轮结束前必须再发一帧，保证快照等于终态前缀。Hub 缓冲仅 16，实现须保证 AppendEvent 节奏不会把订阅者永久丢帧到「只能靠终态」——终态始终完整，UI 允许从终态补齐。

加密或上游只给不可展示摘要：`thinking` 空字符串，`thinking_redacted: true`。UI 一行「模型未返回可展示的思考」，不打字机空块。

## 7. 方言映射

内部级别为唯一产品语义。请求里**只放一种方言**的字段。

### 7.1 `auto` 推断（按顺序，命中即停）

1. `thinking_dialect` 非 `auto` → 用该值。  
2. Base URL 含 `openrouter.ai` → 发送 OpenRouter `reasoning` 对象（见 7.2「openrouter」；UI 无单独选项）。  
3. 模型名或 Base URL 含 `deepseek` → `deepseek`。  
4. 模型名或 Base URL 含 `qwen`、`dashscope`、`aliyuncs.com` → `qwen`。  
5. 模型名像推理模型（大小写不敏感子串：`o1`、`o3`、`o4`、`gpt-5`、`gpt-6`、`grok`、`gemini-2.5`、`gemini-3`、`r1`）→ `openai`。主机为 `api.openai.com` / `api.x.ai` / `generativelanguage.googleapis.com` **不能单独**升级为 openai：`gpt-4o` 等非推理名仍走第 6 步 `omit`，避免 400。  
6. 否则 `omit`（**不**给 GPT-4o / Llama 等发 `reasoning_effort`）。

强制 `openai` 时即使模型名不像推理模型也发 `reasoning_effort`（给自建网关）。强制 `omit` 永不发思考相关字段。

### 7.2 字段表

| 方言 | `off` | `low` | `medium` | `high` |
|------|-------|-------|----------|--------|
| `openai` | `reasoning_effort: "none"` | `"low"` | `"medium"` | `"high"` |
| `deepseek` | `thinking: {type:"disabled"}` | `thinking: {type:"enabled"}` | 同 left | 同 left |
| `qwen` | `enable_thinking: false` | `enable_thinking: true` 且 `thinking_budget: 1024` | `true` + `thinking_budget: 8192` | `true`，省略 budget |
| openrouter（仅 auto） | `reasoning: {effort:"none"}` | `{effort:"low"}` | `{effort:"medium"}` | `{effort:"high"}` |
| `omit` | 不发 | 不发 | 不发 | 不发 |

DeepSeek 无低中高粒度：`low`/`medium`/`high` 都是开启。部分 Gemini / Grok / GPT 拒绝 `none`：该次请求去掉 off 对应字段并当作无法关闭，Chat 不报硬错误（可打日志）；用户选「关」时仍可能看到思考块。

### 7.3 上游流式字段（解析）

按出现读取，拼进思考缓冲，**不**混进 `content`：

- `choices[].delta.reasoning_content`（DeepSeek 等）  
- `choices[].delta.reasoning`（字符串时）  
- OpenRouter `delta.reasoning`  
- 正文：`choices[].delta.content`（既有）

终态 message 上的同名字段作补齐。忽略未知字段。

## 8. Provider 与 Engine

- `llm.Provider` 增加可选流式能力（实现计划定具体接口名）：回调思考增量、正文增量、完整 `llm.Message`（含 `Thinking`）。现有 `Chat()` 保留给 mock、测试桩、降级。  
- `OpenAI`：`stream: true`；SSE 解析；组装 tool_calls 与今日非流式语义一致。  
- 上游拒绝 stream 或握手失败：同一 Run **该轮**改 `Chat()` 非流式；若响应带思考则写入终态，UI 无打字机。  
- 渠道出站：仍等 Run 成功后的 assistant `content`；忽略思考流。

思考正文**禁止**完整写入 info 日志；错误截断与现有 LLM 错误体策略一致（短）。

## 9. UI

### 9.1 发送栏

`ModelChip` 旁「思考」芯片：默认 / 关 / 低 / 中 / 高。只对**当前** `conversationId` 生效：同会话刷新用 `sessionStorage` 恢复；**换到另一会话时芯片回到「默认」且不恢复旧会话覆盖**。只读、无模型、发送中：禁用。

文案进 `strings.ts`。不展示 `reasoning_effort` 等协议名。

### 9.2 设置 → AI 模型

默认思考：关 / 低 / 中 / 高（不再用「禁用思考」主复选）。高级：思考协议 = 自动 / OpenAI / DeepSeek / 通义 / 不发送。

仅 admin 可改档案（现 ACL）。运营可在 Chat 覆盖。

### 9.3 实时

1. 收到该 `turn` 的思考 delta：展开「思考中」块，打字机追 `text`，块底光标。  
2. 同 turn 正文 delta 开始：思考块收成一行「已思考」（可点开全文）；正文块打字机。  
3. 无思考或 `thinking_redacted`：不出现思考打字机（redacted 一行说明）。  
4. 工具：思考块留在该轮工具卡之前；下一 `turn` 新开思考块。

打字机以 SSE 累计快照为准（set 全文，不是只 +1 字符），避免丢帧造成乱序；视觉仍是连续打出。

### 9.4 历史

助手气泡：有 `thinking` 或 `thinking_redacted` 则默认折叠「查看思考」；展开只读全文（不再打字机）。历史事件回放把各 turn 的 `llm.thinking` 接到对应工具卡之上（与 Live 同结构）。`foldEvents` 不因思考事件多冒助手气泡。

## 10. 风险与依赖

- 各网关字段不一致 → 方言表 + 手动覆盖 + `omit` 兜底。  
- 严格网关 400 → 该轮降级非流式；不在同一请求里双方言齐发。  
- 事件体积 → 累计快照限频，不做每 token 行。  
- Hub 缓冲 16 → 终态完整；UI 用终态补齐。  
- SSE 现以 store 为源 → delta 必须 `AppendEvent`（跨副本 nudge 才能读到）。  
- 部分模型关不掉思考 → 不硬失败。

## 11. 测试与验收（规格级）

- 方言表单测：DeepSeek / OpenAI 推理名 / 通义 / 未知 omit / 强制 openai / OpenRouter URL。  
- `disable_thinking` → `thinking_level` 迁移。  
- 流式桩：思考 delta → 正文 delta → `llm.message` 含完整 thinking；消息 GET 带 `thinking`。  
- 省略 Run 级别用档案默认；显式覆盖只影响该 Run。  
- Chat 不传级别；换 `conversationId` 芯片回到默认。  
- 运营不能 PATCH 档案级别；能看 Chat 思考。  
- 无 stream / 无思考字段：与今日一致（整段 `llm.message`，无思考入口）。  
- 前端：实时折叠节奏；历史展开；redacted 文案。

## 12. 后继（非本批）

- 渠道打字机或渠道展示思考摘要。  
- 思考块回灌多轮 tool（Claude `thinking` blocks）。  
- 完整 i18n。  
- 会话覆盖落库（多设备同步）。  
- delta 事件压缩（Run 结束后删除中间快照，只留终态）。

## 13. 文档与账本

- 账本 / 确认清单：LLM-THINK 从「可开刀（先规格）」改为 **范围已确认**。  
- P1 非目标「模型自身思考级别」由本文承接。  
- README 在实现交付后补一句思考级别与可见思考过程。
