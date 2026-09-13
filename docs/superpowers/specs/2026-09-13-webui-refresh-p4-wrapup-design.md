# WebUI P4 收尾：走查、a11y、CSS 工程与文档对齐

- 日期：2026-09-13
- 状态：已交付
- 归属：WebUI 体验改版 P4（母规格 `2026-09-09-webui-experience-refresh-design.md` §10）
- 前置：P0–P3（含 P3-A/B/C/D）与助手功能 IA 收口已合并；公开仓已同步至对应切片
- 方法：验收驱动收尾（头脑风暴方案 1，已批准）

## 1. 背景与目标

体验改版主线功能已落地，但仍欠母规格定义的 **P4 收尾**：明/暗与多宽度全面走查、可访问性底线、测试补齐、CSS 工程收口（`style.css` 仍为聊天样式主文件且未抽出 `styles/chat.css`）、README 用户路径术语与现导航对齐、重建嵌入 `dist`。P1 聊天视觉验收表中仍有多项「待复测」。

**目标：** 闭合 WebUI 体验改版（P4）：明/暗与多宽度可用、可访问性底线达标、无令牌外裸色漏网、关键回归有测试、CSS 工程干净、README 设置导航术语与现 UI 对齐，并重建嵌入 `dist`。

**成功标准（可验收）：**

1. 明/暗 × 桌面/平板/手机走查清单全部勾完（聊天 + 设置首页 + 每组代表页），严重视觉/布局问题本轮修完。
2. 可见样式（除 tokens 定义与允许的透明渐变）无硬编码色；正文对比度 ≥4.5:1。
3. 键盘可操作主要控件；焦点环可见且走 `--focus-ring`。
4. `npm test` / `tsc` / `npm run build` 全绿；`internal/ui/dist` 已更新。
5. README（中/英）设置路径用语与现导航一致（如「助手功能」「业务系统」），不改 API 文档契约表述。

## 2. 已确认决策

1. 下一里程碑选 **WebUI P4**（非 F 生产硬化）。
2. 落地方式选 **验收驱动收尾（方案 1）**，不做「仅工程收口」或「收尾+体验加码」。
3. 不引入 Playwright E2E；不做消息组三页全面人话化；不改后端 API/ACL/路由 id。
4. README 只对齐**面向用户的导航说法**；API 路径与字段名保持技术标识。
5. 截图进 README 为可选（0–2 张），非关门条件。
6. 规格/计划/验收 notes 仅存 `docs/superpowers/**`（baize_real）；推开源仓由用户另行下令。

## 3. 范围

### 3.1 在范围内

- 程序化扫描 + 人工走查（对照并更新 P1 验收表，扩展设置页矩阵）。
- 发现的中高优视觉/布局/a11y bug 修复。
- 抽出 `styles/chat.css`，删除或清空冗余 `style.css`（零行为搬移优先）。
- 补 vitest 缺口（迁移冒烟、ConfirmDialog 替换路径、走查修出的可单测回归）。
- README 轻量术语对齐；重建 `dist`。
- 验收记录：`docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md`。

### 3.2 不在范围

- 后端 API / ACL / 路由 id 变更。
- Playwright E2E、i18n 框架、消息组三页全面人话化、新功能字段。
- 大改 README 架构或海量截图。
- F 生产硬化、OTel、SDK、架构文档 P1/P2（另开里程碑）。

## 4. 走查矩阵与验收清单

**原则：** 可勾选、可度量；严重问题（布局崩、暗色穿帮、无法操作）本轮必修；纯文案偏好记入 notes，不阻塞关闭。

### 4.1 主题 × 视口矩阵

| 视口 | 宽度参考 | 浅色 | 暗色 |
|------|----------|------|------|
| 桌面 | ≥1280 | ☐ | ☐ |
| 平板 | ≤1024 | ☐ | ☐ |
| 手机 | ≤768（含 ~390） | ☐ | ☐ |

每格至少覆盖：**聊天** + **设置总览** + **一组代表设置页**（见 4.3）。

### 4.2 聊天（承接并关闭 P1 验收表待复测项）

| ID | 项 | 通过标准 |
|----|-----|----------|
| C-A4 | 暗色无浅色块穿帮 | 程序化或人工：主区域底/面亮度不「白天块」 |
| C-C2 | 空态 composer 高度 | 无模型时高度≈有模型单行，不被芯片撑高 |
| C-D1 | 模型芯片布局 | 横排胶囊，不竖排压扁 |
| C-D4 | 工具/HITL/workflow 卡 | 样式未回归；人话短语可见 |
| C-E1 | ≤768 抽屉 | 侧栏抽屉可用，输入区不被挤爆 |
| C-E2 | 触控操作常显 | `hover:none` 下消息操作可点 |
| C-E3 | 正文对比度 | 明/暗正文 vs 底 ≥4.5:1（抽测消息/侧栏/设置正文） |

### 4.3 设置（代表页，不全页穷举字段）

| 组 | 必走页 | 关注点 |
|----|--------|--------|
| 总览 | `/settings` | 卡片、徽标、刷新、运营「仅管理员」锁 |
| 助手 | 模型、助手功能、技能 | 只读 gate；弹窗/上传按钮；树列表窄屏 |
| 连接 | 业务系统、外部工具服务、对外提供能力 | Modal 内滚、高级折叠、暗色 Textarea |
| 消息 | 微信；消息回调或外部来信择一深页 | 二维码区、运营 login 边界 |
| 系统 | 账号、存储、运行参数 | Field a11y、危险操作 ConfirmDialog |

