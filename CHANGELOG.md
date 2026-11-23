# Changelog

## 0.37.0 — 2026-11-23

### Workflow

- **Workflow editor**：`GET/PUT/DELETE /v0/skills/{id}/workflow`；用户技能可在 Settings 编辑线性步骤、参数 JSON、审批开关、失败重试次数与 `when` 条件（leaf / all / any / not）。
- 新增原始 YAML 保存（`PUT .../workflow/raw`）、渲染预览（`POST .../workflow/preview`）、内置技能复制为用户覆盖（`POST .../workflow/clone`）。
- 步骤 `retry`（0–5）：工具 `is_error` 时立即重试并发 `workflow.step_retry`；登录/审批暂停、人工拒绝与取消不重试。
- 步骤 `timeout_ms`（0–600000）：单次工具调用超时后取消并发 `workflow.step_timeout`；超时按失败处理，可配合 `retry`。
- 步骤 `continue_on_error`：失败记为 `{"is_error":true}` 并继续后续步骤，发 `workflow.step_failed`；完成事件状态为 `partial`。登录/审批暂停、人工拒绝与取消仍停止。
- 保存前校验：步骤 ID 唯一且不含点号、工具存在、模板引用只能来自 `input` 或前序步骤；内置 / 托管技能只读。
- 支持 `{{input.xxx}}` / `{{step.result.xxx}}`，仍然不支持循环或任意脚本。

### Skills

- **Domain guard**：`SKILL.md` 支持 `requires_tools` / `requires_input`；技能列表显示缺失工具，保存 agent 默认技能时阻止缺工具，运行时激活与 workflow 启动前再次校验。
- 域专用技能作为账号默认时返回警告；必需输入缺失时在第一次工具调用前失败，并给出缺失键名。
- 技能卡片显示 domain、必需工具、必需输入与缺失状态；Workflow 编辑器仅对用户技能开放写操作。

### Goal

- `goal_complete` 在当前目标有 `criteria` 时必须带 `evidence`；缺失时拒绝完成，栈不变。
- 完成时发 `goal.completed`，记录目标、标准与证据；GoalBar 详情显示完成标准与数量。
- GoalBar 编辑面板可直接修改完成标准（每行一条，最多 8 条）与轮次/Token 预算；保存根目标或焦点时一并写回，避免改标题时清空标准，也便于预算暂停后调高预算再继续。
- **Goal 轮次/Token 预算**：`max_turns` / `max_tokens` 可设在根目标/焦点；每轮 LLM 调用计数并累计上报 token，达到上限自动暂停目标、结束当前 Run 并提示恢复；发 `goal.budget_exhausted`（含 `budget`/`limit`/`used`）。GoalBar 显示 `used/max`。
- 子运行不能编辑父目标栈：工具列表已过滤，handler 仍会拒绝直接发出的 goal 工具调用；子运行通过自身输出汇报。

### Agents

- **LASE gate v1**：`run.lase_gate`（默认关）开启后，`spawn_group_run` 必须带 `lase.confidence` / `evidence_conflict` / `reason`。
- 置信度 ≥ `lase_confidence_threshold` 且无证据冲突 → 不 spawn，由 leader 直接处理；有冲突才允许子运行。
- 每次决策发 `lase.decision`（allow/skip、置信度、阈值、理由）；不做 voting/MAD，不替代 HITL。

### Postgres

- 多副本启动用 `pg_advisory_lock` 串行化 schema 迁移（60s 获取超时），锁连接独立于业务池。
- 新增 [Postgres 运维文档](./docs/developers/postgres-ops.md)：逻辑备份/恢复、WAL 归档、PITR、failover、监控与多副本限制。

### Schedules

- 新增 `POST /v0/settings/schedules/{id}/run`：立即触发一次运行，不修改 `next_run_at` / 状态；会话忙或无历史运行时返回 409。Settings → 日程新增“立即运行”。

### 证据

- 记录：[`docs/superpowers/notes/2026-11-23-v0.37.0-workflow-editor.md`](docs/superpowers/notes/2026-11-23-v0.37.0-workflow-editor.md)

## 0.36.0 — 未单独发布（并入 0.37.0）

### Workflow

- **完成摘要**：workflow 结束时发 `workflow.completed`，包含每步状态与允许的结果字段（`id` / `title` / `name` / `status`）及人类可读摘要；成功、跳过、失败三类都有输出，失败时标注阻塞步骤与未执行步骤。
- Run 最终输出由固定 `workflow completed` 改为摘要：`3 completed` / `2 completed, 1 skipped` / `failed at step "x"`。
- 摘要只复制白名单标量字段并截断，完整工具响应不进入聊天或事件。
- **敏感文本清洗**：`Authorization` / `api_key` / `token=` / `secret:` / `Bearer` / `sk-` 等形状在 Run error、`llm.error`、`workflow.completed.error` 与工具结果事件中打码。
- 前端 workflow 卡片展开后显示步骤 id、工具、结果标题与跳过状态；旧事件仍可折叠。

### Mobile RT

- **流恢复与重试**：新增 `RetryStream()`，错误/取消后复用最后一条 user 消息，不追加重复轮次；错误状态保留已收 `text`。
- Android 空壳用进程级会话表跨 Activity 重建复用同一 `Agent`；回前台先 `pollStream`，仍在流式就继续拉取。
- 进程被杀时从会话快照恢复并显示“流中断、部分回复已保留”，不静默清空。
- **附件元数据**：`SendWithAttachments` / `StartStreamWithAttachments` / `SessionAttachments`；快照保留名称 / MIME / 大小 / `local_ref`，文件本体不入库；恢复后可显示附件与缺失状态。

### WebUI

- **待审批 SSE 事件总线**：HITL 设置/清除时进程内广播，空闲时 30s 兜底重查；不再每连接 2s 查库。连接限额、慢客户端超时与 ACL 过滤不变。
- **跨副本待审批 nudge**：redis 事件总线用保留 run id 传播 pending 变化；其他副本只唤醒本地 SSE，不回写总线。

### 供应链

- `sign-release.ps1` 支持 `-ExpectedFingerprint`；签名前校验公钥 DER SPKI SHA256 指纹，不匹配直接失败。
- 签名后写入 `cosign-key-fingerprint.txt`（公钥名、指纹、签名时间），随 Release 上传。
- 新增 [发布签名与密钥交接](./docs/developers/release-signing.md)：生成、离线保管、交接、轮换、吊销/泄露、CI 边界与验证清单。
- 新增 `scripts/rotate-release-key.ps1`：生成到独立目录、拒绝覆盖、`-CheckOnly` 指纹校验、写 `KEY-HANDOFF.txt`；与签名脚本共用指纹实现。

### 评测

- `expect_state` 统一为 `fields`：点号访问嵌套对象，数组用数字下标；`value` 为标量时按 `eq`，`op` 支持 `eq` / `contains` / `regex`。
- 移除 `title_contains`、顶层 `id` / `status` 与 `tickets` 快照信封，避免维护多套等价写法。
- `POST .../check-state` 只接受 `state_url`；删除 `mock_base_url` 兼容路径。
- Go 与 PowerShell 评分器、示例 cases、fixtures、前端类型同步迁移。

### 证据

- 真实 API：`project-create` 三条链路——全部完成、条件跳过、模板渲染失败；Run 输出与 `workflow.completed` 状态一致。
- 记录：[`docs/superpowers/notes/eval-artifacts/workflow-completion-output-20261122/report.md`](docs/superpowers/notes/eval-artifacts/workflow-completion-output-20261122/report.md)
- 评测状态路径：真实 API 用嵌套对象、数组下标、`eq` / `contains` / `regex` 命中并 `last_score_pass=true`；非法 op 返回 400，`mock_base_url` 不再生效。
- 记录：[`docs/superpowers/notes/eval-artifacts/eval-state-paths-20261122/report.md`](docs/superpowers/notes/eval-artifacts/eval-state-paths-20261122/report.md)

