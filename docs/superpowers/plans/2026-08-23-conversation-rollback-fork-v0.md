# 对话回滚与 Fork v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：subagent-driven-development 或 executing-plans。

**规格：** `docs/superpowers/specs/2026-08-23-conversation-rollback-fork-v0-design.md`（已批准）

**分支：** `feat/conversation-rollback-fork`

**目标：** Cursor 式回滚（截断后续续聊）与 Fork（前缀复制到新会话）。

**架构：** `TruncateFrom` / `Fork` on `conversation.Store`；`HasActiveRun` on `store.Store`；`POST rollback` / `POST fork`；ChatPage 气泡菜单。

**技术栈：** Go 1.22+、React/Vite。

**全局约束：** commit 中文；UI 后 `npm run build` + `internal/ui/dist/**`

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/conversation/errors.go` | `ErrMessageNotFound` |
| `internal/conversation/store.go` | `TruncateFrom`, `Fork` |
| `internal/conversation/sqlite.go` | SQLite 实现 |
| `internal/conversation/*_test.go` | Store 单测 |
| `internal/store/store.go` | `HasActiveRun` |
| `internal/store/memory.go` / `sqlite.go` | 实现 |
| `internal/api/server.go` | handlers + routes |
| `internal/api/server_conversation_mutate_test.go` | API 测试 |
| `web/chat/src/api.ts` | `rollbackMessages`, `forkConversation` |
| `web/chat/src/components/Composer.tsx` | `draft` 预填 |
| `web/chat/src/pages/ChatPage.tsx` | 气泡菜单 |
| `web/chat/src/index.css` | `.bubble-menu` |

---

### 任务 1：conversation Store

- [ ] `TruncateFrom` / `Fork` Memory + SQLite + 测试
- [ ] Commit `feat(conversation): TruncateFrom 与 Fork`

### 任务 2：HasActiveRun + API

- [ ] `store.HasActiveRun`
- [ ] `POST rollback` / `POST fork` + 测试
- [ ] Commit `feat(api): 对话回滚与 fork 端点`

### 任务 3：Chat UI + dist

- [ ] `api.ts` + `ChatPage` + `Composer` + CSS
- [ ] `npm test` + `npm run build`
- [ ] Commit `feat(ui): 对话回滚与 fork 菜单`

### 任务 4：README

- [ ] 简短操作说明
- [ ] Commit `docs: 对话回滚与 fork`
