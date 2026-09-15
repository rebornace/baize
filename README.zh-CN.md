[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)

# Baize（白泽）

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

[English](README.md) | **中文**

**不改业务代码，旁挂一层就能用上 AI；卸掉不留痕。**

白泽是挂在现有 HTTP API 旁边的 Agent Runtime：有接口文档就自动变成可调用能力，写操作可人工审批，用完停掉进程即可。`/ui` 是给操作员用的控制台，不是面向终端用户的聊天产品。

---

## 亮点

- **零侵入**：业务服务不用改、不用嵌 SDK，旁挂即可试用与下线。
- **OpenAPI 变工具**：上传或指定接口文档，接口自动变成助手可调用的能力。
- **写操作审批闸**：变更类调用可先等人批，再真正打到业务系统。
- **可卸载**：单进程、配置在外；停掉 Runtime，业务侧几乎无残留。
- **中英操作台**：`/ui` 支持中文 / English，语言偏好保存在本机浏览器。

---

## 核心能力

- 把已有 HTTP 服务接成「可对话、可调用」的助手能力（有 OpenAPI / Swagger / Postman 文档即可上手；没有文档可用小 HTTP 插件）。
- 人工审批（HITL）：敏感写操作在卡片上批准或驳回。
- 操作员控制台：对话列表、工具卡片、设置与审批，均在 `/ui`。
- 可选对接外部工具服务（MCP）、告警/工单入站（Inbox）、个人微信私信渠道等——细节见开发者文档与设置页。
- 生产与试用路径分开：试用用 mock，不消耗真实模型 Key；上线再配真实 LLM。

---

## 适用场景

**遗留 HTTP 系统旁挂 AI**  
服务已在线、暂不想动代码：旁挂白泽，用接口文档把现有 API 变成助手工具，先在内网试用，确认价值后再谈改造。

**运营审批写操作**  
助手可以查、可以建议，但建单、改状态、发指令等写操作必须经运营在 `/ui` 批准，降低误操作与越权风险。

---

## 30 秒试用

**环境：** Go 1.25+（与 CI 一致；无需 C 编译器）

Windows（仓库根目录）：

```powershell
.\demo.cmd
```

POSIX：

```bash
./scripts/demo.sh
# 或：go run ./cmd/baize demo
```

试用栈使用 mock LLM + 内嵌演示 HTTP，**无需 API Key**。

- 控制台：http://127.0.0.1:8080/ui
- 演示 HTTP：http://127.0.0.1:18080

打开 `/ui` 后可发一句「VPN 挂了，请建一条记录」，并在工具卡片上批准写操作。端口占用时先结束旧的 `baize` 进程，或设置 `BAIZE_LISTEN`（如 `:9080`）后重启。

生产启动（真实 LLM，须 `BAIZE_API_KEY`）：Windows `.\start.cmd`，POSIX `./scripts/start.sh`。配置、部署与渠道细节见开发者文档。

---

## 性能说明

基准数据将在后续 **PERF** 阶段回填。此处**不写**竞品对比数字；有可复现的实测结果后再更新本节。

---

## 下一步（技术读者）

构建、测试、配置、HTTP 面与部署：

- [开发者文档入口](docs/developers/README.md)
- [入门：构建与贡献](docs/developers/getting-started.md)
- [架构](docs/developers/architecture.md) · [部署](docs/developers/deployment.md) · [配置](docs/developers/configuration.md) · [HTTP API](docs/developers/http-api.md)

---

## 许可证

[MIT](LICENSE)