## 0.35.0 — 2026-11-22

### Mobile RT

- **会话快照 / 恢复**：SDK 新增 `ExportSession` / `ImportSession`；只含消息历史、条数与网络策略，不含 API key、connector auth、pending 或 stream。
- 导出会截断未完成的 tool call，避免悬空 `tool_call_id`；恢复校验角色与 tool 配对，上限 4 MiB / 2000 条。
- Android 空壳把每个会话快照存入 `EncryptedSharedPreferences`，启动时恢复会话列表、名称与文本；pending 明确失效。
- **私网 allowlist**：新增 `SetPrivateNetworkAllowlist` 与远程配置 `allowed_private_hosts` / `allowed_private_cidrs`；CIDR 必需，hosts 可选，默认仍拒绝。拨号时校验 IP/CIDR，重定向逐跳校验。

### WebUI

- **待审批 SSE 限额**：每进程最多 100 路；超限返回 429 + `Retry-After`；慢客户端 5s 写超时断开；`GET /v0/hitl/pending` 返回 `stream_active` / `stream_dropped` / `stream_slow`。

### 供应链

- **cosign 签名**：`scripts/sign-release.ps1` 对 `SHA256SUMS.txt` 与 OCI 归档生成 `*.sigstore.json`；离线 `verify-blob` 通过；私钥不入库。
- **许可证报告**：Go 依赖 + Alpine 包共 93 个；无 Go GPL/AGPL，复核项均来自基础镜像工具。

### Workflow

- **条件步骤**：`workflow.yaml` 步骤支持 `when`（`path` + `op` + `value`，或 `all` / `any` / `not`）；不满足时发 `workflow.step_skipped` 并继续。
- 支持 `eq` / `ne` / `contains` / `in` / `exists` / `not_exists` / `gt` / `gte` / `lt` / `lte`；无循环、无任意脚本。
- **结构化 workflow 输入**：`POST /v0/runs` 可带 `input_args`（≤64 键 / 64 KiB），与 `input.text` 一起进入模板；`text` 不可覆盖。该字段只用于 workflow，不直接进入模型对话。
- **通用条件建/改示例**：`examples/skills/project-create` 用 `input_args.project_title` 创建项目，仅在 `target_status=archived` 时更新状态；展示条件步骤与结构化输入，不绑定工单域。
- `ticket-resolve` 暂未包含：当前 workflow 输入只有 `input.text`，无法稳定取得 id/状态，硬做会误改数据。

### 评测

- **状态检查通用化**：`check-state` 接受完整 `state_url`；响应支持 `records` / `items` / `data` / 旧 `tickets` / 裸数组；`expect_state` 增加 `fields` 任意字段包含匹配。
- 产品 UI 的默认 agent 回退、评测文案与示例 placeholder 去掉 ticket；ticket 只保留在示例、测试和 mock fixture。

### 证据

- 公共 SDK：快照 round-trip、继续对话、pending 截断；全量测试与 Android 构建通过。
- 记录：[`docs/superpowers/notes/eval-artifacts/mobile-session-store-20261122/report.md`](docs/superpowers/notes/eval-artifacts/mobile-session-store-20261122/report.md)
- 私网 allowlist：默认拒绝、CIDR 放行、host 限制、非法配置、远程配置五条路径通过；Android 构建通过。
- 记录：[`docs/superpowers/notes/eval-artifacts/mobile-net-allowlist-20261122/report.md`](docs/superpowers/notes/eval-artifacts/mobile-net-allowlist-20261122/report.md)
- 签名与许可证：`SHA256SUMS.txt` 与 OCI 归档 `Verified OK`；许可证报告 93 包、10 个复核项。
- 记录：[`docs/superpowers/notes/eval-artifacts/supply-chain-sign-20261122/report.md`](docs/superpowers/notes/eval-artifacts/supply-chain-sign-20261122/report.md)
- 条件 workflow：`cond-skip` 跳过 create 并 `succeeded`；`cond-run` 两步执行并建单；单测覆盖操作符与非法配置。
- 记录：[`docs/superpowers/notes/eval-artifacts/workflow-conditional-20261122/report.md`](docs/superpowers/notes/eval-artifacts/workflow-conditional-20261122/report.md)
- SSE 限额：限额 1 时第二条 429、`dropped=1`；取消后 active 回 0；正常快照不变。
- 记录：[`docs/superpowers/notes/eval-artifacts/pending-sse-limits-20261122/report.md`](docs/superpowers/notes/eval-artifacts/pending-sse-limits-20261122/report.md)
- 评测去 ticket 化：`records/items/data/tickets` 与裸数组均可解析，`fields` 匹配通过；全量 Go + 前端 862 tests。
- 记录：[`docs/superpowers/notes/eval-artifacts/eval-generic-state-20261122/report.md`](docs/superpowers/notes/eval-artifacts/eval-generic-state-20261122/report.md)
- 结构化 workflow 输入：真实 API 带 `input_args` 渲染到工具参数并建单；Job 透传与 64 键/64 KiB 限额通过。
- 记录：[`docs/superpowers/notes/eval-artifacts/structured-workflow-input-20261122/report.md`](docs/superpowers/notes/eval-artifacts/structured-workflow-input-20261122/report.md)
- 通用条件建/改示例：`project-create` 在 `archived` 时执行 create→update，`active` 时跳过 archive；两次 Run 均成功。
- 记录：[`docs/superpowers/notes/eval-artifacts/project-conditional-example-20261122/report.md`](docs/superpowers/notes/eval-artifacts/project-conditional-example-20261122/report.md)

## 0.34.0 — 2026-11-22

`0.34.0`：Mobile 私网边界 / 流式 / 多会话、Docker 多架构、Postgres 运维、待审批 SSE 与依赖安全。

### Mobile RT

- **私网边界（默认拒绝）**：`http_get`、`FetchRemoteConfig`、`spec_url` 与所有连接器调用默认拒绝回环、私网、链路本地、CGNAT、组播和保留地址。
- **流式回复**：`StartStream` / `PollStream` / `CancelStream`；SSE 增量、非流式 JSON 回退、审批暂停即结束当前流、取消等待中的 LLM 请求；原 `Send` 行为与返回值不变。
- **多会话**：`NewAgent()` / `NewSession()`；每个会话独立消息、流与待审批，`PendingApproval()` 用于切换时恢复审批卡片；`NewSession` 复制配置与连接器但不复制历史。
- 校验发生在 DNS 解析后的实际拨号地址，重定向与 DNS rebinding 不能绕过；这些请求不使用环境 HTTP 代理。
- 显式开启：`SetAllowPrivateNetworks(true)`（`Agent` 同名方法）或远程配置顶层 `"allow_private_networks": true`；不会被 `allow_writes` / `methods` 隐式打开。
- 错误信息保持通用，不暴露内网地址是否存在。

### WebUI

- **待审批中心 SSE**：新增 `GET /v0/hitl/pending/stream`；服务端每 2s 推送变化快照，前端流式接收，断线自动重连并回退 15s 轮询；toast / 桌面通知逻辑复用原列表。

### 部署
- **Docker 镜像**：运行镜像使用非 root 用户 `baize`，`/app/data` 数据卷，`HEALTHCHECK` 指向 `/healthz`，`STOPSIGNAL SIGTERM`。
- **多架构 / SBOM**：`buildx` 产出 `linux/amd64` + `linux/arm64` OCI，附 SPDX SBOM 与 SLSA provenance；`apk upgrade` 清掉 OS 层漏洞。
- compose 增加 `restart: unless-stopped`、`no-new-privileges`、可选 `BAIZE_ADMIN_TOKEN` / `BAIZE_SETTINGS_KEY`。
- `.dockerignore` 排除公开导出、发布目录、根 `node_modules` 等构建上下文。
- **Postgres 启动检查**：迁移后核对关键表/列；缺表或缺列时启动即失败，给出“备份后手工迁移”的可操作错误，不再等到查询时才报 SQL 错。

