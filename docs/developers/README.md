**中文** | [English](./README.en.md)

# 开发者文档

面向贡献者与集成方。产品介绍与试用见仓库根目录 [README（中文）](../../README.zh-CN.md)（[English](../../README.md)）；官网能力叙事见 [rebornace.github.io/baize](https://rebornace.github.io/baize/)。

版本细节以 [CHANGELOG](../../CHANGELOG.md) 为准。下列能力已写入产品面（不仅是日志条目），技术边界见 [架构](./architecture.md)：

| 能力主题 | 用户可感知入口 | 技术落点 |
|----------|----------------|----------|
| 决策层 / 工具收窄 | 设置 → 匹配与决策；运行参数 → 智能提速 | System One、`toolindex`、决策 knobs |
| 托管登录与 MCP 导出 | 对外登录；对外提供能力 | `/v0/auth/hosted/...`、MCP export、`identity_slots` |
| CatalogDiff 与出站告警 | 连接器保存提示；`events.webhook` | `catalog_diff`、`type=catalog.diff` |
| 运行可追溯 | 聊天「查看运行绑定」 | `run.binding`、`contract_hash` / `catalog_revision` |
| 运维可观测 | 设置 → 运行指标；运行参数 → 观测 | `GET /metrics`、OTLP、slog 热更新 |
| 出站 mTLS | 连接器编辑器 → 出站 TLS / mTLS | `tls.cert_file` / `key_file` / `ca_file` |

| 文档 | 内容 |
|------|------|
| [入门](./getting-started.md) · [EN](./getting-started.en.md) | 构建、测试、CI、贡献 |
| [配置](./configuration.md) · [EN](./configuration.en.md) | YAML / 环境变量 / CLI |
| [HTTP API](./http-api.md) · [EN](./http-api.en.md) | 控制面 HTTP 鸟瞰 |
| [架构](./architecture.md) · [EN](./architecture.en.md) | 运行时边界、插件与核心抽象 |
| [部署](./deployment.md) · [EN](./deployment.en.md) | 单机与渠道适配器部署 |
| [移动端](./mobile.md) · [EN](./mobile.en.md) | Capacitor 控制台壳；手机是客户端，Runtime 仍在服务端（非 gomobile） |
| [官网（GitHub Pages）](../../site/README.md) | 静态官网源码与启用说明 |
| [性能探针](./performance.md) · [EN](./performance.en.md) | 本机热点路径复现 |
