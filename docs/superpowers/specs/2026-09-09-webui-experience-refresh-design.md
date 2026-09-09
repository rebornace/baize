# WebUI 体验改版设计（清爽现代 · 零术语 · 暗色 · 移动端）

> 状态：设计稿（仅私有仓 `docs/superpowers`，不导出公开仓）
> 日期：2026-09-09
> 方法：Superpowers 头脑风暴 + 视觉伴侣（用户已逐节认可方向）

## 1. 背景与问题

白泽 WebUI（`web/chat`，React 19 + React Router 7 + Vite 6 + 手写 `style.css`，打包后经 Go `//go:embed` 进二进制）当前存在两类体验问题：

1. **不美观、交互生硬**：零组件库、零图标库（除 `qrcode`），`style.css` 已 1329 行且缺少统一设计令牌；视觉层级、间距、反馈、空/载/错状态粗糙；大量使用浏览器原生 `confirm`/`alert` 和裸红字报错。
2. **技术术语堆砌，普通用户无法使用**：设置导航平铺 `Tools / OpenAPI / Skills / MCP / MCP 导出 / 插件 / Webhook / Inbox / 渠道 / 模型 / 存储 / 运行时`；聊天里直接暴露「智能路由 Auto」「Fork」「编辑并回滚」「回滚到此」、工具调用名 + JSON；欢迎语出现「粘贴临时 Token」。

## 2. 目标与非目标

### 目标
- **视觉**：清爽现代（方向 A）——充足留白、克制的蓝、适中圆角，贴近主流 AI 助手，专业且长期耐看。
- **零术语**：聊天和**所有设置页**对普通人可读；技术细节默认隐藏、按需展开。
- **暗色模式**：默认跟随系统，可手动切换并记忆。
- **响应式**：桌面 / 平板（≤1024）/ 手机（≤768）均可用。

### 非目标
- **不改任何后端 API 契约**：所有路由路径、字段名、角色权限语义保持不变（现有书签/链接不失效；内部 id 如 `tools/openapi/mcp` 保留，仅展示层改名）。
- 不引入重型组件库（Ant 等）、不做框架迁移。
- 不增删设置页的功能字段，只做重组、改名、加说明、改善交互。
- 不做 Playwright E2E（沿用既有决策 U2）。
- 不做多语言/i18n 框架（本轮只做中文人话；代码集中文案便于将来国际化）。

## 3. 设计语言（令牌驱动）

### 3.1 设计令牌
- 在 `<html data-theme="light|dark">` 上定义两套 CSS 自定义属性（令牌），全站只引用令牌、不写死色值：
  - **中性色阶**：背景 / 表面 / 表面次级 / 边框 / 文本 / 文本弱化。
  - **主色**：沿用蓝 `#2563eb` 体系，补充 hover/active/软底色（主色 8–12% 透明）。
  - **语义色**：成功、警告、危险、信息，各带软底色。
  - **间距**：4px 基准（4/8/12/16/24/32）。
  - **圆角**：控件 8–10、卡片 14、气泡 12–16、胶囊 999。
  - **阴影**：三档（细边、浮层、弹窗）；暗色下降阴影、改描边。
  - **字号阶**与行高；**过渡** 120–200ms ease；z-index 分层令牌。
- 现状 `style.css`（1329 行）拆分为：
  - `styles/tokens.css`（明/暗令牌）
  - `styles/base.css`（reset、排版、全局）
  - `styles/components.css`（基础组件类）
  - `styles/layout.css`、`styles/chat.css`、`styles/settings.css`（页面级）
  - 由单一入口 `main.tsx` 引入；逐页把硬编码色值替换为令牌。

### 3.2 图标
- 引入 **`lucide-react`**（tree-shaking 按需，bundle 增量小），替换现有 emoji/裸字符（✕ 等），统一 16/18/20px、`currentColor`。

### 3.3 暗色模式
- 默认跟随 `prefers-color-scheme`；侧栏/设置提供「浅 / 深 / 跟随系统」分段开关。
- 选择存 `localStorage`（如 `baize.theme`），首屏内联小段脚本或尽早设置 `data-theme`，避免闪烁（FOUC）。
- 纯前端，零后端改动。

### 3.4 响应式断点
- 手机 ≤768px：左侧对话栏与设置导航变为**抽屉**（汉堡按钮 + 遮罩，选后自动收起）；设置卡片单列；输入区贴底、安全区适配；触摸目标 ≥40px；模型芯片/附件按钮不挤占输入。
- 平板 769–1024px：侧栏收窄、设置卡片两列。
- 桌面 >1024px：维持现有双栏，设置卡片三列。

## 4. 信息架构