### 安全 / 依赖

- **Go 1.27.3**：`go.mod` 升级到 `go 1.26.0` + `toolchain go1.27.3`；CI 与 Docker 构建镜像同步到 1.27.3。
- **依赖升级**：kin-openapi 0.153、pgx 5.13、grpc 1.83.2、x/net 0.63、excelize 2.11.1、otel 1.47。
- **镜像扫描**：Trivy `HIGH,CRITICAL` 为 **0**（OS 0 / baize 0 / weixin-adapter 0）。

### 证据

- 真实链路（公共 SDK + mock-ticket）：默认 `http_get`、连接器、远程配置均被拒；显式开启后 `http_get` 200、连接器写入成功。
- 记录：[`docs/superpowers/notes/eval-artifacts/netguard-20261122/report.md`](docs/superpowers/notes/eval-artifacts/netguard-20261122/report.md)
- Docker：镜像 24.8 MB，`healthy`，uid 100，容器内 Run `succeeded` 且导出正常；compose `Up (healthy)`。
- 记录：[`docs/superpowers/notes/eval-artifacts/docker-image-20261122/report.md`](docs/superpowers/notes/eval-artifacts/docker-image-20261122/report.md)
- Postgres：0.32 / 0.33 库升级后旧 Run 与导出正常；备份恢复计数一致；缺列库启动退出 1 且错误可读。
- 记录：[`docs/superpowers/notes/eval-artifacts/pg-ops-20261122/report.md`](docs/superpowers/notes/eval-artifacts/pg-ops-20261122/report.md)
- 流式：真实 `deepseek-chat` SSE 返回 3 个增量，最终文本与预期一致；单测覆盖非流式回退、审批暂停与取消。
- 记录：[`docs/superpowers/notes/eval-artifacts/mobile-stream-20261122/report.md`](docs/superpowers/notes/eval-artifacts/mobile-stream-20261122/report.md)
- 多会话：历史与 pending 隔离、clone 复制配置/工具；空壳 Spinner 切换 + New/Clear，Android 构建通过。
- 记录：[`docs/superpowers/notes/eval-artifacts/mobile-sessions-20261122/report.md`](docs/superpowers/notes/eval-artifacts/mobile-sessions-20261122/report.md)
- 待审批 SSE：真实 pinned workflow 从 `count=1` 推送到审批后 `count=0`；Go/前端解析测试通过。
- 记录：[`docs/superpowers/notes/eval-artifacts/pending-sse-20261122/report.md`](docs/superpowers/notes/eval-artifacts/pending-sse-20261122/report.md)
- 镜像验收：多架构 + SBOM/provenance 通过；依赖升级后 Trivy `HIGH,CRITICAL` 为 **0**。记录：[`docs/superpowers/notes/eval-artifacts/dep-sec-20261122/report.md`](docs/superpowers/notes/eval-artifacts/dep-sec-20261122/report.md)

## 0.33.0 — 2026-11-22

会话导出 v1 与 EV-PRES hold-out 收口。

### 新增

- **会话导出 v1**：`GET /v0/conversations/{id}/export` 支持 `format=markdown|json|html` 与 `include_thinking=1`；默认仍是 Markdown。
- **UI**：侧边栏「导出」打开对话框，选择格式与是否包含思考；按格式下载 `.md` / `.json` / `.html`。
- **附件清单**：从消息媒体引用提取文件名，按消息列出；不含内部媒体 URL、不嵌入附件字节。
- **HTML**：单文件、内联 CSS、无脚本；内容/思考/文件名全部转义。
- **兼容**：不带参数的行为与文件名保持 v0 不变。

### Mobile RT v2.1

- **方法白名单**：每连接器可选 `methods`（GET/POST/PUT/PATCH），非空时覆盖 `allow_writes`；DELETE 仍不支持。
- **表单请求**：支持 `application/x-www-form-urlencoded` 对象 schema。
- **multipart**：支持文本字段与 `format: binary` 文件字段；文件以 base64 传入，可配 `<字段>_filename`；单文件 512 KiB、整体 1 MiB。
- **审批预览**：multipart summary 只显示字段与文件名，不回显 base64。

### A5 / A9

- **`followup_create`**：一次性后续任务，`in_minutes` / `fire_at` 二选一，需人工批准；不出现在设置页日程，也不占活跃日程上限。
- **渠道发送策略**：`send_assistant_replies: "false"` 可静态静音助手最终回复；运营镜像与审批通知不受影响。
- **显式 `ticket-create` 技能**：opt-in workflow（先 list、再 create，create 需审批），不加入默认 agent skills；强制激活验证可停在审批。
- **非工单域 hold-out**：新增 `examples/mock-project` 与 7 条 project 用例 × 3 轮；baseline 21/21、candidate 20/21，发现 1 个间歇性 new fail（域专用技能作账号级默认会偶发带偏）。
- **CI eval 门禁**：新增 `evals` job，跑 `test_summarize.py` 与 evidence-eval `verify-score.ps1`。
- **pinned workflow 自动执行**：`@mention` / `run.skills` 显式指定的 workflow 技能在首轮 LLM 前执行；agent 默认技能保持原语义。
- **技能 `domain` 元数据**：`SKILL.md` 可选 `domain`；`ticket-triage` / `ticket-create` 标记为 ticket。设置页提示域专用技能优先按需 `@` 激活，不建议作账号默认。

### 证据

- 4 条新 hold-out 用例 × 2 轮 = 8 对；模型 `deepseek-chat`。
- baseline **8/8**，candidate **6/8**；**0 new pass / 2 new fail**。
- 失败根因：技能 workflow 固定 `list → create`，只读请求也会尝试建单；写步骤 `approve: true` 使 Run 停在审批，后续链路中断。
- 成本差异（token -0.1%、时长 -10.3%）来自失败提前结束，不是效率提升。
- 决策：不晋升当前 `ticket-triage`；先拆纯提示版/条件 workflow。
- 记录：[`docs/superpowers/notes/eval-artifacts/ev-pres-holdout-20261121-2130/report.md`](docs/superpowers/notes/eval-artifacts/ev-pres-holdout-20261121-2130/report.md)

### 修复

- **移除 `ticket-triage/workflow.yaml`**：去掉固定 `list → create`，改为纯提示技能；只读请求不再触发建单尝试，写操作只在用户明确要求时发生。
- evidence-eval 将 workflow 的 `hitl.waiting` 合成为一次工具尝试，符合“waiting_human 算 consequential 尝试”的既有口径；避免只读请求的建单尝试被漏判。
- 集成测试改用本地 workflow fixture，不再依赖示例技能；`catalog_test` 锁定 `ticket-triage` 必须保持 prompt-only。

### 验证

- 第三轮（同一 hold-out）：两臂 **8/8**，**0 new pass / 0 new fail**；token +0.6%，回归消失。
- 记录：[`docs/superpowers/notes/eval-artifacts/ev-pres-holdout-r3-20261121-2145/report.md`](docs/superpowers/notes/eval-artifacts/ev-pres-holdout-r3-20261121-2145/report.md)
- `verify-score.ps1`：9 fixtures / 0 failed；第二轮 `--fail-on-new-fail` 退出 1，第三轮退出 0。
- **发布前真实环境回归**：Postgres 16 + mock-ticket。pinned workflow 审批通过/拒绝、Markdown/JSON/HTML 导出、进程重启后持久化、Mobile `methods` 白名单与审批后真实写入均通过。记录：[`docs/superpowers/notes/eval-artifacts/v0.33.0-pg-regress-20261122/report.md`](docs/superpowers/notes/eval-artifacts/v0.33.0-pg-regress-20261122/report.md)

