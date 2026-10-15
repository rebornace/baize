# Changelog

## Unreleased

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