### 4.1 路由（全部保留，新增首页）
- `/` 聊天（不变）。
- `/settings`：**新增「人话卡片」首页** `SettingsHome.tsx`，作为 admin 默认落地页（`SettingsIndex` admin 由重定向 `tools` 改为渲染首页）。
- 现有子路由（`tools/openapi/skills/identities/mcp/mcp-export/plugins/webhooks/inbox/channels/weixin/models/storage/runtime`）路径**全部不变**。

### 4.2 设置首页（人话卡片，方案一）
- admin 进入设置先看到按「要做什么」分组的卡片网格；每卡：图标 + 人话名 + 一句用途说明 + **当前状态徽标**（数据驱动，如「微信：未登录/运行中」「已接 N 个业务系统」「N 个模型」），点击进入对应现有页。
- 分组（五组）：
  1. **助手**：AI 模型、技能与流程。
  2. **能力**：能做什么、已登录账号。
  3. **对接**：业务系统接口、外部工具服务、自定义插件、对外提供能力。
  4. **消息**：微信、消息入口、事件推送。
  5. **数据与系统**：数据存储、高级设置。
- 桌面端在卡片首页之外，设置左栏保留**分组后的人话菜单**（同五组），便于管理员快速直达。
- **operator**：设置区仍只有「已登录账号」（权限与现状一致），其首页即单卡/直达该页。

### 4.3 导航数据源
- 扩展 `settingsNav.ts`：每项含 `{ to, label（人话）, group, icon, desc, adminOnly, status? }`；菜单分组与卡片首页共用同一数据源，避免两处漂移。保留 `never` 穷尽检查。

## 5. 术语翻译表（集中到 `strings.ts`）

所有展示文案集中管理（含错误码映射、档位、模型标签），内部技术 id 不出现在标题/按钮（技术名仅在「详情/高级」里出现）。

| 现状（技术词） | 展示（人话） |
|---|---|
| Tools | 能做什么（功能开关） |
| OpenAPI | 业务系统接口（按接口文档接入） |
| MCP | 外部工具服务 |
| MCP 导出 | 对外提供能力 |
| 插件（HTTP 插件） | 自定义插件 |
| Webhook（出站） | 事件推送 |
| Inbox | 消息入口 |
| 渠道 / weixin | 微信 |
| 模型 | AI 模型 |
| 智能路由（Auto） | 智能选择（默认） |
| 档位 light / standard / power | 轻快 / 标准 / 强劲 |
| supports_vision「视觉」 | 能看图（标签） |
| 存储 | 数据存储 |
| 运行时 | 高级设置 |
| Fork | 复制成新对话 |
| 编辑并回滚 | 编辑后重新回答 |
| 回滚到此 | 回到这里 |
| 工具调用 `query_order` + JSON | 友好步骤「正在查询订单系统…」；「详情」展开技术名与入参/出参 |
| HITL waiting_human | 需要你确认（同意 / 拒绝 / 留言） |

- 工具友好名优先取工具已有的 `title`，无 title 再回退技术名（仅在详情内）。
- 各设置页内的字段级术语（如 `base_url` / `api_key_env` / `context_tokens` / `outbox` / HMAC / Secret 等）同步补中文标签 + 一句白话说明；高级/危险字段折叠在「高级」区。

## 6. 聊天体验（默认说人话）

- **模型选择**：从独立行收敛为输入框内**小芯片**「🧠 智能选择」（默认智能选择）；点击弹出选择，具体模型显示「轻快/标准/强劲」「能看图」标签；行为沿用现有「选择不记忆、发送后重置」。
- **工具/流程可视化**：
  - 工具调用折叠为友好步骤行（旋转图标 + 「正在查询订单系统…」+ 完成态 ✓/失败态），点击「详情」展开技术工具名、入参、出参、耗时。
  - HITL：渲染「需要你确认」卡片（同意/拒绝 + 意见输入），不出现 `waiting_human` 字样。
  - workflow：显示「第 n/共 m 步」与步骤中文名，而非裸事件。
- **消息操作**（常驻只留高频、低术语）：
  - 助手消息：复制、重新回答。
  - 用户消息：编辑后重新回答、复制。
  - **「复制成新对话（原 Fork）」「回到这里（原回滚到此）」等低频/技术项收进「⋯」溢出菜单。**
- **欢迎区（空会话）**：**只保留克制、专业的一句问候/产品语**。
  - **明确不提供示例问题 / prompt 模板 / demo 类提示**（避免显得像 demo、不专业）。
  - **不出现** 「粘贴临时 Token」等技术措辞。
- **错误与确认**：
  - 裸红字错误 → 顶部 **Toast / 横幅**，展示人话；建立后端错误 `code → 中文` 映射（如 `vision_unsupported`、`conversation_busy`、`no_model_configured`、`invalid_signature` 等），未知错误提供「详情」展开原始信息。
  - 删除对话等危险操作的原生 `window.confirm` → 自定义 **ConfirmDialog**（说明后果、危险按钮样式）；删除成功给 Toast。