## 0.32.0 — 2026-11-21

> 汇总 `v0.31.0` 之后的所有改动。本地开发切片编号仅保留在规格/账本中，不作为发布版本号。

### Chat / WebUI

- **全局消息搜索**：跨会话搜索聊天记录，FTS/ILIKE/Memory 三种实现，按会话 ACL 过滤；侧边栏「搜索消息」250ms 防抖。
- **搜索筛选**：角色（你 / 助手 / 系统）、最近 7/30/90 天、仅当前会话；过滤在 store 的 `LIMIT` 之前生效。
- **命中定位**：点击结果滚动到具体消息并高亮 2.2 秒；3 秒内找不到只打开会话，不报错。
- **会话导出**：侧边栏「导出」下载 Markdown，含标题、时间与消息；不含 thinking 和内部媒体链接，附件不嵌入。
- **待审批中心**：跨会话汇总 `waiting_human`，侧边栏计数、15 秒轮询、一键跳回。
- **面板内审批**：直接同意/拒绝，展示工具参数，可选留言；拒绝二次确认，失败行内提示并刷新。
- **新待办提醒**：新出现的等待项 toast + 标题 `(N)` 计数；首次加载静默。
- **桌面通知**：待审批面板内可选开启系统通知；仅窗口隐藏且出现新待办时提醒，点击回到面板；无权限时静默降级。
- **记忆设置页分页**：首屏 50 条、加载更多、跨页选择、刷新保留深度，并明确“只基于已加载项”。
- **记忆后端搜索**：`GET /v0/settings/memory` 支持 `q` / `source`，三 store 过滤后分页；前端防抖并丢弃旧响应。
- **浏览器收口修复**：托管登录成功页说明“回到对话点我已登录，继续”，避免误以为对话会自动恢复。
- **搜索 FTS 修复**：SQLite FTS5 `MATCH` 改用表名，LIKE 回退成功后不再返回陈旧 FTS 错误。

### 渠道 / HITL

- **忙时文本合并**：会话有活跃 Run 时，后续文本进入待合并队列，上一轮结束后自动起一轮后续 Run；审批优先，4000 runes 上限。
- **合并持久化**：`channel_pending` 保存批次，重启恢复；Run 创建成功后才删除，崩溃时 at-least-once。
- **附件合并**：忙时文本与附件一起排队并持久化；每会话批次 10 MiB 上限，超限回退固定文案；旧表自动迁移。
- **HITL 审批链接**：渠道审批提醒可附短时签名页 `GET/POST /v0/hitl/approvals/{token}`，手机打开即可同意/拒绝；绑定 run、等待起点与到期。
- **Webhook 入站硬化**：验签后立即 202，原始消息落 `webhook_inbound_jobs`，附件/建 Run 交给可重试 worker；去重 key、重启恢复、HTTP 超时。
- **死信可见与重试**：`GET /v0/settings/channels/inbound-jobs` 列出 dead 任务，设置页可一键重试；不回显原始消息体。
- **日程 `in_minutes`**：`schedule_create` 的 `kind=once` 支持 1–43200 分钟相对时间，与 `fire_at` 互斥。
- **A5 日程 claim 租约**：`ClaimDue` 写入 2 分钟 `claim_expires_at`，启动/每轮扫描只回收过期占位；旧表自动补列，`SetRunningRunID` 不覆盖其他 Run。
- **A9 出站 dead 可见**：渠道永久失败写 `channel.outbound.dead` 到 Run 时间线，Chat 显示系统提示；只含 channel/kind/attempts/错误摘要，不含正文；设置页重试保留。

### Mobile RT

- **机上工具面 v0**：设备端解析 OpenAPI 3 GET，支持 path/query、operationId、summary/description 和单个认证头；远程工具先审批后执行。
- **演示连接器**：`cmd/mobilebind/demo-connector` 单进程提供配置、spec、票据 API 与 `X-API-Key` 校验；空壳显示工具名。
- **配置恢复**：空壳启动自动重拉已保存 config URL，失败静默保留本地 LLM 配置，防重复请求。
- **多连接器**：`readonly_apis` 数组、`ConfigureReadOnlyAPIs`、全局唯一工具名、8 连接器 / 100 工具上限、独立 base/auth、整批原子替换。
- **连接器写操作**：每个连接器可选 `allow_writes`，解析 POST/PUT/PATCH；JSON body 对象展开、`$ref`、数组/标量用 `body`；每次调用先审批，`method` / `write` 展示；写请求不跟随 3xx，body 上限 64 KiB。
- **Android M3**：Play 上架工程（图标、启动、Manifest、Gradle Wrapper、签名模板、AAB 脚本、商店资料、校验脚本）；签名/提审仍需 Play Console 账号。

### 评测 / 质量

- **EV-PRES 实战**：`ticket-triage` 技能 5 案 × 2 轮真实 `deepseek-chat` 批跑；两臂 9/10，0 新过/新挂，candidate token +43.9%、时长 +24.3%；本语料不晋升该技能。证据在 [`docs/superpowers/notes/eval-artifacts/ev-pres-20261121-2041/`](docs/superpowers/notes/eval-artifacts/ev-pres-20261121-2041/)。
- **Evidence-eval 修复**：读取 `tool.result.tool_call_id`（兼容旧 `id`）；PowerShell 5.1 用 `curl` + UTF-8 读取 mock 状态；修复 `$id:` 变量解析。
- **质量收口**：`golangci-lint` / `go vet` 清零，修正审计与架构文档漂移，死信设置页真实浏览器走查。

### 验证

- `go test -p 1 ./...`、`go vet ./...`、`golangci-lint run ./...` 通过。
- 前端 855 测试、`tsc`、`lint`、`build` 通过。
- 真实浏览器走查：待审批、桌面通知、搜索筛选/定位、会话导出、死信页。
- Android AAR 重建与 `assembleDebug` 通过；发布资源检查通过。
- EV-PRES：`verify-score.ps1` 9 fixtures 全过；`test_summarize.py` 20 tests OK；`--fail-on-new-fail` 退出 0。
- 发布记录见 [`docs/superpowers/notes/2026-11-21-release-prep-0.51.0.md`](docs/superpowers/notes/2026-11-21-release-prep-0.51.0.md)。

## 0.31.0 — 2026-11-20

EV-PRES 薄规格：给现有评测加一层保全感知的成对报告，先预注册，再报 Pass/Avg、新过/新挂、成本与机制归因。纯脚本与文档，不改 Runtime。

### 新增

- **`scripts/ev-pres/summarize.py`**：校验 JSONL；输出每 arm Pass/Wilson 95% CI/Avg score/tokens/时长；成对 Win/Tie/Loss、新过/新挂/未配对；成本只算成对样本；按 mechanism 分组。
- **退出码**：`--fail-on-new-fail` 在有“基线过、候选挂”时退出 1，可供 PR/CI 使用。
- **模板**：`prereg.md`（预注册）与 `report.md`（报告）；示例 `sample.jsonl` 与用法说明。
- **测试**：`test_summarize.py` 覆盖校验、统计、得失、成本、机制与退出码。

### 口径

- `pass` 是主指标；`score` 只作辅助，不替代二元结果。
- 小样本必须报 Wilson 区间；不得用点估计宣称显著改善。
- 未配对 case 必须显式列出，不能悄悄丢弃。

## 0.30.0 — 2026-11-20

MOBILE-RT M3 工程收口：iOS 空壳具备商店上架所需的图标、隐私、工程设置、归档脚本与资料。签名/提审仍需 Apple 账号。

### 变更

- **iOS 商店工程**：AppIcon 1024、AccentColor、LaunchBackground、`PrivacyInfo.xcprivacy`、iPhone-only、版本 1.0.0、加密豁免、本地网络说明。
- **归档与校验**：`ExportOptions.plist`、`scripts/archive-ios.sh`、`scripts/check-release.py`。
- **商店资料**：`AppStore/README.md` 覆盖文案、关键词、隐私回答、审核备注、截图尺寸、签名/TestFlight 清单与已知阻塞。

