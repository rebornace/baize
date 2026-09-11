# WebUI 体验改版 P3-A：表单地基 + 账号/存储页人话化设计

> 状态：设计稿（仅私有仓 `docs/superpowers`，不导出公开仓）
> 日期：2026-09-11
> 方法：Superpowers 头脑风暴（用户逐节认可）
> 上游：`2026-09-09-webui-experience-refresh-design.md` 路线图 §10 的 **P3 逐页人话化**，本规格为 P3 的第一批（P3-A）。

## 1. 背景与定位

P0 已落地设计令牌、CSS 模块化、基础组件、暗色与移动端骨架；P1 完成聊天重构；P2 完成设置信息架构（卡片总览首页、分组导航、状态徽标、5 个页面的运营只读 gate）。

但卡片总览背后的**设置子页面内部**大多仍是技术形态：字段裸英文术语、裸红字错误、原生 `window.confirm`、未使用 P0/P1 的基础组件。

P3 分多批推进。本批（**P3-A**）先补齐**表单型设置页的通用原语**，并选两个差异明显的页面作试点钉牢范式：

- **账号页**（`IdentitiesSettings.tsx`，列表型，运营可访问）；
- **存储页**（`StorageSettings.tsx`，表单 + 高危确认型，仅管理员）。

后续批次（业务系统、MCP、MCP 导出、插件、消息回调、外部来信）直接套用本批原语，不在本规格范围。

## 2. 目标与非目标

### 目标
- 新增表单地基原语：`PageHeader`、`Field`、`Input`、`Select`、`EmptyState`，全部令牌化，暗色/移动/键盘/对比度（正文 ≥4.5:1）到位。
- 账号页、存储页用新原语完整人话化：错误走 Toast、高危操作走 `ConfirmDialog`、技术细节收进「详情/高级」。
- 移除账号页的「粘贴用户 Token」输入框（仅 Bearer、面向开发者，正常流程不需要）。
- 为后续 6 个复杂设置页确立可复制范式。

### 非目标
- **不改任何后端 API 契约**：路由、字段名、状态语义不变；`POST .../identities`（手动创建身份）接口保留，仅账号页不再调用。
- 不做新的业务系统鉴权方式（OAuth 自动刷新、Basic 预设、Query 参数 API-Key、Cookie 等）。现状连接器层 `static/passthrough/vault_ref` 已能表达任意 HTTP 头（头名 + 头值），覆盖 Bearer / 自定义 API-Key 头 / 企业网关头 / 手填 Basic；新鉴权方式若需要，另开后端规格。
- 不建本批用不到的原语：`Switch`、`Textarea`、`Tooltip`、`Drawer`、`Segmented`（YAGNI；本批两页无布尔开关语义，确认项用 checkbox）。
- 存储页不做数据迁移逻辑（维持"切换不自动迁移"语义）。
- 不做多语言框架。

## 3. 设计语言

沿用 P0 令牌（`styles/tokens.css`）：色值只许引用令牌，禁止裸色值；间距用 `--space-*`、圆角用 `--radius-*`、聚焦用 `--focus-ring`、悬停用 `--hover`；语义色用 `--danger/--success/--warning` 及软底。控件高度、触摸目标（移动端 ≥40px）与 P0/P1 既有控件一致。

## 4. 表单地基原语（`web/chat/src/components/ui/`）

仅新增本批两页真实使用的原语；样式统一写入 `styles/components.css`（明暗两套都覆盖）。

### 4.1 `PageHeader`
- Props：`title: ReactNode`、`description?: ReactNode`、`actions?: ReactNode`。
- 结构：上方一行，左侧标题（沿用现有页面标题字号/字重），右侧 `actions` 操作区；标题下方一行弱化色说明。
- 替代各页不统一的 `<h1>/<h2> + settings-meta/hint`。

### 4.2 `Field`
- Props：`label: ReactNode`、`htmlFor?: string`、`hint?: ReactNode`、`error?: ReactNode`、`required?: boolean`、`children`、`className?`。
- 结构：`<label>`（标题）→ 子控件 → `hint`（弱化色白话说明）→ `error`（危险色）。
- 可访问性：label 与控件用 `htmlFor/id` 关联；有错误时控件设 `aria-invalid="true"`，错误/说明节点用 `aria-describedby` 关联。

### 4.3 `Input` / `Select`
- 透传原生 `<input>` / `<select>` 的 props（含 `id`、`type`、`value`、`onChange`、`disabled`、`placeholder`、`autoComplete`）。
- 统一边框、内边距、高度、圆角、背景（明暗）、聚焦环（`box-shadow: 0 0 0 3px var(--focus-ring)`）、disabled 态、错误态（边框取 `--danger`）。
- `Select` 的选项由调用方提供（本批用于存储 driver）。

### 4.4 确认勾选（checkbox 统一样式）
- 本批没有"运行时开关"语义，故**不引入 `Switch`**；存储页"我了解不迁移数据"是提交前的**确认勾选**，使用语义正确的原生 `<input type="checkbox">`，仅在 `components.css` 统一其间距/对齐/聚焦环样式（令牌化、暗色、可点标签）。