运营角色：总览锁卡 + 微信扫码 + 助手/技能只读，各至少走一次。

### 4.4 可访问性底线（全站抽测）

- 焦点环可见且用 `--focus-ring`。
- Tab 可达：侧栏/抽屉、设置导航、主按钮、Modal 开关。
- 表单：`label`↔控件；错误时 `aria-invalid` + `aria-describedby`（已有 Field 的页保持）。
- 残留 `window.confirm`（如运行参数清空凭据）→ 改为已有 `ConfirmDialog`。

### 4.5 程序化门禁

- 扫描 `web/chat/src/**/*.{css,tsx}`：除 `tokens.css` 与白名单（透明渐变、第三方画布）外禁止裸 `#` / `rgb` / `rgba` 色。
- `npm test`、`npx tsc --noEmit`、`npm run build` 全绿。

## 5. 工程改动

### 5.1 CSS

| 动作 | 说明 |
|------|------|
| 抽出 `styles/chat.css` | 将现有 `style.css`（聊天壳/消息/composer 等）整文件迁入；`main.tsx` 改为引入 `./styles/chat.css` |
| 删除或清空 `style.css` | 迁移后无残留引用；禁止两份并行维护 |
| 零行为优先 | 首任务只搬移 + 改 import；类名与选择器不变 |
| 走查修样式 | 在对应模块文件改，只用令牌 |

不强制再拆更细文件（unlock 可留在 `chat.css`）。

### 5.2 交互债

- `RuntimeSettings` 中 `window.confirm` → `ConfirmDialog`。
- 不新开组件库；尽量不改 ConfirmDialog 公共 API。

### 5.3 测试策略

已有：`theme` / `ThemeToggle` / `SettingsHome` / `ConfirmDialog` / `Field` 等。P4 增量：

1. CSS 迁移动态冒烟（确认引入路径 / 构建成功）。
2. Runtime 清空凭据路径的 ConfirmDialog 交互断言。
3. 走查修出的中高优 bug：能单测则补 vitest，否则写入 acceptance note「仅人工」。
4. 全量绿后重建 `internal/ui/dist`。

不做 Playwright；不做全设置页截图 CI。

### 5.4 README

- `README.md` / `README.zh-CN.md`：用户路径旧名（Tools / OpenAPI / Inbox 等导航说法）→ 现 `settingsNav` 人话。
- 保留 API 路径、JSON 字段、curl 中的技术标识。
- 截图可选，非 DoD。

### 5.5 双仓

- 提交进 `real/main`（中文 Conventional Commits）。
- 过程文档仅 `docs/superpowers/**`。
- 开源导出由用户明确下令后再执行。

## 6. 交付物与任务预览

| 产物 | 位置 |
|------|------|
| 本规格 | `docs/superpowers/specs/2026-09-13-webui-refresh-p4-wrapup-design.md` |
| 实现计划 | `docs/superpowers/plans/2026-09-13-webui-refresh-p4-wrapup.md` |
| 验收记录 | `docs/superpowers/notes/2026-09-13-webui-p4-acceptance.md` |
| 代码 | `web/chat`、README、`internal/ui/dist` |

**任务预览（计划阶段细化为 TDD）：**

1. CSS 迁移：`style.css` → `styles/chat.css` + `main.tsx`。
2. 裸色门禁脚本/测试 + 清违规。
3. ConfirmDialog 替换 Runtime `window.confirm` + 测试。
4. 按 §4 矩阵走查并修中高优 bug。
5. README 术语对齐。
6. 全量验证 + 重建 dist + 填写 acceptance note。

## 7. 完成定义（DoD）

- [x] §4 矩阵与清单在 acceptance note 中全部勾选或标明「降级/已知债 + 理由」
- [x] 无未解释的裸景色；`npm test` / `tsc` / `build` 绿
- [x] `style.css` 不再作为第二聊天样式源
- [x] Runtime 危险确认走 `ConfirmDialog`
- [x] README 用户导航术语与 `settingsNav` 人话一致
- [x] `internal/ui/dist` 已随构建更新并提交
- [x] 本规格非目标均未纳入本里程碑

## 8. 风险与对策

| 风险 | 对策 |
|------|------|
| 搬 CSS 漏选择器 | 整文件搬移 + build 后点开聊天壳 |
| 走查发现大范围 IA 问题 | 记 backlog，不扩 P4 |
| 对比度需改令牌 | 只动 `tokens.css`，全站抽测回归 |

## 9. 之后

P4 关门后，开源路线图下一刀为 backlog **F 生产硬化 + 架构文档 P1/P2**（另开头脑风暴，不塞进本计划）。

## 10. 参考

- 母规格：`docs/superpowers/specs/2026-09-09-webui-experience-refresh-design.md`
- P1 验收表：`docs/superpowers/notes/2026-09-10-webui-p1-chat-visual-acceptance.md`
- OSS backlog：`docs/superpowers/notes/2026-08-28-oss-backlog-and-enterprise.md`