### 待账号

- Apple Developer Team ID、Bundle ID 注册、审核演示端点/测试 key、真实截图、公开隐私政策 URL、TestFlight / App Review。

## 0.29.0 — 2026-11-20

记忆设置页按 key 分组、标冲突/近似并支持批量清理。向后兼容；选 **minor**（v0.29.0）。

### 新功能

- **按 key 分组**：设置 → 记忆按 key 分组展示；无 key 的条目进「未分组」，组内最新在前，冲突/近似组优先。
- **冲突与近似**：同 key 多条显示「同 key 多条」；文本字符 bigram Jaccard ≥ 0.72 的条目标「近似」（与后端 NEARDUP 同阈值）。
- **批量清理**：每条可勾选，组头可全选；「清理选中」经确认后逐条删除，失败保留并提示。
- **不改后端**：复用现有 list / patch / delete，无 schema 或 API 改动。

## 0.28.0 — 2026-11-20

异步子代理支持空闲自动继续（默认关）。向后兼容；选 **minor**（v0.28.0）。

### 新功能

- **空闲自动继续**：设置 → 匹配与决策 → 连接器分组新增开关，默认关。开启后，异步子代理成功完成且父 Run 已结束、会话无活动 Run 时，自动用回执开一轮新 Run。
- **不插队**：父 Run 运行中、等审批或会话有其它活动 Run 时跳过，保留「继续处理」手动入口。
- **单层防循环**：自动唤醒产生的 Run 带 `run.auto_wake` 标记，其子代理完成不再自动唤醒。
- **可见**：新 Run 会追加「子代理完成，自动继续」系统提示；不伪造用户气泡。

## 0.27.2 — 2026-11-20

真实浏览器 UI 走查修复：空会话加载、活动条状态、实时用时与子 Run 上下文隔离。向后兼容；选 **patch**（v0.27.2）。

### 修复

- **新对话首次加载**：Gate 开启时，空会话还没有 `conversation_meta`，operator 读取消息/目标不再 403；有内容但缺 meta 的旧会话仍拒绝。
- **活动条状态**：审批完成后不再聚焦 `approved` 工具卡并显示加载图标，结果到达后聚焦最新完成步骤。
- **实时用时**：忽略 Go 零值时间戳（`0001-01-01`），不再出现「1700 万小时」。
- **子 Run 上下文隔离**：`Depth > 0` 的子 Run 不再继承父会话历史、父目标与父记忆，只吃 brief；恢复路径同样生效。

## 0.27.1 — 2026-11-20

异步历史子代理卡自动补齐回执。向后兼容；选 **patch**（v0.27.1）。

### 修复

- **历史卡补回执**：父 Run 已结束、异步子 Run 已完成时，子代理卡在挂载或展开刷新时回读父 `run.receipt`，补齐状态、摘要与 facts，不再等页面重载。
- **父事件优先**：已有 `run.receipt` 折叠值不被回读覆盖；回读失败保留现有卡片，fail-open。
- **每 child 一次**：终态回读只做一次，不增加持续轮询。

## 0.27.0 — 2026-11-20

异步子代理完成后可受控继续：回执预填输入框，发不发由人决定。向后兼容；选 **minor**（v0.27.0）。

### 新功能

- **继续处理**：异步子代理终态卡片显示「继续处理」，点击把目标、状态、摘要与 facts 预填到 Composer；不自动发送、不自动开 Run。
- **忙时排队**：当前有活动 Run 或在等审批时按钮禁用并提示「当前回复结束后可继续」；空闲后同一入口恢复。
- **只对异步**：同步子代理的父已处理回执，不显示该按钮；历史卡与 live 卡一致。

## 0.26.0 — 2026-11-20

子代理支持异步第一步：父对话不被长任务堵住，子完成有回执与会话提示。向后兼容；选 **minor**（v0.26.0）。

### 新功能

- **异步子代理**：`spawn_group_run` 增加可选 `async`；开启后父 Run 立即拿到 `started` 并继续当前对话，不再进入 `waiting_child`。默认仍同步。
- **完成提示**：异步子终态写 `run.receipt` 到父事件，并向会话追加「子代理完成/失败/已取消/已拒绝」系统提示，包含摘要与 facts。
- **唤醒守卫**：只有父仍在等这个子（`waiting_child` 且 child id 匹配）才唤醒父；异步子不会误触发父恢复。
- **界面**：子代理卡显示「异步」标记；停止、自动刷新、状态兜底沿用 v1.1a。

## 0.25.0 — 2026-11-20

稳定 prompt 前缀，减少跨轮缓存失效；卸出声明截断数量。向后兼容；选 **minor**（v0.25.0）。

### 变更

- **前缀缓存**：system 不再写当前时间；当前时间、记忆块与活动目标移到历史之后、当前用户输入之前。新 Run 的 system + 历史前缀可复用，不再因时间秒数或记忆命中变化而整段重算。
- **卸出声明**：`FormatOffloadMessage` 增加 `truncated: true/false (preview N of M chars)`，模型明确知道预览被截断以及如何继续读取。
- **文档**：补长会话中途切换模型/努力度可能损失缓存的提示。

## 0.24.1 — 2026-11-20

子代理卡补停止、自动刷新与状态兜底。向后兼容；选 **patch**（v0.24.1）。

### 新功能

- **停止子代理**：子代理运行或等待人工时，卡片内可「停止子代理」，二次确认后取消子 Run；父 Run 收到 cancelled 回执并恢复。
- **轨迹自动刷新**：卡片展开且子代理未终态时每 3 秒刷新子事件与状态；收起或终态即停，手动刷新仍保留。
- **状态兜底**：父 Run 漏收 `run.receipt` / `run.child_resumed` 时，用远端子 Run 状态纠正卡片；终态优先，避免回退。

## 0.24.0 — 2026-11-20

组作用域子 Run 在聊天里可见、可下钻、可审批；回执补结构化 facts。向后兼容；选 **minor**（v0.24.0）。

### 新功能

- **子代理卡片**：`spawn_group_run` 在父聊天显示「子代理」卡，展示目标、连接器组、只读/读写、状态、回执摘要与 facts；不再重复显示 spawn 工具卡。
- **子轨迹下钻**：卡片内展开，按 `child_run_id` 拉取事件与 `run.binding`，显示最近 40 条工具/助手/等待事件；不新增页面。
- **子审批入口**：子 Run 进入 `waiting_human` 时父卡片显示「等待审批 / 等待登录」，展开后可直接同意、拒绝或浏览器登录，审批仍落在子 Run 上。
- **结构化回执**：子 Run 按约定在结尾输出 `RECEIPT_JSON`，引擎解析为 `summary` / `facts` 并写入 `run.receipt`；格式不对时回退原文，不影响子 Run 终态。
- **父状态事件**：新增 `run.child_waiting` / `run.child_resumed`，让父卡片在子等待与恢复时准确变化。

### 变更

- **工作流内禁用 spawn**：工作流步骤内调用 `spawn_group_run` 会明确报错，不创建子 Run；避免 `waiting_child` 与工作流状态机交叉。

## 0.23.0 — 2026-11-20

工作流步骤内的登录与工具审批都能带状态暂停；审批步骤事件补全；中断步骤诚实失败。向后兼容；选 **minor**（v0.23.0）。

### 新功能