### 4.5 `EmptyState`
- Props：`icon?: ReactNode`、`title: ReactNode`、`description?: ReactNode`、`action?: ReactNode`。
- 居中空态：图标 + 一句白话标题 + 可选说明/动作。

### 4.6 反馈与确认（复用 P1，不新建）
- 成功/失败提示：`useToast` + `<ToastRegion/>`，错误文案过 `friendlyError(ApiError)`。
- 高危确认：`ConfirmDialog`（`danger`、初始焦点在取消、Esc/遮罩关闭、busy 禁用）。

## 5. 「账号」页人话化（`pages/IdentitiesSettings.tsx`）

### 5.1 页面定位
管理"当前浏览器会话中，助手已登录了哪些业务系统账号"。路由 `/settings/identities` 不变；运营 `full`，不加只读 gate。

### 5.2 结构
- `PageHeader`：
  - title：**账号**。
  - description：「这里显示助手当前已登录的业务系统账号。正常使用时，在对话中完成登录会自动出现在这里，无需手动填写。」
- **移除「粘贴用户 Token」整块**：删除 `pasteToken` 状态、对应 `Field/input/保存 Token` 按钮及本页对 `createIdentity` 的调用。
  - 依据：该入口只生成 `Authorization: Bearer`（见 `internal/identity/manual_auth.go` 的 `CredentialFromUserToken`），形态局限且面向开发者；排障时可在对话中直接粘贴 Token，由 `ExtractAndStripToken` 自动识别 JWT/Authorization 并写入会话身份，能力不丢。
  - 后端接口与对话内捕获逻辑保持不变。
- **账号列表**：每个身份用一张 `Card`：
  - 主标题：`label || id`。
  - 副标题（来源人话，映射移入 `strings.ts`）：
    - `login_capture` → **对话中登录**；
    - `env` → **系统预设**；
    - `manual` → **临时提供**；
    - 未知来源 → 原值（兜底，避免吞信息）。
    - 前缀展示 `scheme`（如有），如 `Bearer · 对话中登录`。
  - `claims_summary`（JWT 摘要 JSON）默认收进「详情」折叠（`<details>`），不再默认摊开 `<pre>`；展示前仍经 `redactSensitive`。
  - 操作：
    - 默认账号：`Badge`「默认中」；
    - 非默认账号：次要按钮「设为默认」→ `setDefaultIdentity`；
    - 非 `env` 来源：危险文本按钮「退出」→ `deleteIdentity`。
- **清空捕获账号**：仅当存在非 `env` 身份时显示；由直接执行改为先弹 `ConfirmDialog`（危险操作，说明将移除这些登录），确认后调 `clearIdentities`。
- **空态**：`EmptyState`：标题「暂无已登录的业务账号」，说明「在对话中登录业务系统后，会自动显示在这里」。
- 顶部技术味的「会话 `conv_xxx`」标识默认隐藏，收进页面底部「详情/开发者」小字（`<details>`）。该 id 来自本地 localStorage（`baize.conversation_id`），普通用户无需理解。

### 5.3 反馈
- 列表加载失败：Toast 人话提示，列表区显示可重试/空态，不整页崩溃。
- 操作成功：Toast（如「已退出账号」「已设为默认」「已清空登录账号」）；失败：Toast 显示 `friendlyError`。
- 移除现有裸红字 `settings-error` 用法。

## 6. 「存储」页人话化（`pages/StorageSettings.tsx`）

### 6.1 页面定位
选择对话与数据的保存位置。低频、高危（保存即重启、不自动迁移数据）。路由 `/settings/storage` 不变；运营 `locked`（侧栏不可见、路由被 `AdminOnly` 包裹），本页不加只读 gate。

### 6.2 driver 人话映射
内部提交值保持英文 driver，展示映射（移入 `strings.ts`）：

| driver（提交值） | 展示 |
|---|---|
| `sqlite` | 本地文件（SQLite） |
| `postgres` | PostgreSQL 数据库 |
| `memory` | 内存（重启即清空，仅试用） |
| 其他/未知 | 原值兜底 |

可选项来自 `info.drivers`，缺失时回退 `['memory','sqlite','postgres']`。

### 6.3 结构
- `PageHeader`：
  - title：**数据存储**。
  - description：「选择助手数据的保存位置。更改并保存后服务会重启，且不会自动搬迁旧数据，请先自行备份。」
- 一张 `Card` 包住表单，字段用 `Field`：
  - 「保存方式」：`Select`，选项用上表人话标签，value 仍为英文 driver。
  - 选 `sqlite`：`Input`「数据库文件路径」（`sqlite_path`），hint「默认 `./data/baize.db`；换成新路径不会自动搬数据」。
  - 选 `postgres`：`Input type=password`「连接地址（DSN）」，placeholder 保留 `postgres://user:pass@host:5432/baize?sslmode=disable`，hint「形如 `postgres://用户名:密码@主机:5432/库名`，仅保存在服务端配置」；回显用 `info.dsn_redacted`（占位），不回显明文。
