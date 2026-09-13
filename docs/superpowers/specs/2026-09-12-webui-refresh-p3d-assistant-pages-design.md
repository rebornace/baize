# P3-D：助手组设置页人话化（模型 / 技能 / 助手功能）

- 日期：2026-09-12
- 状态：已交付（2026-09-13 账本对齐）
- 归属：WebUI 体验改版 P3 逐页人话化，批次 D
- 前置：P3-A（表单原语 + 账号/存储）、P3-B（业务系统/插件）、P3-C（外部工具服务/对外提供能力）已合并

## 1. 背景与目标

助手组三页（模型、技能、助手功能）已完成 P2 运营只读 gate，但仍停留在旧 `settings-*` 壳：页内英文标题（`Tools` / `Skills`）、裸 `err.message`、`window.confirm`（模型删除）、危险删除无确认（技能 / 助手功能）、登录捕获字段全英文。与账号、连接器页体验不一致。

本批目标：

1. 三页迁到 P3-A 原语与人话文案，页标题与导航一致。
2. **不改后端契约**；运营 `readOnly` 行为保持不变。
3. 按三批落地：D1 模型 → D2 技能 → D3 助手功能；约定统一，**不**抽通用 `AssistantListShell`。

## 2. 已确认的产品决策

1. 拆批：三批（模型 → 技能 → 助手功能）。
2. 架构：方案 A——各页独立落地，共用文案 / 错误映射 / 危险确认约定，不强抽壳。
3. 模型表单：主区保留名称 / 服务地址 / 模型名 / API 密钥 / 任务档位；视觉、禁用思考、上下文长度、环境变量名收进「高级」（默认收起）。
4. 模型新建/编辑：`Modal`（对齐账号 / 连接器），不用页内联或侧栏抽屉。
5. 助手功能「登录捕获」：能力保留；字段全部人话化；默认收进「高级」。不隐藏、不平铺常显。
6. 助手功能树形结构保留；添加工具由侧栏抽屉改为 `Modal`。
7. 空列表时页头与 EmptyState 不重复「添加」按钮（对齐连接器去重约定）。

## 3. 范围

在范围内：

- `ModelSettings.tsx`、`SkillsSettings.tsx`、`ToolsSettings.tsx`（及 `CaptureSettingsFields.tsx`）人话化 + 原语迁移。
- `strings.ts` 新增 `MODELS` / `SKILLS` / `TOOLS` 常量与对应人话错误映射函数。
- 危险操作统一 `ConfirmDialog`；反馈统一 `Toast`。
- 更新 / 新增 vitest；`tsc` / build；重建 `internal/ui/dist`。

明确不做（见第 10 节）：后端契约 / 新 ACL、消息组三页、运行参数、P4 全面走查、登录入口 `@` 直达、MCP OAuth、技能可视化编辑器、抽 `AssistantListShell`、聊天侧模型芯片逻辑。

## 4. 共用约定

| 约定 | 说明 |
|------|------|
| 标题 | 页内标题 = 导航：模型 / 技能 / 助手功能 |
| 原语 | `PageHeader`、`Card`、`Field`、`Input`、`Select`、`Textarea`、`Button`、`Modal`、`ConfirmDialog`、`Toast`、`EmptyState`、`Badge`、`Spinner` |
| 反馈 | 成功/失败 `Toast`；表单校验错误留在 `Field` / Modal 内 |
| 危险操作 | 一律 `ConfirmDialog`；busy 时不可关窗 |
| 错误 | `ApiError` → 人话 title；禁止裸 `err.message` 上屏 |
| 文案 | 集中 `strings.ts`；测试不绑定易变长文案时可对稳定 key / role |
| 权限 | `readOnly = role !== 'admin'`；operator 隐藏写控件；不引入新 ACL |
| 空列表按钮 | 仅 EmptyState 显示添加；有数据或加载/错误时 PageHeader 显示 |

路由与内部 id 不变（`/settings/models`、`/settings/skills`、`/settings/tools`）。

## 5. D1 模型页

### 5.1 结构

- `PageHeader`：标题「模型」+ 一句话说明（智能选择 / 按任务档位；不写 Auto 路由实现细节）+「添加模型」（admin；空列表时隐藏，改由 EmptyState）。
- 列表 `Card` 行：名称、档位 Badge、视觉 Badge、模型名 / 服务地址摘要、凭据提示（`env:…` / 脱敏 key）。
- 行操作（admin）：编辑、删除。
- 零模型：EmptyState / 告警；admin「请添加」、operator「请联系管理员」（沿用现有分流语义）。

### 5.2 新建 / 编辑 Modal

主区：

| 字段 | 人话标签 | 备注 |
|------|----------|------|
| name | 名称 | 必填 |
| base_url | 服务地址 | 原 Base URL |
| model | 模型名 | 必填 |
| api_key | API 密钥 | 编辑留空 = 不修改 |
| tier | 任务档位 | Select：自动识别 / 快速 / 标准 / 深度思考（沿用 `TIER_OPTIONS`） |

高级（默认收起）：

| 字段 | 人话标签 |
|------|----------|
| supports_vision | 视觉（支持图片附件） |
| disable_thinking | 禁用思考 |
| context_tokens | 上下文长度 |
| api_key_env | API Key 环境变量名 |

载荷：继续使用现有 `buildCreatePayload` / `buildPatchPayload` 语义，不改 API。

### 5.3 删除

`ConfirmDialog`；最后一个模型时确认文案附加现有警告语义。

## 6. D2 技能页