- **工作流登录续跑**：工作流步骤遇到 `login_required` 时持久化 `Kind=login` + 工作流状态，Run 进入 `waiting_human` 并释放 worker；完成登录后重试同一步，重启后仍可继续。工作流登录只提供「继续 / 放弃」，不跳过步骤。
- **工作流内工具审批**：非 `approve: true` 步骤的工具自身 `require_approval` 现在也带工作流状态暂停，同意后回到同一步并继续，不再落回 ReAct。
- **结构化等待**：`hitl.waiting` 增加 `workflow_skill` / `workflow_step` / `workflow_step_index`；工作流卡片直接按步骤 ID 标记「等待审批」或「等待登录」，不再解析 prompt 文本。

### 修复

- **审批步骤事件补全**：同意后补发 `workflow.step_started` / `workflow.step_completed`，步骤不再一直显示「运行中」。
- **中断步骤诚实失败**：进程在工作流步骤执行中途退出时，重新入队后判失败并提示「可能已执行；请核对后再重新发起」，不重跑该步骤或整条工作流。

## 0.22.0 — 2026-11-20

工作流审批改为持久化暂停与续跑；工作流暂停步骤在界面可见。向后兼容；选 **minor**（v0.22.0）。

### 新功能

- **工作流审批续跑**：`workflow.yaml` 的 `approve: true` 步骤改为持久化暂停：状态（skill、下一步、结果树）写入 HITL 载荷，Run 进入 `waiting_human` 并释放 worker；同意后执行当前步骤并从下一步继续，重启后从 `hitl_json` 恢复。旧等待没有状态，启动时仍失败并提示重跑。
- **HITL-2 UI**：工作流卡片标出「等待审批」步骤；审批卡提示「工作流已暂停；重启后仍可继续。」。

### 变更

- **移除阻塞 Gate 旧路径**：工作流审批不再占用 worker；ReAct 工具审批行为不变。

## 0.21.0 — 2026-11-19

定时静默与全局功能搜索；旧 Run 的运行来源显示修复。向后兼容；选 **minor**（v0.21.0）。

### 新功能

- **定时来源与静默**：日程开出的 Run 带 `origin=scheduled`；系统提示模型“没有新变化只回 `NO_UPDATE`”。命中后 Run 仍 `succeeded`、`output` 为空，记 `run.silent`，WebUI 留一行「定时检查：无新增。」，不写助手消息、不发渠道、不抽取记忆。手动 Run 的 `NO_UPDATE` 按普通回复处理。
- **功能搜索**：侧栏「搜索功能」/ `Ctrl/Cmd+K` / `/` 打开命令面板；设置页从 `settingsNav` 自动索引并直达，聊天动作（查看运行绑定、重新回答、审批/登录等）显示位置提示，不执行动作。

### 修复

- **运行绑定来源**：旧 Run 的 `origin` 为空时按 `manual` 显示，不再整行隐藏。

## 0.20.0 — 2026-11-19

零工具调用却声称写完成时追加系统提示与审计事件。向后兼容；选 **minor**（v0.20.0）。

### 新功能

- **写操作诚实性**：Run 成功但整个 Run 零工具调用、最终回复却声称“已创建 / 已复制 / 已更新 / 已删除”时，追加 `run.unverified_write` 事件与系统提示「本轮没有调用任何工具，但回复声称写操作已完成。请核实结果。」；不改答案、不改运行状态。有任意工具调用时不检查（v1 边界）。

## 0.19.0 — 2026-11-19

待审批默认 24 小时过期；审批卡与日程显示已等待时长；重启时立即清理无法冷恢复的工作流审批。向后兼容；选 **minor**（v0.19.0）。

### 新功能

- **HITL 生命周期**：待审批 / 登录等待默认 24 小时过期（`hitl.approval_ttl_sec`，显式 `0` 关闭）；过期后 Run 记为 `cancelled` 并释放会话与日程。审批卡与日程列表显示已等待时长；启动时无法冷恢复的工作流审批直接失败并提示重新发起。

## 0.18.0 — 2026-11-19

连接器补齐表单/query/静态认证；记忆合并与近义去重；日程显示会话忙。向后兼容；选 **minor**（v0.18.0）。

### 新功能

- **记忆合并**：`remember_fact` 支持 `supersedes` 按 key 替代旧条；注入带 key 便于纠正；自动抽取同轮显式优先、正文去重、单轮上限 3 条；设置页可按显式/自动过滤与搜索。
- **记忆近义去重**：自动抽取写入前用字符 bigram Jaccard ≥ 0.72（数字/长度守卫）挡“换虚词/调语序”的近义复述；语义改写明确不承诺。

### 修复

- **OpenAPI 连接器**：支持 `application/x-www-form-urlencoded` 请求体（Django/DRF 等读 `request.POST` 的后端）；补充 query 参数的 schema/required 与 URL 拼接；会话内调用也会回退到连接器的静态认证头（捕获的登录身份仍优先）。
- **日程阻塞原因**：新增 `conversation_busy`，会话因 RUN-GUARD 有活跃 Run 而跳拍时，日程列表显示「暂停触发 · 对话忙」，不再误显示「进行中」。

## 0.17.0 — 2026-11-19

会话级单活跃 Run；运行中目标实时刷新；Inbox 在网关下的会话归属修复。向后兼容；选 **minor**（v0.17.0）。

### 新功能

- **会话级单活跃 Run**：同一会话在同一时刻只允许一轮 Run。忙时 `POST /v0/runs` 返回 409 `conversation_busy`；inbox 返回 409 + `Retry-After` 并释放幂等占位（同一 key 可在会话空闲后重试）；渠道保持忙回；日程到点遇忙静默跳拍，下一次扫描重试。守卫为进程内，多副本行为见开发者文档「已知限制」。
- **目标实时刷新**：运行中 Agent 修改目标（`goal.set/push/pop/update`）时，聊天页目标栏即时更新，不再等本轮结束。

### 修复

- **日程阻塞文案**：日程因等待审批等原因暂停触发时，状态显示「已启用」而不是「进行中」，避免与徽标并列时读起来矛盾。
- **Inbox 网关下的会话归属**：开启控制面口令后，同一 `external_id` 的第二条 Inbox 消息曾被会话归属校验挡成 500；现在 Inbox 以机器身份访问自己创建的会话，第二条起正常返回 202/409。

## 0.16.0 — 2026-11-19

日程列表标出阻塞原因，运营账号可正常使用目标与停止运行。向后兼容；选 **minor**（v0.16.0）。

### 新功能

- **日程阻塞可见**：设置页日程列表标出「暂停触发 · 等待审批 / 等待登录 / 等待子任务 / 等待处理 / 运行记录不可用」，并提供「去处理」回到对应会话。只读派生，不改触发与审批语义。

### 修复

- **目标接口权限**：运营账号现在可以读写自己会话的目标（此前未列入 ACL，落进默认管理员规则返回 403，聊天页显示「目标加载失败」）。归属校验与 `active_goal_enabled` 开关不变。
- **停止运行权限**：运营账号现在可以停止自己会话里的 Run（`POST /v0/runs/{id}/cancel` 此前未列入 ACL，返回 403，聊天页提示权限失效）。
- **403 文案**：权限不足不再提示「登录已失效，请重新解锁」，改为提示联系管理员；只有 401 才提示重新解锁。

### 文档

- 开发者文档补「已知限制」：工作流步骤审批占 worker 槽且重启不可恢复、多副本日程抢领窗口、记忆不覆盖同名条目、定时 Run 无来源标记。

## 0.15.2 — 2026-10-29

连接器可配置鉴权失败识别；OpenAPI 响应大纲进工具描述；卸出可关；流式输出不再被打字机与同步落库拖慢。向后兼容；选 **patch**（v0.15.2）。

### 新功能

- **鉴权失败识别（login_signals）**：连接器高级区可配置 HTTP 状态 / 业务码 / 文案关键词；命中后映射为 `login_required`，避免 400/404 等歧义状态被单独当作掉登录。
- **OpenAPI 响应大纲**：对象型成功响应摘要写入工具 Description，便于模型直接选用字段。
- **卸出开关**：运行参数可关闭「大结果卸出到工作区」；开启时阈值更高、预览更长，默认不鼓励多余 `read_file`。

