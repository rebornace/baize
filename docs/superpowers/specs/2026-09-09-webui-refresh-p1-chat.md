# WebUI 体验改版 P1：聊天重构（默认说人话 · 技术按需展开）

> 状态：已交付（2026-09-13 账本对齐）
> 日期：2026-09-09
> 上游：`2026-09-09-webui-experience-refresh-design.md` 第 6/7/10 节
> 范围：P0 地基已落地后的 **P1 聊天重构，一次全做**；纯前端（`web/chat`），零后端契约改动。

## 1. 背景与目标

P0 已交付明暗设计令牌、样式拆分、基础组件（Button/Card/Badge/Spinner）、主题切换、移动端抽屉与局域网白屏修复。聊天核心仍是"技术态"：

- 工具卡直接显示技术工具名 + 入参/出参 JSON；HITL 显示 `waiting_human` 式按钮；workflow 显示裸步骤 id。
- 模型选择是独立行下拉框，档位文案为"轻量/标准/强力"，且**每次发送后强制重置回 Auto**。
- 消息操作平铺 `编辑并回滚 / 重新生成 / 回滚到此 / Fork` 等技术按钮。
- 欢迎语含"粘贴临时 Token"；错误是裸红字；删除对话走原生 `window.confirm`。
- 回看历史对话时，工具/流程步骤丢失（事件其实已拉回前端，却只提取了分析页链接）。

目标：聊天体验对普通人可读，技术细节默认收起、按需展开；同时不丢失任何专业能力与可调试性。

### 非目标

- 不改任何后端路由、字段、角色语义（后端已在发结构化错误 `{error:{code,message}}`、工具目录已带 `title`、resume 已支持 `comment`，本轮只在前端消费）。
- 不做"模型自身思考级别"（低/中/高、thinking 开关）——这是模型运行时参数，需扩后端，另立规格。
- 不改设置页（P2/P3）：设置页里的 `window.confirm` 本轮不动。
- 不引组件库、不做 i18n、不做 Playwright E2E。

## 2. 关键事实（已核实，作为设计依据）

- 后端错误统一为 `writeError(w,status,code,message)` → JSON `{"error":{"code","message"}}`；前端 `parseJSON` 目前**丢弃 code**，只抛 `HTTP <status>: <message>`。
- `/v0/tools` 返回 `ToolInfo[]`，含可选 `title` / `description`；可建立 `name -> {title,description}` 友好名映射。
- `resumeRun(runId, decision, comment='')` 已支持驳回留言。
- 后端持久化完整 run 事件（`llm.tool_call` / `tool.result` / `hitl.*` / `workflow.*`）；前端看历史时已 `listEvents(runId)`（`ChatPage.tsx` 约 397 行），但仅 `extractAnalysisPagesFromEvents`，工具/流程事件被丢弃。
- `ChatMessage`：`{id, conversation_id, role:'user'|'assistant'|'system_note', content, run_id?, created_at}`；助手历史消息带 `run_id`。
- 现状发送后重置：`ChatPage.tsx:545 setSelectedModelId('')`。
- 内部路由档位 id 为 `light / standard / power`；Auto 哨兵 id 为 `auto`（`AUTO_MODEL_ID`）。

## 3. 方案与范围

采用**自底向上、反馈层先行、纯前端 TDD 分层推进**（上游设计第 10 节顺序）。一次交付以下五块：

1. 错误与文案基础层（结构化错误 + `strings.ts`）。
2. 反馈/浮层基础组件（Toast / Modal / ConfirmDialog / DropdownMenu）。
3. 工具 / HITL / Workflow 人话化（实时 + 历史）。
4. 模型芯片 + 消息操作收纳 + 克制欢迎区 + 模型选择持久化。
5. 接线、测试、重建嵌入产物。

## 4. 详细设计

### 4.1 错误与文案基础层

**结构化错误（扩展，不破坏现有判断）**

- 新增前端错误类 `ApiError extends Error`，字段：`status:number`、`code:string`、`message`（人类可读原始 message）。
- `parseJSON` 在 `!res.ok` 时读取 `body.error.code` / `body.error.message`，抛 `ApiError`；其 `message` 保留 `HTTP <status>: <detail>` 前缀形态，保证现有 `GateRoot` 的 `startsWith('HTTP 401:')` 判断继续成立。
- 网络层失败（`fetch` reject，无 response）单独包装为 `ApiError`，用稳定 code `network_error`、status=0。
- 提供类型守卫 `isApiError(e)` 与读取器 `errorCode(e)`、`errorDetail(e)`。

**错误码 → 中文映射（`strings.ts`）**

- 纯函数 `friendlyError(e): { title: string; detail?: string }`：优先按 `code` 映射；未知 code 给通用人话 title，`detail` 放原始 `code/message`，由 UI「详情」展开。
- 首批映射（可在实现中按后端实际 code 增补，未知一律安全兜底）：
  - `network_error` → 网络连接失败，请检查服务是否正在运行。
  - 401/403 → 没有访问权限或登录已失效，请重新解锁后再试。
  - `vision_unsupported` → 当前模型看不了图片；改用「智能选择」或带「能看图」的模型。
  - `conversation_busy` → 上一条还在处理中，请稍候再发。
  - `no_model_configured` → 还没有可用的 AI 模型，请到「设置 → AI 模型」添加一个。
  - `invalid_signature` → 连接校验未通过，请刷新页面后重试。
  - 5xx / `internal_error` → 服务暂时出了点问题，请稍后重试。
  - 404 / `not_found` → 内容不存在或已被删除。