## 7. 自建轻量基础组件（不引组件库）

新增 `web/chat/src/components/ui/`：
- `Button`（variant: primary/secondary/ghost/danger；size: sm/md；loading/disabled）
- `IconButton`、`Card`、`Badge` / `StatusDot`
- `Modal` / `ConfirmDialog`
- `Toast`（含 `useToast`/轻量容器，自动消失 + 手动关闭 + aria-live）
- `EmptyState`、`Spinner` / `Skeleton`
- `DropdownMenu`（「⋯」溢出菜单，键盘可达、点外关闭）
- `Switch`、`Field` / `Input` / `Select` / `Textarea`（统一标签、说明、错误态）
- `Tooltip`、`PageHeader`（标题 + 副标题 + 操作区）、`Drawer`（移动端抽屉）、`Segmented`（主题/分段选择）

可访问性：焦点可见、键盘操作、`aria-*`、对比度满足正文 ≥4.5:1。

## 8. 工程结构

- 新增/改动：
  - 新增 `styles/*.css`（拆分自 `style.css`）。
  - 新增 `components/ui/*`、`strings.ts`（文案与错误映射）、`useTheme.ts`。
  - 新增 `pages/SettingsHome.tsx`；`pages/SettingsLayout.tsx` 改为分组导航 + `<Outlet/>`。
  - 扩展 `settingsNav.ts`（分组/图标/说明/状态）。
  - 改 `pages/ChatPage.tsx`、`components/Composer.tsx`、`components/ModelSelect.tsx`、`components/ToolCard.tsx`、`components/WorkflowCard.tsx` 等；逐设置页替换术语与基础控件。
- 依赖：仅新增 `lucide-react`；React 19 / RR7 / Vite 6 版本不动。
- 构建与嵌入：`npm run build`（`tsc && vite build`）→ `internal/ui/dist` 由 Go `//go:embed` 打进二进制的流程不变；完成后重建 `dist`。

## 9. 测试与验收

- **前端单测（vitest）更新/新增**：
  - 受文案/结构影响的现有用例（`settingsNav`、`ModelSettings`、`ChatPageModelSelect`、`Composer` 等）改为断言行为与稳定的 `data-testid`/角色，不绑定易变中文文案。
  - 新增：导航分组与 admin/operator 可见性、设置首页卡片跳转与状态徽标、主题切换与持久化、工具友好名折叠/详情展开、错误码中文映射、ConfirmDialog/Dropdown 键盘交互。
  - 保证 `npm run test` 全绿、`tsc --noEmit` 零错误、`npm run build` 成功。
- **手动走查**：明/暗双主题 × 桌面/平板/手机三宽度；聊天（含工具调用、HITL、workflow、图片/附件、删除对话）与全部设置页；键盘与对比度。
- **后端**：不改契约，Go 测试应保持全绿；仅在重建嵌入 `dist` 后提交产物。
- **gofmt**：本次不动 Go 代码逻辑；若仅重建 `dist`，无 Go 格式化问题。

## 10. 建议落地顺序（交由 writing-plans 细化为 TDD 任务）

- **P0 地基**：令牌 + CSS 拆分 + 基础组件库 + 暗色/响应式骨架 + `lucide-react`。
- **P1 聊天重构**：模型芯片、友好工具卡（可展开）、HITL/workflow 人话、消息操作收纳、克制欢迎区、Toast/ConfirmDialog、错误映射。
- **P2 设置 IA**：`SettingsHome` 卡片首页 + 分组导航 + 状态徽标 + 术语集中。
- **P3 逐页人话化**：AI 模型、微信、能做什么、数据存储优先；其余设置页字段标签/说明/高级折叠。
- **P4 收尾**：移动端与暗色全面走查、可访问性、测试补齐、重建 `dist`、必要的 README/截图更新。

## 11. 风险与对策

- **回归面广（60+ 文件、14 设置页）**：分阶段提交，P0 先换令牌/控件不改结构；每阶段保持测试与构建绿。
- **暗色遗漏导致硬编码色**：以 lint/评审约束「禁止裸色值，只许用令牌」；收尾做暗色全屏走查。
- **改名导致用户找不到功能 / 书签失效**：路由与 id 不变，仅展示改名；卡片首页提供说明与状态。
- **bundle 体积**：仅加 `lucide-react`（按需），不自建图标库、不引组件库；构建后核对产物体积。
- **过度友好损及专业感/调试能力**：技术信息不删除，只收进「详情/高级」；欢迎区保持克制，不放 demo 示例。
