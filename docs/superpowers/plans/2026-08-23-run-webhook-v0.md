# Run Webhook v0 实现计划

> **工作者：** superpowers:subagent-driven-development

**目标：** Run 事件 Webhook 出站 + 设置页 UI + 聊天高级覆盖；全功能 UI 可操作。

**规格：** `docs/superpowers/specs/2026-08-23-run-webhook-v0-design.md`（已批准）

**全局约束：**
- 分支 `feat/run-webhook`
- commit 中文；UI 后 `npm ci && npm test && npm run build` + dist
- `$env:GOPROXY='https://goproxy.cn,direct'`；Go PATH 含 sdk
- minimal.yaml 默认 webhook 空；demo 可不预置

---

### 任务 0：`git checkout -b feat/run-webhook`

---

### 任务 1：Store — settings KV + run webhook_json

- `settings` 表 `key`/`value_json`；`GetSetting`/`UpsertSetting`
- `runs.webhook_json`；`CreateRunInput.WebhookConfig`
- 测试 round-trip

---

### 任务 2：`internal/webhook` Dispatcher

- 订阅 Hub Publish/PublishEnd 或扩展 notifyingStore
- 异步 POST 30s；payload 对齐 SSE
- `UpdateConfig(url, headers)` 热更新
- httptest 单测

---

### 任务 3：API + bootstrap

- GET/PUT `/v0/settings/events-webhook`
- POST `/v0/settings/events-webhook/test`
- `handlePostRun` webhook 字段
- bootstrap：yaml 默认写入 store（若 store 空）；Dispatcher 注入 Server
- `server_*_test.go`

---

### 任务 4：集成测试

- `tests/integration/webhook_test.go`

---

### 任务 5：WebhookSettings UI

- `api.ts`：get/put/test events webhook；`createRun` 可选 webhook
- `WebhookSettings.tsx` + test
- `settingsNav` + `main.tsx` 路由
- `ChatPage` 高级折叠 per-run URL
- build dist

---

### 任务 6：文档

- architecture §3、README 中英

---

### 任务 7：`go test ./...` 全绿