- 确认勾选（checkbox，统一样式）：「我了解：切换存储不会自动迁移数据，旧库中的数据需自行处理」。未勾选提交时，就地给出错误提示（沿用现有校验，文案人话化），不发请求。
- 表单底部放置普通主操作按钮（`Button variant=primary`）「保存并重启」，点击先弹 `ConfirmDialog`，**危险样式只出现在弹窗内的确认键**，避免页面上常态出现刺眼红按钮：
  - title「保存并重启服务？」
  - body「服务将立即重启，进行中的对话会中断；数据不会从旧存储自动迁移。确认继续？」
  - `danger`、确认文字「保存并重启」、初始焦点在「取消」。
  - 取消：不发请求；确认：调 `putStoreSettings({ driver, sqlite_path, dsn, acknowledge_no_migrate:true, restart:true })`。
- `config_path / overlay_path` 技术信息收进页面底部「详情」小字。
- 提交中：按钮 loading + 全表单 disabled，防重复提交。

### 6.4 反馈
- 成功：Toast 显示 `resp.message` 或「正在重启…」。
- 失败（如 postgres 缺 DSN、后端 400）：优先在对应 `Field` 上显示错误；通用失败走 Toast `friendlyError`。
- 移除 `window.confirm` 与裸红字。

## 7. 工程结构

**新增**
- `components/ui/PageHeader.tsx`、`Field.tsx`、`Input.tsx`、`Select.tsx`、`EmptyState.tsx`。
- 对应测试：`PageHeader`/`Field`/`EmptyState` 至少各一；`Select` 的选项映射通过存储页测试覆盖（控件类以行为断言为主）。
- `components/ui/index.ts` 补导出。
- 样式写入 `styles/components.css`（含确认 checkbox 的统一令牌化样式）。

**修改**
- `pages/IdentitiesSettings.tsx`：按 §5 重写。
- `pages/StorageSettings.tsx`：按 §6 重写。
- `strings.ts`：来源标签、空态、driver 映射、字段标签/hint、Toast/确认文案集中管理。
- `api.ts`：`createIdentity` 保留（不删，避免牵连），账号页不再 import 使用。
- 完成后 `npm run build` 重建 `internal/ui/dist` 并提交嵌入产物。

**依赖**：不新增 npm 依赖（图标用既有 `lucide-react`）。

## 8. 边界与错误处理

- 账号列表加载失败：Toast + 空/错误态，不白屏。
- 存储 `drivers` 缺失：回退默认三项；未知 driver 原值显示，不丢失。
- ConfirmDialog 打开期间：底层提交按钮不可再触发；Esc/遮罩等同取消。
- busy 期间：相关按钮 loading/disabled，防重复提交；存储表单整体 disabled。
- 明暗主题与移动端：新原语在 ≤768px 触摸目标 ≥40px，字段单列；卡片/弹窗在窄屏不溢出。

## 9. 测试策略（vitest + jsdom，TDD）

**原语**
- `Field`：label 与控件 `htmlFor/id` 关联；传 `error` 时控件 `aria-invalid` 且错误节点被 `aria-describedby` 引用。
- `Select`：driver 选项显示人话标签、提交值仍为英文 driver（在存储页测试中覆盖）。
- `PageHeader`/`EmptyState`：渲染 title/description/action。

**账号页**
- 不渲染 Token 输入框与「保存 Token」（断言查询不到）。
- 空列表渲染 `EmptyState` 文案。
- 来源标签映射（env/login_capture/manual/未知）。
- 非 env 账号出现「退出」、非默认出现「设为默认」、默认账号显示「默认中」徽标。
- 「清空」先弹 ConfirmDialog，取消不调 API，确认才调 `clearIdentities`。

**存储页**
- 三种方式切换：sqlite 显示文件路径字段，postgres 显示 DSN 字段，memory 不显示额外字段。
- 未勾选确认项提交：不发请求且出现提示。
- 点「保存并重启」先弹 ConfirmDialog；取消不发 PUT；确认才 PUT，且 body 含 `acknowledge_no_migrate:true`、`restart:true` 与正确 driver。

**回归**：保持现有前端用例全绿（必要时把绑定中文/结构的断言改为稳定 `role`/`testid`）；`tsc --noEmit` 零错误；`npm run build` 成功。后端不改逻辑、Go 测试保持绿；仅重建嵌入 `dist`。

## 10. 风险与对策

- **表单原语抽象过早/不当**：只建两页真实用到的原语，props 从两页真实需求反推，不预留 speculative 属性；后续页面若有差异再演进。
- **删除粘贴框影响排障**：对话内直接贴 Token 仍被 `ExtractAndStripToken` 捕获；后端接口保留；规格与（后续）文档说明该替代路径。
- **样式回归/暗色遗漏**：禁止裸色值、只用令牌；完成后明暗双主题人工走查两页。
- **driver 映射吞值**：未知 driver 一律原值兜底显示与提交。