**文案集中**

- 所有 P1 新增/改动的展示文案（档位、能力、动作、HITL、欢迎语、高级项、错误）集中在 `strings.ts` 导出的常量/函数；技术 id 不出现在文案层。
- 档位对外更名：`light→快速`、`standard→标准`、`power→深度思考`（**仅展示**，内部 id 与后端不变；同步修改 `tierLabel`）。
- 能力标签 `视觉 → 能看图`；Auto 标签 `智能路由（Auto） → 智能选择`。

### 4.2 反馈与浮层基础组件（`components/ui/`）

全部使用 P0 令牌、`lucide-react`、焦点可见、键盘可达、明暗双主题；交互测试用 `jsdom`，不引 testing-library。

- **Toast**：`useToast()`（`toast({tone:'success'|'error'|'info', title, detail?})`）+ `<ToastRegion toasts=.../>`，容器 `aria-live="polite"`。自动消失：info/success 4s，error 6s；可手动关闭；最多同时 3 条。
- **Modal（最小通用底座）**：标题、正文、操作区、开关控制；Esc 关闭、点遮罩关闭、焦点陷阱、打开时焦点入弹窗。仅服务 ConfirmDialog 与「错误详情」，不预置复杂表单能力。
- **ConfirmDialog**：基于 Modal；标题 + 后果说明 + 「取消 / 确认」；危险操作确认键用 danger 样式，**初始焦点落在「取消」**；返回 Promise<boolean> 或受控 `onConfirm/onCancel`。
- **DropdownMenu**：`useDropdown()` + 渲染层；点外关闭、Esc 关闭、`↑/↓` 循环、Enter 激活、`aria-expanded/menuitem/menuitemradio`；锚点对齐；触摸目标 ≥40px。供消息「⋯」菜单与模型芯片选择复用。

### 4.3 工具 / HITL / Workflow 人话化

**友好名（渲染层映射，纯函数可测）**

- 新增 `friendlyTool.ts`：`friendlyToolName(name, catalog)` 有 `title` 用 title，否则回退技术名；动作措辞由一张"进行时/完成态"短语表 `toolPhrase(title|name, status)` 给出，查表不到时用通用模板（进行中「正在处理：{name}…」、成功「已完成：{name}」、失败「处理失败：{name}」），不在代码里对任意工具名硬拼动词，避免别扭中文。
- `ChatPage` 进入时拉取一次 `listTools()` 建目录（失败/为空则目录为空，全部回退技术名，不阻塞聊天）；通过 props/context 注入卡片，不依赖模块级可变状态。

**ToolCard 重写**

- 折叠态一行：状态图标（running 旋转 / succeeded ✓ / failed ✕）+ 动作短语（如"正在查询订单系统…"），右侧「详情」。
- 「详情」展开：技术工具名、入参 JSON、出参 JSON（保持现有 `<pre>`）；分析页预览维持"折叠也可见"。
- 状态不再裸显示 `succeeded/failed`；失败在短语处体现并可在详情看错误。

**HITL 审批卡**

- waiting 态渲染醒目卡：标题「需要你确认」+ 要做什么（取 title/description），不出现 `waiting_human`。
- 操作「同意 / 拒绝」；**仅「拒绝」提供选填「留言」**（默认收起的输入框 → `resumeRun(..., 'reject', comment)`）。
- 提交中按钮 loading/禁用；失败 Toast；决议后转结果态「已同意 / 已拒绝」。保留进入 waiting 自动展开参数。
- **历史回看**中的 HITL 卡为只读结果态，不渲染操作按钮。

**WorkflowCard 重写**

- 显示「第 n / 共 m 步」与当前步骤；步骤中文名按 id 从目录/技能元数据映射，取不到回退「步骤 1、步骤 2…」序号，不显示裸事件 id；保留 pending/running/done/failed 四态视觉。

**历史完整渲染（关键新增）**

- 新增纯函数（建议 `foldToolBlocks(runId, events)`，或在 `foldEvents` 增加 `assistantText:false` 选项）：从持久化事件**只折叠工具/流程块**，剔除 `llm.message`（避免与已持久化的助手文本重复）。
- `ChatPage` 在现有按 `run_id` 拉 `listEvents` 的循环里，同时把工具/流程块按 `run_id` 存入新状态 `historyBlocks: Record<runId, ChatBlock[]>`。
- 渲染：在该 `run_id` 对应的助手消息位置插入这些块（工具/流程用现有 ToolCard/WorkflowCard，只读、无 HITL 操作）；**同一 `run_id` 的块只渲染一次**，挂在最早一条带该 `run_id` 的消息处，避免多条助手消息共享 run 时重复。分析页行为不变；拉取失败静默降级纯文本。