### 修复

- **流式打字机**：界面直接跟随服务端快照，去掉本地限速放出。
- **流式落库**：`llm.*.delta` 异步 upsert，并减轻热路径上的全量事件重载，避免拖慢上游读流。

## 0.15.1 — 2026-10-24

对话用时可见；侧栏新建工作区可点；官网 Persuade 面。向后兼容；选 **patch**（v0.15.1）。

### 新功能

- **回复用时**：助手气泡显示本轮墙钟时长（思考 + 工具 + 回复；进行中实时刷新，历史按事件首尾时间戳）。

### 修复

- **新建工作区**：侧栏入口改为带边框次要按钮（含「+」），避免被当成说明文字。

### 文档

- 官网按「玉册」方向重做：冷调纸面、首屏去徽章、手机双路径独立成节。

## 0.15.0 — 2026-10-23

手机双路径、连接器分组与长任务韧性；评测语料与对话日程；上下文卸出。向后兼容；选 **minor**（v0.15.0）。

### 新功能

- **手机双路径**：Capacitor 壳遥控远端 `/ui`；Go 编译的 `mobileagent`（AAR / XCFramework）机上精简编排，App 自绘 UI（`Configure` / `Send` / 审批 JSON）。
- **连接器分组与组作用域子任务**：热配置分组；需要审批互不碰撞时可开组作用域子 Run。
- **长任务韧性**：轮内压缩与会话检索、步数收尾、LLM 重试回退、超时拆分、只读工具并行、HITL 释槽；sticky Top-K 与 MCP sandbox / 飞书钉钉适配器样例。
- **上下文卸出**：超大体量工具结果与用户粘贴写入会话工作区 `offload/`，模型侧仅预览+路径，可用 `read_file` 分段回读。
- **Admin 评测语料（设置页）**：按时间窗筛选失败/慢 Run、快照入库 EvalCase、确定性 evidence 重打分与 JSON 导出；人审标定（同意/不同意）与一致率摘要；轨迹约束（`forbid_tools` / `arg_contains` / `expect_state`）与终态只读活查。
- **对话定时任务**：人同意后保存一次性、每天或每周日程；到点在原会话新开一轮。写操作仍走审批。设置页可管理当前账号自己的日程。

### 修复

- **工作流 HITL**：持有租约时 `resume` 仍可唤醒等待中的人工审批（避免 Gate 与租约竞态丢唤醒）。

### 文档

- README / 官网按去空话改写；公开文档去掉规划代号；移动端写成双路径上手指南；主 README 将手机能力提到开篇与专节。

## 0.14.0 — 2026-10-15

已交付能力在设置页可发现，观测项可热更新；登录 HITL 弹框终态清理。向后兼容；选 **minor**（v0.14.0）。

### 新功能

- **观测热更新**：`GET/PATCH /v0/settings/runtime` 可改有效 `log_format` / `otel_otlp_endpoint` / `otel_service_name`（设置 → 运行参数 → 观测）；写入运行时 KV 覆盖并立即换 slog Handler / OTLP exporter，**不写回 YAML**。空 endpoint 强制关 OTel；`observability_reset: true` 清除覆盖并恢复 YAML（及启动时 `BAIZE_LOG_FORMAT` 基线）。
- **设置页可发现性**：连接器编辑器「出站 TLS / mTLS」（本机路径）；Webhook 页说明会收 `catalog.diff`；MCP 导出「调用审计」；聊天助手消息旁「查看运行绑定」（`run.binding`）；运行指标页链到观测说明。

### 修复

- **登录 HITL 弹框**：对话结束后不再因历史 `login_required` 事件残留暂停横幅；仅当前 live Run 可展示。

### 文档

- architecture / configuration / http-api / baize-help 对齐设置入口、热更新语义与边界。

## 0.13.0 — 2026-10-15

管理后台直接查看运行指标与近期进程日志。向后兼容；选 **minor**（v0.13.0）。

### 新功能

- **设置 → 运行指标**：管理员可在 `/ui` 后台同源拉取 Prometheus 文本（`GET /metrics`）与进程内近期日志（`GET /v0/settings/logs`），无需另开终端或独立刮取端。
- **近期日志 API**：`GET /v0/settings/logs?limit=`（Admin；默认 200、上限 1000）返回内存环形缓冲中的 slog 行（约最近 500 行）；与 stderr 双写；**不持久化**，重启清空。指标仍为进程内计数，重启清零；长期留存请用外部 Prometheus 刮取。

### 其他

- 去掉启动时打印的示例 `curl … input:你好` 提示（避免误解为真实请求）。

### 文档

- architecture / configuration / http-api / baize-help 说明后台入口、日志缓冲边界与非持久化语义。

## 0.12.0 — 2026-10-15

MCP HTTP 出站 mTLS。向后兼容；选 **minor**（v0.12.0）。

### 新功能

- **MCP HTTP 出站 mTLS**：`type: mcp` 且 `transport: http` 时，discover 与每次 `tools/call` 复用连接器 `tls.cert_file` / `key_file` / `ca_file`（与 OpenAPI / HTTP 插件同一字段）。未配置 `tls` 时行为与 0.11 一致。**stdio** MCP 不适用。

### 文档

- architecture / configuration / http-api 说明出站 mTLS 覆盖 OpenAPI + HTTP 插件 + MCP HTTP；backlog 注记 MCP 出站 mTLS 已落地。

## 0.11.0 — 2026-10-15

连接器出站 mTLS（文件路径）。向后兼容；选 **minor**（v0.11.0）。

### 新功能

- **连接器出站 mTLS**：OpenAPI 与 HTTP 插件工具 invoke 可选用本机文件路径客户端证书 / CA（`tls.cert_file` / `key_file` / `ca_file`）。`cert_file` 与 `key_file` 必须成对；仅 `ca_file` 合法（定制 RootCAs）。Store **只存路径**，不持久化 PEM。与 `auth.mode` **正交**（TLS 握手用证书，HTTP 头仍按 auth）。未配置 `tls` 时行为与 0.10 一致。**MCP** 出站 mTLS 未做。

### 文档

- architecture / configuration / http-api 说明连接器 `tls` 字段与边界；backlog 注记出站 mTLS（OpenAPI/HTTP）已落地；MCP 出站 mTLS 仍后续。

## 0.10.0 — 2026-10-15

CatalogDiff 出站告警 + OTel 契约属性。向后兼容；选 **minor**（v0.10.0）。

### 新功能

- **CatalogDiff → `events.webhook`**：连接器 `PUT` 成功且 `catalog_diff` 非空时，向全局 `events.webhook` 异步 POST `type=catalog.diff`（含 `connector_id`、`catalog_revision`、`diff`）；空 Diff 或未配置 URL 则跳过；入队/投递失败 **fail-open**，不改 PUT 成功语义。Outbox kind `catalog_diff`，作用域键 `connector:<id>`，按 revision 去重。
- **OTel Chat 契约属性**：`runtime.otel_otlp_endpoint` 开启时，Chat 路径 `tool.result` span 写入 `baize.contract_hash`、`baize.catalog_revision`（事件 Data 有时；MCP 导出路径此前已具备）。

### 文档

- architecture / configuration / http-api 说明 `catalog.diff` 出站与 OTel 属性；backlog 注记 CatalogDiff 告警已落地；configuration / architecture 声明独立 webhook URL 与连接器 mTLS 仍后续。

## 0.9.0 — 2026-10-15

baumgaerben observability：标准 Prometheus 刮取面 + 结构化 slog。向后兼容；选 **minor**（v0.9.0）。

### 新功能

