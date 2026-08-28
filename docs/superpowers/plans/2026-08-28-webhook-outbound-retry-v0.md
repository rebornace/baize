# Webhook 出站重试 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。

**目标：** SQLite outbox + 进程内 worker 实现出站 Webhook 指数退避重试与死信；Settings API/UI 可查看并重投。

**架构：** Hub 回调改为 `Enqueue` 写 `webhook_outbox`；`Dispatcher.StartWorker` 拉取 due 行 POST；Store 接口扩展；admin GET/POST deliveries API；WebhookSettings 表格。

**技术栈：** Go 1.25、现有 `internal/webhook`、SQLite/Memory store、React Webhook 页。

**规格：** `docs/superpowers/specs/2026-08-28-webhook-outbound-retry-v0-design.md`  
**Git：** 分支 `feat/webhook-outbound-retry-v0`

---

### 任务 0：分支

```bash
git checkout -b feat/webhook-outbound-retry-v0
```

---

### 任务 1：Store outbox 模型与测试

**文件：** `internal/store/store.go`、`sqlite.go`、`memory.go`、`webhook_outbox_test.go`

- [ ] `WebhookOutboxEntry`、status/kind 常量、`WebhookOutboxMaxAttempts=5`
- [ ] Store 方法：`PutWebhookOutboxIfAbsent`、`ListWebhookOutboxDue`、`ListWebhookOutbox`、`GetWebhookOutbox`、`UpdateWebhookOutbox`、`ResetWebhookOutboxRetry`
- [ ] SQLite 建表 + Memory 实现 + 单测

---

### 任务 2：webhook 退避与 worker

**文件：** `internal/webhook/retry.go`、`retry_test.go`、`dispatcher.go`

- [ ] `Retryable(statusCode, err)`、`Backoff(attempt)`
- [ ] `Enqueue`、`StartWorker`、`processDue`、`deliverOne`
- [ ] `SendTest` / Hub 路径改走 Enqueue
- [ ] 单测：503 重试成功、4xx 直接 dead

---

### 任务 3：bootstrap 启动 worker

**文件：** `internal/bootstrap/bootstrap.go`

- [ ] `newAPIServer` 内 `StartWorker` + shutdown cancel

---

### 任务 4：Settings API + ACL

**文件：** `internal/api/server.go`、`server_webhook_test.go`、`internal/controlplane/acl.go`

- [ ] GET deliveries、POST retry
- [ ] ACL admin；API 测

---

### 任务 5：集成测试

**文件：** `tests/integration/webhook_retry_test.go`

- [ ] 503→200 E2E；全失败 dead 行

---

### 任务 6：WebhookSettings UI

**文件：** `web/chat/src/api.ts`、`WebhookSettings.tsx`、测试、`internal/ui/dist`

- [ ] 列表 + 重投；`npm test` + build dist

---

### 任务 7：文档

- [ ] architecture §3、README.zh-CN 出站可靠性小节

---

### 任务 8：全量验证

```bash
go test ./... -count=1
cd web/chat && npm test && npx tsc --noEmit
gofmt -l .
```

---

## 规格覆盖

| 规格 | 任务 |
|------|------|
| §2 数据模型 | 1 |
| §3 worker | 2, 3 |
| §4 API | 4 |
| §5 UI | 6 |
| §6 测试文档 | 5, 7, 8 |
