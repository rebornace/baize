# HTTP 插件设置页 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。

**目标：** `/settings/plugins` 替换 ComingSoon，管理员可 PUT `type:http` 侧车 Connector。

**架构：** 复用 `getConnector` / `putConnector`；UI 对称 `McpSettings`；`source=plugin` 聚合列表。

**技术栈：** React + Vite、现有 api 层、Go 后端无变更（除非测试缺口）。

**规格：** `docs/superpowers/specs/2026-08-23-http-plugin-settings-v0-design.md`

**全局约束：**
- 分支 `feat/plugin-settings`
- commit 中文 `type(scope): 说明`
- UI 变更后 `cd web/chat && npm ci && npm test && npm run build`，提交 `internal/ui/dist/**`
- 不做 capture 模式表单、不装侧车进程

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `web/chat/src/api.ts` | `ConnectorAuth`、扩展 `ConnectorInfo`、`putConnector` |
| `web/chat/src/pages/PluginSettings.tsx` | 设置页 |
| `web/chat/src/pages/PluginSettings.test.ts` | 纯函数测试 |
| `web/chat/src/main.tsx` | 路由 |
| `README.md` / `README.zh-CN.md` | 设置页说明 |

---

### 任务 0：分支

- [ ] `git checkout -b feat/plugin-settings`

---

### 任务 1：api.ts 类型与 putConnector

- [ ] 扩展 `ConnectorInfo`、`ConnectorAuth`、`putConnector` body
- [ ] `npm test`（现有测试仍绿）

---

### 任务 2：PluginSettings 页面

- [ ] 实现 `PluginSettings.tsx`（列表 + 抽屉 + 保存 + 错误码）
- [ ] `PluginSettings.test.ts`
- [ ] `main.tsx` 替换 ComingSoon
- [ ] `npm test`；`npm run build`；提交 dist
- [ ] Commit `feat(ui): HTTP 插件设置页`

---

### 任务 3：README

- [ ] 更新中英 README 设置 → 插件 说明
- [ ] Commit `docs: HTTP 插件设置页说明`

---

### 任务 4：回归

- [ ] `go test ./... -count=1`
