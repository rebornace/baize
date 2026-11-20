# Changelog

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
