# Changelog

## Unreleased

### 文档

- 公开文档去掉内部 `DP-*` 编号用语；README 补充「哪些过程改由决策模型承担」与评测分层说明。
- 补充 layer B：同语料下 System One（tev1）vs 备用模型判定的端到端耗时 / 成功率对比（见 README 与 `scripts/tool-routing-eval`）。
- 界面与公开文档用语：决策模型 / 主模型 / 备用模型；去掉「聊天小助手」「兜底」「总闸」「一点启用」等口语化说法。

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
