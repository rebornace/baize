# 运行参数页人话化（UI-RUNTIME）

- 日期：2026-09-13
- 状态：已批准（头脑风暴）
- 归属：开源首版史诗 **UI-RUNTIME**（确认清单合并策略 A）；承接 P3-D / runtime 热更新规格附注的体验债
- 方法：壳 + 三分区；压缩区开关外置 + 高级折叠；口令区温和人话；文案抽离附录（非完整 i18n）

## 1. 背景与目标

运行参数热更新（`GET/PATCH /v0/settings/runtime`、凭据端点）与设置页**功能已交付**。页面仍偏旧壳：标题「运行时设置」、标签夹带英文字段名、裸 `settings-field`、状态串反馈；压缩细参与主旋钮平铺；口令区术语偏运维黑话。与消息组 / P3 原语体验落差大。

**目标：** 把「运行参数」拉到与消息组同级的人话 + 原语体验；**不改**后端 API / ACL / 热更新语义。

**成功标准：**

1. 页标题与导航一致为「运行参数」；无页内主标题「运行时设置」。
2. 使用 `PageHeader`、`Field`/`Input`/`Button`、`Toast`、既有 `ConfirmDialog`；错误走 `friendlyError` / 集中 `RUNTIME` strings。
3. 可见三分区：**对话与工具行为** · **历史压缩** · **控制面口令**（口令仅 admin）。
4. 压缩：主开关外置；细参（比例 / 预留 token / 保留条数 / 摘要超时）默认收进「高级压缩设置」。
5. 口令区温和人话（运营口令 / 管理口令），小字可保留 `operator` / `admin`；不改 JSON 字段名。
6. 运营只读行为不变（无写控件；不渲染口令区）。
7. 相关 vitest 更新；`tsc` / `build` 绿；重建 `internal/ui/dist`。
8. **无 Go / API / 路由 id 变更。**

## 2. 已确认决策

1. 范围：**壳 + 分区**（非只换壳、非拆口令到账号页）。
2. 压缩展示：**开关外置 + 高级折叠**。
3. 口令文案：**温和人话**（非保留纯术语、非本刀迁页）。
4. 合并策略 A：**文案键抽离到 `strings.ts`**，完整多语言切换属 **UI-I18N**，本刀不做。
5. 引擎参数仍**一次保存**覆盖分区 1+2 的改动（与现 `PATCH` 语义一致）；口令区独立提交。

## 3. 范围

### 3.1 在范围内

| 路径 | 做什么 |
|------|--------|
| `web/chat/src/pages/RuntimeSettings.tsx` | 人话壳、三分区、压缩折叠、Toast、口令区文案 |
| `web/chat/src/pages/runtimeSettingsHelpers.ts` | 字段分组元数据；人话 label/hint（技术名进 hint） |
| `web/chat/src/strings.ts` | 扩写 `RUNTIME`；稳定键名 |
| 相关 `*.test.tsx` / `*.test.ts` | 分区标题、折叠、只读、ConfirmDialog 回归 |

### 3.2 不在范围

- 后端契约、新热更新项、ACL、路由
- 口令迁「账号」页或新子路由
- 完整 i18n 语言切换 / 英包
- F / OPS-HOT、思考级别、MCP OAuth、Memory
- Playwright E2E

## 4. 信息架构与文案

**原则：** 路由与 JSON 字段名不变；界面人话；技术标识进 hint/`code`/高级区；界面文案不出现产品名。

### 4.1 页头

| 元素 | 文案 |
|------|------|
| 标题 | 运行参数 |
| 副标题（admin） | 调整对话长度、工具超时与历史压缩；保存后立即生效，无需重启。控制面口令也可在本页轮换。 |
| 副标题（operator） | 只读查看当前生效的运行参数；修改请联系管理员。 |

### 4.2 分区 1 · 对话与工具行为