- **`GET /metrics`**：Prometheus 文本格式，进程默认开启；**Admin Bearer** 鉴权（无 Bearer `401`，Operator `403`）。
- **metricsx**：低基数 Run / 工具 / MCP 导出指标；`connector_id` 空时为 `unknown`；**无** tool 名标签；指标更新 fail-open，不阻断 Run。
- **applog**：`runtime.log_format` 与 **`BAIZE_LOG_FORMAT`**（`text` | `json`）；bootstrap 与 reconcile 等热路径结构化日志。

### 文档

- architecture / configuration / http-api 公开 observability、刮取鉴权与非目标（pin / 沙箱 / OBO / 告警 / mTLS）；backlog 注记 metrics+slog 已落地。

## 0.8.0 — 2026-10-15

执行期依赖闭包（R1 reconstructability）：Run 可追溯当时绑定，工具契约可按指纹对比。向后兼容；选 **minor**（v0.8.0）。

### 新功能

- **`run.binding`**：首次主模型调用前固化模型/knobs/记忆注入/收窄后工具契约指纹；写入失败阻断 Run。
- **调用级契约指纹**：`llm.tool_call` / `tool.result` 带 `contract_hash` 与 `catalog_revision`；Chat 工具卡可展示。
- **连接器修订历史**：实质重导时 `catalog_revision++`，`GET /v0/connectors/{id}/catalog-revisions`；设置页编辑器可查看。
- **MCP 导出审计**：`GET /v0/settings/mcp-export/invocations` 记录导出调用的契约指纹。
- **`GET /v0/runs/{id}/binding`**：读取闭包。

### 文档

- architecture / http-api / baize-help 说明 reconstructability；backlog R1 标为已落地。

## 0.7.0 — 2026-10-14

连接器重导可见性、可选 OTel、身份边界文档。向后兼容；选 **minor**（v0.7.0）。

### 新功能

- **CatalogDiff**：连接器重导成功后 `PUT /v0/connectors/{id}` 返回 `catalog_diff`（新增 / 删除 / method·path·schema 变更）；设置页保存提示展示摘要。
- **可选 OTel OTLP**：`runtime.otel_otlp_endpoint` 非空时将 Run / `tool.result`（及 MCP 导出调用）导出为 span；默认关闭、失败 fail-open。
- **事件审计字段**：`tool.result` 等可带 `connector_id` 等非密钥字段；永不写 token/密码。

### 文档

- 澄清 `passthrough` / 导出身份 / `identity_slots` = 下游业务账号权限，**不是**终端用户 IdP OBO；明确不做 per-tenant 沙箱与 Prometheus `/metrics`（本版本）。
- configuration / architecture / http-api / baize-help 与配置样板注释对齐。

## 0.6.0 — 2026-10-14

托管登录页、对外登录向导与 MCP 导出下游鉴权。向后兼容；选 **minor**（v0.6.0）。

### 新功能

- **托管登录页**：公网 `/v0/auth/hosted/...`（表单提交、多方式切换、OAuth 回调）；凭证不经大模型，写入导出身份槽或 Chat 工作区账号池。
- **对外登录**：设置「对外登录」向导——规则候选 + 可选 AI 建议、多种 `hosted_login.methods`、公开基址与导出身份绑定；连接器高级区改为跳转本页。
- **MCP 导出身份槽**：一把 Key 可按连接器 / 角色绑定多个身份；鉴权失败优先 URL elicitation，无 elicitation 的客户端（如豆包）工具错误中**立即**返回 `hosted_login_url`。
- **Chat 登录 HITL**：`login_required` 时可打开浏览器登录、忽略该系统继续，或结束本轮。

### 文档

- README / 架构 / 配置 / HTTP API / baize-help / 官网与现实现状对齐（对外登录、对外提供能力、账号与导出身份分界、`public_base_url` 可达性）。
- 公开文档去掉内部 `DP-*` 编号用语；README 补充决策模型与评测分层说明；用语统一为决策模型 / 主模型 / 备用模型。

### 修复

- CI：`gofmt` Session 字段对齐；staticcheck QF1001（MCP 鉴权重放守卫）。

## 0.5.0 — 2026-10-09

决策模型 System One 一键启用，国内安装源 ModelScope。向后兼容；选 **minor**（v0.5.0）。

### 新功能

- **决策模型（System One）**：设置「匹配与决策」一键启用本机 Ollama `tev1`（需 ≥0.35）或兼容 API；写入 `decide_systemone_*`；`/v0/settings/systemone*`。
- 判定链路：System One → 可选 `decide_profile_id` 备用模型 → Rules；留空不会改用主模型。

### 体验 / 性能

- 国内 Windows Ollama 安装优先 [ModelScope `Lixiang/ollama-release`](https://www.modelscope.cn/models/Lixiang/ollama-release/files)，跳过体积不符的陈旧镜像。
- 智能提速文案澄清：总开关 + 各功能开关才省主模型 token；「备用模型」仅在决策模型不可用时使用。

### 文档

- README / 架构 / 配置 / HTTP API 与现实现状对齐。

### 修复

- CI：gofmt、硬编码色、Linux 下 unused 安装探测函数。

## 0.4.0 — 2026-10-08

工具匹配增强：可选本机 Ollama / 云端 Embedding，设置页一键安装与清理。向后兼容；选 **minor**（v0.4.0）。

### 新功能

- **工具匹配**：设置中心独立「工具匹配」页；默认标准匹配；增强匹配支持本机 Ollama（国内镜像安装、模型拉取、安装/模型路径展示与自定义模型目录、可选卸载本体）或 OpenAI 兼容 Embedding API。
- 工具收窄支持 BM25 + 向量混合检索；增强匹配失败时回退标准匹配，不影响对话。
- 证据评测脚本 `scripts/evidence-eval/`，用于回放/打分工具调用轨迹。

### 体验

- 运行参数页改为分栏：常用 / 智能提速 / 记忆与压缩 / 安全。
- 本机已装 Ollama 且匹配模型就绪时，主按钮文案为「启用增强匹配」，不再误显示「下载」。

## 0.3.3 — 2026-10-06

- 滚动摘要改为结构化检查点（目标 / 进度 / 决定 / 必须保留的事实），不把执行策略写进压缩模板。

## 0.3.2 — 2026-10-04

- 修复 golangci-lint：删除未使用的 `ensureConversationMeta`，投影测试按 De Morgan 写法通过 QF1001。
- 公开仓导出排除 `docs/decisions/`（与 `docs/superpowers/` 一样只留在私有仓）。

## 0.3.1 — 2026-10-04

- `gofmt` 对齐 `internal/identity/scope.go` 与投影测试，修复 GitHub CI 格式检查。
- 评测脚本与注释去掉内部系统名，避免开源导出被私人词表拦住。

## 0.3.0 — 2026-10-04

Web 工作区、可选长对话投影、产品自助说明，以及对话流式体验修复。向后兼容；选 **minor**（v0.3.0）而不是补丁号。

### 新功能

- **工作区**：侧栏切换 / 新建；同工作区共享 Web 登录身份，跨工作区隔离；新对话落在当前工作区（v1 不搬迁已有对话）。
- **长对话保留重点**：运行参数开关（默认关）；只整理给模型看的投影，不改写已保存聊天；失败回退原有压缩。不承诺省云端 API token。
- **baize-help**：默认内置技能，说明白泽怎么用、设置在哪；不能改 `config.yaml` 或重启进程。
- 技能 `tools:` 作为最低保留集合，不再在工具筛选时把未列出的连接器工具整表关掉。

### 体验

- 思考默认收成一行；流式结束时用量立刻写上，无需刷新。
- 流式完成时不再先闪用户气泡再补答案。

### Breaking

- Web 客户端（`web/chat/src/api.ts`）移除未使用导出 `patchToolRequireLogin`、`clearMessages`。请改用 `patchTool`；清空消息若需可直接调用 `DELETE /v0/conversations/{id}/messages`（当前 UI 未提供封装）。