### 4.4 模型芯片、消息菜单、欢迎区、模型持久化

**模型芯片（`ModelSelect.tsx` 改造为芯片）**

- 输入框左下角一枚小芯片，默认「智能选择」；点击用 DropdownMenu 弹出选项，标注档位（快速/标准/深度思考）与「能看图」。
- 手动选中后芯片显示模型简称；无模型时芯片为「添加模型」，点击跳 `/settings/models`。
- 发图片但当前选择不能看图：沿用 `visionGate` 在发送前**同步拦截**，**不自动改路由**；用单按钮警示 Modal（标题 + `visionGate.message` + 「知道了」，不发送、不关闭输入），由用户自行换模型/切智能选择/移除图片。不用 Toast（承载不下且需明确阻止本次发送）。

**模型选择持久化（改变现有行为）**

- 移除 `ChatPage.tsx:545` 发送后 `setSelectedModelId('')`。
- 选中态写 `localStorage`（键如 `baize.model_choice`），初始化时读取；刷新、重开浏览器、开新对话都保留，直到用户主动切回「智能选择」。
- 仅本设备 WebUI，不随会话/后端持久化。启动时若所选具体模型已不存在，回退智能选择并 Toast 轻提示。

**消息操作收纳**

- 常驻高频：助手「复制 / 重新回答」；用户「编辑后重新回答 / 复制」。
- 低频进「⋯」DropdownMenu：「复制成新对话」（原 Fork）、「回到这里」（仅 system_note，原回滚到此）。
- 文案改人话；复制成功 Toast；显示门控（持久化消息、非忙碌、非实时 run）逻辑不变。
- 左侧对话列表删除按钮改用 `ConfirmDialog`（危险、说明后果、删除成功 Toast）；消息体不放删除。

**欢迎区与高级项**

- 空会话只保留：主标题「有什么可以帮你？」+ 一句克制产品定位副标题；无示例问题、无 demo 提示、无 Token 措辞。
- 「临时用户 Token」「Run Webhook URL」保留在「高级」折叠区，默认收起；标签改人话（如「临时访问凭证（选填）」），技术解释以小字放其下。

**状态/错误呈现**

- 发送与运行错误由裸红字改为 Toast（走 4.1/4.2）；输入区上方保留轻量状态行（如「正在思考…」「第 n 步…」），去除技术措辞。

## 5. 工程结构

- 新增：`strings.ts`、`apiError.ts`（或并入 `api.ts`）、`friendlyTool.ts`；`components/ui/`：`Toast*`、`Modal*`、`ConfirmDialog*`、`DropdownMenu*`（含 hooks）。
- 改动：`api.ts`（ApiError）、`modelSelect.ts`（档位/能力/Auto 文案、去"每次重置"注释）、`foldEvents.ts`（历史只取工具/流程块）、`ToolCard.tsx`、`WorkflowCard.tsx`、`ModelSelect.tsx`（芯片）、`Composer.tsx`、`ChatPage.tsx`（ToastRegion、ConfirmDialog、溢出菜单、欢迎区、模型持久化、historyBlocks）、相关样式（令牌化，必要时新增 `styles/chat.css`）。
- 依赖：不新增（`lucide-react` P0 已装）。完成后 `npm run build` 重建 `internal/ui/dist` 并随提交嵌入。

## 6. 测试与验收（TDD）

- 纯函数：`friendlyError`（已知 code / 未知 / network / 401）、`friendlyToolName` 回退、历史只折叠工具/流程块且文字不重复、档位与动作文案。
- 组件（jsdom）：Toast 自动消失/关闭/aria-live；ConfirmDialog 的 Esc/点遮罩/初始焦点/危险确认；DropdownMenu 点外/Esc/方向键/Enter；模型芯片选择与 localStorage 持久化、失效模型回退；HITL 同意/拒绝 + 仅拒绝留言；历史块只读无操作。
- 现有用例：受文案/结构影响者改为断言行为与稳定 `data-testid`/角色，不绑定易变中文。
- 验收命令：`npm test` 全绿、`npx tsc --noEmit` 零错、`npm run build` 成功。
- 手动走查：明/暗 × 桌面/手机；实时与历史工具步骤、HITL、workflow、图片/附件、删除对话、复制/Fork/回滚、模型选择持久化、键盘操作与对比度。
- 后端：无改动，Go 测试保持绿；仅重建并提交嵌入 `dist`。

## 7. 风险与对策

- 历史块与助手文本重复/错位：折叠时剔除 `llm.message`，按 `run_id` 精确挂到对应消息；用纯函数单测固化顺序与去重。
- 工具目录缺失导致空白：一律回退技术名（仅详情可见），不阻塞。
- 模型持久化选中已删除模型：启动校验回退 Auto + Toast。
- 回归面大：分层提交、每层测试绿；不改路由/id/后端契约；设置页 confirm 留待 P2/P3，避免越界。
- 过度友好损失专业度：技术名/JSON 全部保留在「详情」，仅默认收起。
