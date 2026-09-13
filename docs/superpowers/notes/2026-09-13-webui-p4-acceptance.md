# WebUI P4 走查验收记录

- 日期：2026-09-13
- 规格：`docs/superpowers/specs/2026-09-13-webui-refresh-p4-wrapup-design.md` §4
- 工作树：`.worktrees/webui-p4-wrapup` · 分支 `feat/webui-p4-wrapup`
- 方法：隔离实例 + Chrome/Puppeteer 明/暗 × 三宽度走查；截图见 `docs/superpowers/notes/p4-acceptance-shots/`
- 实例：`go run ./cmd/baize serve -config configs/p4-walkthrough.yaml` → `http://127.0.0.1:18880/ui`（口令 admin=`p4-admin` / op=`p4-op`；独立 DB `data/p4-walkthrough.db`）
- 程序化门禁：`npm test` 487 passed；`npx tsc --noEmit` 绿；`npm run build` 绿；裸色扫描绿
- cursor-ide-browser：本机 MCP 建 tab 后立即失效，改用本机 Chrome + `puppeteer-core` 完成同等矩阵

## 4.1 主题 × 视口矩阵

| 视口 | 宽度参考 | 浅色 | 暗色 |
|------|----------|------|------|
| 桌面 | ≥1280 | ✅ `desktop-light-*.png` | ✅ `desktop-dark-*.png` |
| 平板 | ≤1024 | ✅ `tablet-light-*.png` | ✅ `tablet-dark-*.png` |
| 手机 | ≤768（含 ~390） | ✅ `phone-light-*.png` | ✅ `phone-dark-*.png` |

每格覆盖：聊天 + 设置总览 + 运行参数代表页。

## 4.2 聊天（承接并关闭 P1 验收表待复测项）

| ID | 项 | 通过标准 | 结果 |
|----|-----|----------|------|
| C-A4 | 暗色无浅色块穿帮 | 程序化或人工：主区域底/面亮度不「白天块」 | ✅ 暗色三宽度 `brightSurfaces=[]`；主区 `rgb(28,32,39)`；截图无白天块 |
| C-C2 | 空态 composer 高度 | 无模型时高度≈有模型单行，不被芯片撑高 | ✅ 桌面/平板 composer≈81px；芯片 74×32 横排。手机≈147px 为窄屏 toolbar 换行+多行 placeholder（非竖排芯片撑高）；**仅人工**复测截图 `phone-*-chat.png` |
| C-D1 | 模型芯片布局 | 横排胶囊，不竖排压扁 | ✅ `model-chip-empty` 74×32 |
| C-D4 | 工具/HITL/workflow 卡 | 样式未回归；人话短语可见 | ✅ 构建 CSS 含 `.tool-card`/`.hitl-*`/`.workflow-*`；`ToolCard` 人话文案与 vitest 仍在。本实例无模型，未拉起真人 HITL 对话 → **仅人工**视觉以 CSS+单测为准，非布局回归。`desktop-tool-hitl-phone.png` 为空态聊天、名不副实（见证据诚实说明 / N2） |
| C-E1 | ≤768 抽屉 | 侧栏抽屉可用，输入区不被挤爆 | ✅ `drawer-open` + scrim；截图 `phone-drawer-open-2.png`；composer 仍可用 |
| C-E2 | 触控操作常显 | `hover:none` 下消息操作可点 | ✅ 构建 CSS 含 `@media (hover: none)` 规则 |
| C-E3 | 正文对比度 | 明/暗正文 vs 底 ≥4.5:1 | ✅ 抽测约 16.5:1（浅）/ 13.4:1（暗） |

## 4.3 设置（代表页）

| 组 | 必走页 | 关注点 | 结果 |
|----|--------|--------|------|
| 总览 | `/settings` | 卡片、徽标、刷新、运营「仅管理员」锁 | ✅ admin/op 截图；运营锁卡可见 |
| 助手 | 模型、助手功能、技能 | 只读 gate；弹窗/上传按钮；树列表窄屏 | ✅ 运营技能页无上传/新建等写按钮 |
| 连接 | 业务系统、外部工具服务、对外提供能力 | Modal 内滚、高级折叠、暗色 Textarea | ⚠️ 外部工具服务有实拍（`admin-settings-mcp.png`）；业务系统 / 对外提供能力**未实拍或错贴**（见「证据诚实说明」）；人工打开正确路由未见严重穿帮，但不以错贴图为证 |
| 消息 | 微信；消息回调或外部来信择一深页 | 二维码区、运营 login 边界 | ✅ 微信页可开（适配器未二进制 → 启动异常徽标，预期） |
| 系统 | 账号、存储、运行参数 | Field a11y、危险操作 ConfirmDialog | ✅ 重置凭据打开 `role=dialog` ConfirmDialog（`runtime-confirm-dialog.png`） |

运营角色：总览锁卡 + 微信扫码 + 助手/技能只读，各至少走一次。 ✅

## 4.4 可访问性底线（全站抽测）

