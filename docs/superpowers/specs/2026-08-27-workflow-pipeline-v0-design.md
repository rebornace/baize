# 工作流线性流水线 v0 设计规格

> 状态：已批准（2026-08-27）
> 定位：兑现架构 §6「可选工作流 DSL」开源承诺的最小一刀。刻意做成**线性流水线**而非通用状态机——需要智能判断的场景本就该用 ReAct；workflow.yaml 只承载确定性步骤序列，与项目「开箱即用、易插拔」定位对齐。

## 1. 目标与非目标

**目标**

1. Skill 包可附带可选 `workflow.yaml`（与 SKILL.md 同目录），定义线性工具流水线。
2. Run 激活该 skill 后按序执行步骤，全程不调 LLM；未激活/无 workflow 的 skill 走既有 ReAct，行为不变。
3. 步骤审批复用既有 HITL 机制（`approve: true`），UI 审批卡片零改动。
4. 新增三个 workflow 事件，前端折叠条最小渲染。
5. API 零新增端点。

**非目标（v0 明确不做）**

- `branch` / 条件跳转 / 循环（需要分支 = 用 ReAct）。
- `wait_human` 独立节点类型（审批由 `approve: true` 表达）。
- `llm` 节点类型。
- 表达式语言：无比较运算符、无函数。
- workflow 可视化编辑器。

## 2. 文件形态与全量语法

```
examples/skills/ticket-triage/
├── SKILL.md          # 既有
└── workflow.yaml     # 新增（可选）
```

```yaml
name: ticket-triage
steps:
  - id: fetch
    tool: search_tickets
    args:
      query: "{{input.topic}}"
  - id: reply
    tool: reply_ticket
    approve: true
    args:
      text: "{{fetch.result.summary}}"
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | ✅ | 工作流名（须等于包名） |
| `steps[].id` | ✅ | 包内唯一 |
| `steps[].tool` | ✅ | 已注册工具名 |
| `steps[].args` | ❌ | 任意嵌套 map/slice/标量，含 `{{引用}}` |
| `steps[].approve` | ❌ | true 时先过 HITL |

语法总量：5 个字段 + 一种占位符。校验错误在激活时报出（文件名 + 行语义位置）。

## 3. `{{引用}}` 语义

心智模型一句话：**`{{路径}}` = 从数据树取值**。

| 占位符 | 取值 |
|--------|------|
| `{{input.<path>}}` | Run 初始输入对象下钻 |
| `{{<step_id>.result}}` / `{{<step_id>.result.a.b}}` | 该步工具返回 content map 下钻；数组用数字下标 `.0.` |

规则：

1. **整值替换保类型**：占位符为字符串全部内容且命中非字符串 → 保留原生类型（number/bool/map/slice 直接作为参数值）。
2. **子串替换**：占位符为字符串一部分 → 与前后文本拼接为 string。
3. **递归渲染**：args 的 map/slice 每个元素同样处理。
4. **路径缺失 → fail-fast**：run failed，错误含 step id 与路径，如 `step "reply": path fetch.result.summary not found`。
5. **id 唯一**：文件内唯一；线性顺序天然无环。

## 4. 执行模型

- **模式判定**：Run 期间 LLM 调 `activate_skill` 激活某 skill 时，若该包带合法 `workflow.yaml` → 本 run 进入 DSL 模式（激活时一次性确定，run 内不切换）；否则维持 ReAct。
- **执行器**：新 `internal/workflow` 包——加载/校验/渲染/顺序执行。每步：
  1. AppendEvent `workflow.step_started`
  2. 若 `approve:true` → 复用 HITL（见 §5）
  3. `Tools.Invoke`（沿用 toolTimeout）
  4. AppendEvent `workflow.step_completed`
  5. 结果记入结果表供后续 `{{引用}}`
- 任一步失败（Invoke 错误或路径缺失）→ run failed，错误含 step id；已有事件保留可回溯。
- skills overlay 对工具可见性的过滤照常生效：workflow 引用不可见工具 → 校验期失败。

## 5. HITL 审批

- `approve: true` 步骤进入前：`SetHITL(HITLPayload{Prompt: "workflow step \"<step_id>\": <tool>", ToolName: <tool>, Arguments: 渲染后 args})`，随后 BeginWait/awaitHITL 既有流程。
- 批准 → Invoke 该步。
- 拒绝 → run failed，error = `workflow step "<step_id>" rejected`。
- 冷恢复：沿用 `ContinueFromHITL`——批准后从中断步骤继续，已完步骤不重放（结果表从事件流重建：重放 `workflow.step_completed` 事件）。

## 6. 事件

| event type | data | 说明 |
|------------|------|------|
| `workflow.started` | `{skill, steps: [id...]}` | 进入 DSL 模式时发一次 |
| `workflow.step_started` | `{step, tool}` | 每步开始 |
| `workflow.step_completed` | `{step, is_error}` | 每步结束 |

- 既有 `llm.tool_call` / `tool.result` / `hitl.*` 照发不变。
- SSE 断线续传天然兼容（AppendEvent 既有通道）。

前端：`foldEvents.ts` ChatBlock 联合类型扩展三种 block，折叠条渲染「流水线 fetch ✓ · reply ⋯」进度样式；exhaustive switch 同步扩展。

## 7. 加载与 API

- `internal/skill` 的 Catalog 加载包目录时顺带读取可选 `workflow.yaml`（yaml.v3 已有），解析结果挂到 Package。
- **API 零新增端点**：无独立触发口；是否走 DSL 由 run 内 skill 激活决定。

## 8. 测试策略

- 单测：workflow 解析校验（非法字段/重复 id/name 不匹配）、模板渲染五规则各一例、引用缺失报错文案。
- 引擎测：DSL 模式切换（激活带/不带 workflow）、顺序执行与结果表、步骤失败终止、approve 流程批准/拒绝、冷恢复重建结果表续跑。
- 集成测（tests/integration）：mock-ticket 式样例 skill + workflow.yaml 全链路（SSE 断言三个 workflow 事件序列）。
- 前端：foldEvents 新 case 快照断言。

## 9. 示例与文档

- `examples/skills/ticket-triage/workflow.yaml` 示例更新或新增一个纯流水线示例包。
- README 中英各一句：Skill 包可附 `workflow.yaml` 定义确定性流水线，语法一分钟上手；分支类需求用 ReAct。
- 架构文档 §5 Agent 运行补一小节。

## 10. 文件清单

| 文件 | 动作 |
|------|------|
| `internal/workflow/{model.go,parse.go,template.go,exec.go}` + 各 `_test.go` | 新建 |
| `internal/skill/package.go` / `catalog.go` | 读 workflow.yaml 挂 Package |
| `internal/run/engine.go` / `skills_overlay.go` | 激活切模式 + 执行入口 |
| `internal/store/store.go`（如需结果表持久化则事件重放即可，不加存储字段） | 无改动 |
| `web/chat/src/foldEvents.ts` + test | 三种 block |
| `examples/skills/...` 示例 | 新增示例 workflow |
| README.md / README.zh-CN.md / docs/architecture-and-plugin-protocol.md | 一句话说明 |