- `PageHeader`：标题「技能」+ 说明（对话用 `@` / `/` 调用）；去掉「Skills」与主视觉上的「默认 Agent：{id}」（若需保留 agent 信息，放次要 meta）。
- 工具栏（admin）：上传 `.md` / `.zip`、「保存默认勾选」。
- 列表：显示名优先（有 description 用描述，否则人话化 id）、来源 Badge（内置 / 用户）、工具摘要作次要信息。
- 勾选 = 默认启用集合；operator 下 checkbox 渲染但 disabled（P2 保留）。
- 空列表：`EmptyState` + 上传引导（仅 admin 操作）。
- 删除用户技能：加 `ConfirmDialog`（当前无确认）。
- 上传 / 保存：成功 / 失败 `Toast` + 人话错误。

非目标：不改技能目录解析、workflow 执行、聊天触发逻辑；不做可视化编辑器。

## 7. D3 助手功能页

### 7.1 结构

- `PageHeader`：标题「助手功能」（替换 `Tools`）+ 人话说明。
- 搜索保留；admin「添加」打开 **Modal**（替换 `settings-drawer`）。
- **保留两级树**：连接 → 路径前缀 → 工具行。
- 空态：`EmptyState`，引导去业务系统等连接页（现有路由）。
- 组操作：全部启用 / 停用（admin），人话按钮文案。

### 7.2 工具行

- 显示名 / 说明可编辑（admin）；启用、需登录、MCP 导出策略（人话选项名）。
- 「需审批」用 Badge。
- 删除 extra 工具：`ConfirmDialog`。
- 参数 schema：默认折叠「查看参数说明」，不常显大块 `<pre>`。

### 7.3 连接级设置

- 执行回调 URL：人话标签 + 短说明。
- **登录捕获**（人话化，默认收进高级）：

| 原字段 | 人话标签（定稿） |
|--------|------------------|
| tool_name_glob | 匹配哪些登录功能 |
| token_json_paths | 令牌字段路径（每行一条） |
| label_json_paths | 显示名字段路径（每行一条） |
| header_template | 请求头模板 |
| default_scheme | 默认认证方案 |

- 保存 connector 时必须 **合并保留 `auth.capture`**（及既有 capture 语义）；不得因 UI 未展开高级而清空。加前端回归：PUT body 含正确 capture。

### 7.4 添加工具 Modal

所属连接、请求方法、路径、参数说明（JSON）；校验失败人话错误；成功 Toast + 关窗 + 刷新树。

非目标：不改 require_login / require_approval / MCP export mode 后端语义；不做登录入口 `@` 直达；不与连接设置页合并职责。

## 8. 错误与文案

- 新增页面级常量：`MODELS`、`SKILLS`、`TOOLS`（含标题、说明、按钮、空态、确认文案、高级折叠标题）。
- 错误映射：分别提供 `modelErrorText` / `skillErrorText` / `toolErrorText`（三页错误码面不同，不合并成单一函数），覆盖常见 `not_found` / `invalid_request` / `internal_error` 及各页已知业务码；未知码走该页通用人话句。
- 档位 / 视觉标签继续复用已有 `TIER_LABELS` / `VISION_LABEL` / `tierLabel()`。

## 9. 测试与验收

- 更新：`ModelSettings.test.tsx`、技能纯函数测、`ReadOnlyGate.model-runtime.test.tsx`、`ReadOnlyGate.tools-skills.test.tsx`。
- 新增（按批）：
  - D1：Modal 打开/保存/取消；高级默认收起；ConfirmDialog 删除（含末模型警告）；operator 无写按钮；空列表按钮去重；人话错误。
  - D2：上传/保存 Toast 路径（可 mock）；删除确认；operator checkbox disabled；标题为人话。
  - D3：添加工具 Modal；删除确认；capture 高级折叠；保存 PUT 含 capture merge；树与搜索回归；operator 无写控件。
- 门禁：`npm run test`、`tsc --noEmit`、`npm run build`；重建并提交 `internal/ui/dist`。
- 手动：admin/operator × 明暗；三页主路径；模型高级折叠；Tools 捕获保存后再次打开仍在。

## 10. 明确不做

- 后端 API / 存库字段 / 新 ACL
- 消息组（微信 / 消息回调 / 外部来信）、运行参数页
- P4 移动端/暗色全面走查与 README 大改（可随各批重建 dist）
- 登录入口 `@` 直达、MCP OAuth
- 技能可视化编辑器、Tools 树改扁平连接器壳
- 抽 `AssistantListShell`
- 聊天侧模型芯片 / Auto 路由逻辑变更

## 11. 风险与对策

| 风险 | 对策 |
|------|------|
| Tools ~900 行回归面大 | D3 独立一批；先 D1/D2 钉死范式 |
| capture 被误清空 | 保存路径强制 merge；PUT body 断言测试 |
| 文案散落难维护 | 一律进 strings 常量 |
| 空列表双按钮 | 统一去重约定 + 单测 |

## 12. 建议实现顺序（交 writing-plans 细化）

1. **D1 模型**：strings + 错误映射 → 列表/EmptyState/PageHeader → Modal 表单（主区+高级）→ ConfirmDialog 删除 → 测试 → rebuild dist  
2. **D2 技能**：标题/原语 → ConfirmDialog 删除 → Toast → 测试 → rebuild dist  
3. **D3 助手功能**：标题/树壳原语化 → 工具行与删除确认 → capture 人话+高级 → 添加工具 Modal → capture merge 测试 → rebuild dist  

每批可独立合并到 `main`（real 仓直接推 main，无需 PR）。