| 项 | 通过标准 | 结果 |
|----|----------|------|
| 焦点环 | 可见且用 `--focus-ring` | ✅ Tab 后设置导航 `outline` 可见；CSS 使用 `var(--focus-ring)` |
| Tab 可达 | 侧栏/抽屉、设置导航、主按钮、Modal 开关 | ✅ 抽测设置导航 Tab 可达；ConfirmDialog 可开 |
| 表单 | `label`↔控件；错误时 `aria-invalid` + `aria-describedby` | ✅ 运行参数等 Field 页保持（既有实现，未回归） |
| ConfirmDialog | 运行参数清空凭据无 `window.confirm` | ✅ 点击「重置为基线口令」→ ConfirmDialog；无原生 confirm |

## 4.5 程序化门禁

| 项 | 结果 |
|----|------|
| 裸色扫描（tokens/白名单外） | ✅ `noHardcodedColors.test.ts` |
| `npm test` | ✅ 83 files / 487 tests |
| `npx tsc --noEmit` | ✅ |
| `npm run build` | ✅ → `internal/ui/dist` |

## 证据诚实说明（合并前快修）

走查脚本 `web/chat/scripts/p4-walkthrough.mjs` 曾使用**错误路由**：

| 脚本路径 | 问题 | 正确路由（`settingsNav` / `main.tsx`） |
|----------|------|----------------------------------------|
| `settings/assistant` | 无此路由 → SPA `*` 回落到聊天 | `/settings/tools`（助手功能） |
| `settings/connectors` | 无此路由 → SPA `*` 回落到聊天 | `/settings/openapi`（业务系统） |

因此截图 `admin-settings-assistant.png`、`admin-settings-connectors.png` 与 `desktop-light-chat.png` **SHA256 相同**（聊天空态），**不能**当作助手功能页或业务系统（连接）页的验收证据。对应文件仍保留在 `p4-acceptance-shots/`，仅作脚本产物存档，引用时勿当连接/助手功能实拍。

连接组补充：

- **业务系统**（`/settings/openapi`）：本轮**未实拍**（脚本走错 `connectors`，截图错贴聊天）。
- **对外提供能力**（`/settings/mcp-export`）：本轮**未实拍**（脚本路径列表未包含该路由）。
- **外部工具服务**（`/settings/mcp`）：有独立哈希截图 `admin-settings-mcp.png`，可作该页证据。
- **助手功能**：真页证据为 `admin-settings-tools.png`（正确路由 `/settings/tools`），勿用 `admin-settings-assistant.png`。

`desktop-tool-hitl-phone.png` **名不副实**：实为手机宽度下的**空态聊天**（无 tool/HITL 卡），由 `p4-drawer-check.mjs` 在无模型实例上发送失败后截得；与下方 **N2** 已知债对齐——真机 HITL 对话卡需有模型环境补拍，本文件不可当作 C-D4 布局证据。

## 缺陷与修复

| ID | 现象 | 严重度 | 修复提交或「已知债」 |
|----|------|--------|----------------------|
| N1 | 本机 cursor-ide-browser MCP 无法稳定持有 tab | 工具债 | 已知债：改用 Chrome+Puppeteer；不影响产品 |
| N2 | 走查实例未配模型，无法真人 HITL 对话卡；`desktop-tool-hitl-phone.png` 实为空态聊天 | 低 / 环境 | 已知债：C-D4 以 CSS+单测闭合；真机对话可在有模型环境补一眼；该文件名勿当 HITL 证据 |
| N3 | 文案偏好（卡片描述截断等） | 文案 | note only，不阻塞 |
| N4 | 走查脚本错误路径导致 assistant/connectors 截图错贴聊天；业务系统 / 对外提供能力缺真图 | 证据债 | 已知债：见「证据诚实说明」；不以错贴图为证；合并前已诚实化 note |
| — | 本轮无中高优布局/暗色穿帮/无法操作缺陷 | — | 无修复提交 |

## 复测方法与截图

- 实例：见文首；临时配置 `configs/p4-walkthrough.yaml`（可不入库）
- 截图目录：`docs/superpowers/notes/p4-acceptance-shots/`（含 `walkthrough-report.json`）
- 复测：重建前端 → `serve` 隔离端口 → 按 §4.1 六格各开聊天/设置/代表页；运营口令再走总览/技能/微信；运行参数点重置确认对话框
- 备注：微信适配器二进制缺失时徽标「启动异常」为预期，非 UI 回归
- 证据注意：勿把 `admin-settings-assistant.png` / `admin-settings-connectors.png` 当设置页证据；勿把 `desktop-tool-hitl-phone.png` 当 HITL 卡证据（见「证据诚实说明」）

## DoD（对照规格 §7）

- [x] §4 矩阵与清单全部勾选或标明「降级/已知债 + 理由」（见上文；N1–N4 已知债（含证据诚实说明））
- [x] 无未解释的裸景色；`npm test`（487）/`tsc`/`build` 绿（终验 2026-09-13）
- [x] `web/chat/src/style.css` 不存在；聊天样式在 `styles/chat.css`
- [x] Runtime 危险确认走 `ConfirmDialog`；`window.confirm` 仅测试描述/spy，无生产调用
- [x] README 用户导航术语与 `settingsNav` 人话一致
- [x] `internal/ui/dist` 已随构建更新并提交
- [x] 本规格非目标均未纳入本里程碑
