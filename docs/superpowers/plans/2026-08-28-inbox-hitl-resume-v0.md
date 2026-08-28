# Inbox HITL resume（I1）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在既有 `POST /v0/inbox/{channel_id}` 上增加 `action: "resume"`，用 Channel HMAC 对 `waiting_human` Run 做 approve/reject，并与 UI resume 安全并存。

**架构：** 验签/限速/幂等复用 Inbox v1；`action` 分支后 resume 路径做归属校验（`run.agent_id == channel.agent_id`），再调用与 `handlePostResume` 相同的 `ContinueFromHITL`；不刷新 passthrough headers。幂等命中时用已存 `run_id` 查当前 status 返回，不重复 ContinueFromHITL。

**技术栈：** Go、现有 `internal/inbox` + `internal/api/server_inbox.go`、`store.InboxDelivery`、集成测试 `tests/integration`。

**规格：** `docs/superpowers/specs/2026-08-28-inbox-hitl-resume-v0-design.md`  
**Git：** 分支 `feat/inbox-hitl-resume-v0`（从最新 `main` 拉出）

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/inbox/model.go` | Payload 增加 Action/RunID/Decision/Comment；Validate 按 action 分支 |
| `internal/inbox/model_test.go` | Payload 校验单测（新建或扩展） |
| `internal/api/server_inbox.go` | `handlePostInbox` 在 Validate 后按 action 分流；`handleInboxResume` |
| `internal/api/server_inbox_test.go` | resume 成功/403/409/幂等/兼容 create |
| `internal/run/events.go`（或现有事件常量处） | 若已有 `EventInboxReceived`，并列加 `EventInboxResumed` |
| `tests/integration/inbox_resume_test.go` | E2E：waiting_human → Inbox resume |
| `README.zh-CN.md` | 机器审批 curl 小节 |
| `examples/inbox-alert/`（可选） | resume 示例脚本片段 |
| `docs/architecture-and-plugin-protocol.md` | Channel 行补一句支持 resume |
| Inbox v1 规格 defer | 标 I1 已实现（规格已有交叉引用时可再核对） |

---

### 任务 0：分支

- [ ] **步骤 1：确保 main 最新并建分支**

```bash
git checkout main
git pull real main
git checkout -b feat/inbox-hitl-resume-v0
```

---

### 任务 1：Payload 模型与校验（TDD）

**文件：**
- 修改：`internal/inbox/model.go`
- 创建或修改：`internal/inbox/model_test.go`

- [ ] **步骤 1：写失败测试**

在 `internal/inbox/model_test.go`：

```go
func TestPayloadValidateCreateRequiresInput(t *testing.T) {
	p := Payload{Action: "", Input: ""}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestPayloadValidateResume(t *testing.T) {
	p := Payload{Action: "resume", RunID: "run_1", Decision: "approve"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Decision = "nope"
	if err := p.Validate(); err == nil {
		t.Fatal("expected invalid decision")
	}
	p = Payload{Action: "resume", Decision: "approve"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected missing run_id")
	}
}

func TestPayloadValidateUnknownAction(t *testing.T) {
	p := Payload{Action: "warp", Input: "x"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/inbox/ -count=1 -run TestPayloadValidate
```

预期：FAIL（字段或分支尚未实现）

- [ ] **步骤 3：最少实现**

在 `Payload` 增加：

```go
Action   string `json:"action,omitempty"`
RunID    string `json:"run_id,omitempty"`
Decision string `json:"decision,omitempty"`
Comment  string `json:"comment,omitempty"`
```

常量（同包）：

```go
const (
	ActionCreateRun = "create_run"
	ActionResume    = "resume"
)
```

重写 `Validate()`：

1. 规范化：`action := strings.TrimSpace(p.Action)`；空则视为 create。  
2. `action` 为 `resume`：`run_id` 非空；`decision` 为 `approve` 或 `reject`；**不**要求 `input`；idempotency_key 长度规则仍适用（若提供）。  
3. `action` 为 `` 或 `create_run`：保持现有 input / idempotency / external_id 规则。  
4. 其它 action → `errors.New("inbox payload: unknown action")`。

- [ ] **步骤 4：测试通过**

```bash
go test ./internal/inbox/ -count=1 -run TestPayloadValidate
```

- [ ] **步骤 5：Commit**

```bash
git add internal/inbox/model.go internal/inbox/model_test.go
git commit -m "feat(inbox): Payload 支持 action=resume 校验"
```

---

### 任务 2：事件常量

**文件：** 查找并修改定义 `EventInboxReceived` 的文件（预期 `internal/run/events.go` 或 `internal/api` 引用处）。

- [ ] **步骤 1：增加常量**

```go
EventInboxResumed = "inbox.resumed"
```

若测试无需单独文件，可与任务 3 同 commit；否则：

```bash
git add <file>
git commit -m "feat(run): 增加 inbox.resumed 事件类型常量"
```

---

### 任务 3：Handler 分流与 resume（TDD）

**文件：**
- 修改：`internal/api/server_inbox.go`
- 修改：`internal/api/server_inbox_test.go`

- [ ] **步骤 1：写失败的 API 测试**

模式对齐现有 `TestInbox*`：`testServerWithInbox`、签名 helper。

覆盖：

1. `TestInboxResumeApprove` — Run 置 `waiting_human` + fakeRunner ContinueFromHITL 成功 → Inbox resume → 200，body 含 `action=resume`、`run_id`；Store 有 `inbox.resumed` 事件。  
2. `TestInboxResumeForbiddenAgent` — Channel agent `a`，Run 属 agent `b` → 403 `run_forbidden`。  
3. `TestInboxResumeNotWaiting` — Run 已 `succeeded` → 409 `not_waiting`。  
4. `TestInboxResumeIdempotent` — 同 key 同 body 第二次不二次调用 ContinueFromHITL（用 counting fakeRunner）。  
5. 既有 create 测试仍绿（回归）。

`waiting` Run 可用 `st.CreateRun` + `st.UpdateRun(..., StatusWaitingHuman, ...)`。

- [ ] **步骤 2：运行确认失败**

```bash
go test ./internal/api/ -count=1 -run TestInboxResume
```

- [ ] **步骤 3：实现 handler**

在 `handlePostInbox` 中，`payload.Validate()` 成功后：

```go
action := strings.TrimSpace(payload.Action)
if action == "" || action == inbox.ActionCreateRun {
    // 现有 create 逻辑不变（可抽成 handleInboxCreateRun）
} else if action == inbox.ActionResume {
    s.handleInboxResume(w, r, channel, channelID, payload, rawBody, bodyHash)
    return
}
```

将现有 create 主体挪到 `handleInboxCreateRun(...)` 以保持文件可读（可选但推荐）。

`handleInboxResume` 逻辑（严格按规格）：

1. 计算 `bodyHash`（调用方已算则可传入）、`idempotencyKey`、`deliveryID`。  
2. 若有 idempotency key：Get 命中且 hash 同 → `waitInboxDeliveryRunID` → 查 Run status → `writeInboxResumeOK`；hash 不同 → 409 conflict。  
3. Put 占位（RunID 空），claimed=true（同 create 并发模式）。  
4. `GetRun(payload.RunID)`；缺失 → 404 `run_not_found`。  
5. `runRec.AgentID != channel.AgentID` → 403 `run_forbidden`。  
6. `runRec.Status != waiting_human` → 409 `not_waiting`。  
7. `ContinueFromHITL`（approve/reject + comment）；**不** `SetPassthroughHeaders`。  
8. `AppendEvent` type `inbox.resumed`，data：`channel_id`, `delivery_id`, `decision`, 可选 `comment`。  
9. UpdateInboxDelivery 填 RunID；`writeInboxResumeOK` 200。

```go
func writeInboxResumeOK(w http.ResponseWriter, deliveryID, runID, status string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"delivery_id": deliveryID,
		"run_id":      runID,
		"status":      status,
		"action":      "resume",
	})
}
```

幂等重放：用已存 `RunID` 再 `GetRun` 取**当前** `status`（规格允许）。

- [ ] **步骤 4：测试通过**

```bash
go test ./internal/api/ -count=1 -run 'TestInbox'
```

- [ ] **步骤 5：Commit**

```bash
git add internal/api/server_inbox.go internal/api/server_inbox_test.go internal/run/
git commit -m "feat(api): Inbox action=resume 推进 HITL"
```

---

### 任务 4：集成测试

**文件：**
- 创建：`tests/integration/inbox_resume_test.go`

- [ ] **步骤 1：编写 E2E**

复用 `bootstrap.StartForTest` + mock LLM；配置 Channel；若默认 demo 工具需审批，发 Inbox create 使 Run 进入 `waiting_human`（或直接 Store 更新后仅测 resume 路径——**优先真实路径**：input 触发 `create_ticket` 类审批）。

流程：

1. 签名 POST create_run → 得 `run_id`  
2. poll 至 `waiting_human`  
3. 签名 POST resume approve  
4. poll 至 `succeeded`（或引擎继续后的终态）  
5. 再 POST 同 resume body → 200 且不破坏状态  

- [ ] **步骤 2：运行**

```bash
go test ./tests/integration/ -count=1 -run TestInboxResume -timeout 120s
```

- [ ] **步骤 3：Commit**

```bash
git add tests/integration/inbox_resume_test.go
git commit -m "test(integration): Inbox HITL resume E2E"
```

---

### 任务 5：文档与交叉引用

**文件：**
- `README.zh-CN.md` — 生产集成节增加「机器审批」curl（签名同 create，body 换 resume 字段）  
- `docs/architecture-and-plugin-protocol.md` — Channel 行补「支持 `action=resume`」  
- `docs/superpowers/specs/2026-08-28-webhook-inbox-v1-design.md` — defer 行改为**已实现**并链到 I1 规格  
- 可选：`examples/inbox-alert/README.md` 增加 resume 一句

- [ ] **步骤 1：改文档**  
- [ ] **步骤 2：Commit**

```bash
git add README.zh-CN.md docs/
git commit -m "docs: Inbox 机器审批（HITL resume）说明"
```

---

### 任务 6：全量验证与合并

- [ ] **步骤 1：测试与格式**

```bash
go test ./... -count=1
gofmt -l .
# 若有脏文件：gofmt -w <files>
```

- [ ] **步骤 2：合并推送（需用户明确要求时再执行）**

```bash
git checkout main
git merge --no-ff feat/inbox-hitl-resume-v0 -m "merge: Inbox HITL resume（I1）"
git push real main
# 开源仓：.\scripts\export-public.ps1 后推 public（用户要求时）
```

---

## 规格覆盖

| 规格章节 | 任务 |
|----------|------|
| §1 目标/不做 | 全任务边界 |
| §2 API/错误码 | 1, 3 |
| §3 流程/归属/无 passthrough | 3 |
| §4 事件 | 2, 3 |
| §5 测试文档 | 3, 4, 5 |
| §6 幂等不重复 ContinueFromHITL | 3 |

## 自检

- 无 TODO/待定占位；幂等重放 status 现查已写明。  
- create 默认 action 与 Validate 分支一致。  
- 不含 P4/PG、不含 F。

---

计划保存于 `docs/superpowers/plans/2026-08-28-inbox-hitl-resume-v0.md`。