| 字段 | UI 标签 | hint |
|------|---------|------|
| `max_messages` | 送入模型的最近消息数 | 技术名 `max_messages`；范围 1–500 |
| `max_steps` | 单次运行最多工具步数 | `max_steps`；1–100 |
| `tool_timeout_seconds` | 单个工具最长等待（秒） | `tool_timeout_seconds`；1–600 |

### 4.3 分区 2 · 历史压缩

| 元素 | 文案 |
|------|------|
| 主开关 | 对话过长时自动压缩历史（`compaction_enabled`） |
| 区说明 | 压缩会把旧消息收成摘要以省上下文；细参多数保持默认即可 |
| 折叠标题 | 高级压缩设置（默认收起） |
| 折叠内字段 | 触发比例、预留 token、保留最近原文条数、摘要超时（对应既有 compact_* 键；hint 含技术名与范围） |

### 4.4 分区 3 · 控制面口令（仅 admin）

| 元素 | 文案方向 |
|------|----------|
| 区标题 | 控制面口令 |
| 主口令 | 运营口令 / 管理口令；小字 `operator` / `admin`；留空表示不修改 |
| 来源行 | 热更新覆盖 / 配置基线；口令是否已设置（人话，不暴露明文） |
| 命名账号 | 命名运营账号（原 naming operator）；新增 id + token |
| 重置 | 重置为配置基线口令；沿用 `ConfirmDialog`（可微调 `RUNTIME.confirmReset*`） |
| 徽章 | 「已覆盖基线」等保留语义 |

### 4.5 反馈

- 成功：Toast（保存参数 / 轮换口令 / 新增或移除账号 / 重置）
- 错误：`friendlyError` + `RUNTIME` 文案；校验错误用人话（可基于现 `validateKnobField` 改写）
- 无改动保存：Toast 或短提示「没有改动」

## 5. 实现结构

对标 `2026-09-13-messaging-settings-humanize-design.md` 的前端路径：

1. 扩 `RUNTIME` strings（页头、分区、字段、折叠、Toast、校验、口令）。
2. `runtimeSettingsHelpers`：区分「主区字段」与「压缩高级字段」列表；label/hint 来自 strings 或 helpers 常量（二选一，计划里定，避免双源）。
3. `RuntimeSettings`：`PageHeader` + `ToastRegion`；分区渲染；压缩 `<details>` 或等价折叠；口令区换原语；去掉页内「运行时设置」。
4. 测试：helpers 分组；页面分区标题；高级默认收起；运营只读；重置 ConfirmDialog。
5. `npm run build` → 提交 `internal/ui/dist`。

**不改 Go。**

## 6. 测试与验收

| 项 | 验收 |
|----|------|
| 标题 | 可见「运行参数」，无「运行时设置」主标题 |
| 分区 | 三个区标题存在（运营无口令区） |
| 压缩 | 细参默认不可见；展开后可编辑（admin） |
| 保存 | admin PATCH knobs 行为与现网一致 |
| 口令 | 轮换 / 新增 / 移除 / 重置 + ConfirmDialog |
| 只读 | operator 无保存/口令写控件（既有 gate） |
| 工程 | vitest / tsc / build 绿 |

## 7. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 人话后运维找不到字段 | hint 保留技术名 |
| 折叠导致漏改细参 | 区说明写清「多数默认即可」 |
| strings 与 helpers 双源 | 计划规定单一来源 |

## 8. 附录：文案抽离（面向 UI-I18N）

- 本刀所有新增/改写 UI 字符串进 `RUNTIME`（或同文件分组），**键名稳定、无中文当 key**。
- **不做** `en` 包、语言切换、浏览器语言探测。
- 完整 i18n 框架由史诗 **UI-I18N** 另开规格。

## 9. 参考

- 热更新功能规格：`2026-09-05-runtime-settings-hot-reload-design.md`（已交付）
- 人话范式：`2026-09-13-messaging-settings-humanize-design.md`、`2026-09-13-messaging-copy-outcome-design.md`
- 产品确认：`docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md` §1 UI-RUNTIME

---

*批准后进入 writing-plans 编写实现计划。*
