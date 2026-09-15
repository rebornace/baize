[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)

# Baize（白泽）

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

[English](README.md) | **中文**

**企业侧的 AI Agent Runtime：能对话、会调工具、写操作可审批，挂在现有系统旁边就能用。**

白泽不是又一个「只能聊天的窗口」，而是一台可审计的 Agent：模型思考 → 调用工具 → 必要时等人点头 → 把结果写回会话与渠道。业务 HTTP 不用改代码；有 OpenAPI 就能变成 Agent 的手和脚。`/ui` 是给运营与集成方用的**操作台**，不是面向 C 端的娱乐聊天 App。

---

## 为什么是白泽，而不是「再包一层 ChatGPT」

| 常见做法 | 白泽 |
|----------|------|
| 把文档丢进提示词，让模型「猜」怎么调 API | **OpenAPI / Swagger / Postman → 真正可执行的 Tool**，进目录、进 Run 轨迹 |
| 模型直接改生产数据 | **写操作 HITL**：工具卡片上批准 / 驳回，Run 可暂停再继续 |
| 新系统里嵌 SDK、改业务代码 | **零侵入旁挂**：单进程 Runtime，停掉几乎不留痕 |
| 只做网页对话 | **同一套 Agent**：控制台 + 签名 Inbox + 个人微信私信等渠道共用 Run / 审批 / 出站 |
| 工具散落各处、鉴权靠运气 | **连接器 + 会话身份 + 登录 Skill**：`require_login` 可引导去登录，再带着凭证调下游 |
| 上下文一长就炸 | **滚动摘要压缩**、多模型 Profile、思考级别、会话级 Memory 与工作区文件 |

一句话：**白泽卖的是「会干活、可控、可卸」的 Agent，不是又一个聊天框皮肤。**

---

## Agent 会做什么（产品亮点）

### 思考与执行

- **ReAct Agent**：选工具 → 执行 → 写事件轨迹 → 结束；可选 Skill 内线性 workflow（顺序步骤 + 审批点）。
- **流式 Run**：操作台跟得上推理与工具过程（SSE），不是黑盒「等半天出一句」。
- **多模型 Profile**：多套 LLM 配置可切换；可调**思考级别**（适配支持 thinking 的模型方言）。
- **Skill 包**：`SKILL.md` + 工具绑定；默认技能与本轮 `activate_skill` 扩大能力面。

### 工具从哪来

- **OpenAPI 一键变工具**：上传 / 指定规格，每个 operation 进工具目录，Agent 可直接调。
- **HTTP 插件与执行回调**：没有完美文档时，用侧车或企业回调接遗留逻辑。
- **MCP 双向**：既可当 **MCP 客户端**接入外部工具服务（含 OAuth 登录流），也可 **MCP 导出**把目录只读暴露给其他宿主。
- **登录与身份**：下游要登录时，用托管登录 Skill / 会话 Identity，而不是把密钥糊进提示词。

### 人在回路（HITL）与安全边界

- 敏感写操作进入 `waiting_human`，运营在卡片上决策后再 `resume`。
- 工具可启停；完整凭证不出现在事件回放里。
- 试用走 mock LLM（无需 Key）；生产再配真实模型——路径分开，避免「演示误伤生产」。

### 记性、材料与长会话

- **Memory**：账号级可检索事实，Agent 可引用，不只依赖当前窗口。
- **会话工作区文件 / Blob**：附件与对象可走本地或 S3 等驱动。
- **上下文压缩**：长对话自动滚动摘要，把 token 花在真正有用的历史上。
- 会话 **fork / rollback** 等运营友好操作（见控制台与 API）。

### 渠道：Agent 走到人在的地方

- **签名 Inbox**：告警 / 工单类入站，进入同一套 Agent 与审批。
- **个人微信私信**：进程外适配器 + 出站队列（失败可在设置里重投），不是「演示用 webhook 玩具」。
- 事件也可走 Webhook 出站，方便接企业通知总线。

### 给集成方与运营

- 完整 **HTTP 控制面**（Run / 对话 / 连接器 / 设置），无强制语言 SDK。
- `/ui` **中英双语**操作台：对话、工具卡片、设置 IA、运行参数人话化。
- Runtime **热更新**部分旋钮；存储默认 SQLite，可上 Postgres。

---

## 典型场景

**遗留 HTTP 系统旁挂 Agent**  
服务已在线、暂不动代码：旁挂白泽，用接口文档把 API 变成工具，内网试用「能查、能建单、能审批」，确认价值再谈改造。

**运营盯着写操作的助手**  
Agent 可以查数、起草、建议；真正改状态、发指令必须人在 `/ui` 点头，审计轨迹留在 Run 事件里。

**工具与渠道统一到一台 Agent**  
MCP 工具、自研 HTTP、微信私信、Inbox 告警——同一 Runtime、同一审批与记忆，而不是每个入口各写一套 Bot。

---

## 30 秒感受 Agent

**环境：** Go 1.25+（与 CI 一致；无需 C 编译器）

```powershell
# Windows
.\demo.cmd
```

```bash
# POSIX
./scripts/demo.sh
# 或：go run ./cmd/baize demo
```

试用栈 = mock LLM + 内嵌演示 HTTP，**无需 API Key**。

- 操作台：http://127.0.0.1:8080/ui  
- 演示业务：http://127.0.0.1:18080  

打开 `/ui`，发一句「VPN 挂了，请建一条记录」——看 Agent 选工具、出卡片，再**批准写操作**。端口占用时结束旧 `baize` 进程，或设 `BAIZE_LISTEN`（如 `:9080`）后重启。

生产启动（真实 LLM，须 `BAIZE_API_KEY`）：Windows `.\start.cmd`，POSIX `./scripts/start.sh`。

---

## 开发者入口

构建、测试、配置、部署与 HTTP 面：

- [开发者文档](docs/developers/README.md)
- [入门](docs/developers/getting-started.md) · [架构](docs/developers/architecture.md) · [部署](docs/developers/deployment.md) · [配置](docs/developers/configuration.md) · [HTTP API](docs/developers/http-api.md) · [本机探针](docs/developers/performance.md)

---

## 许可证

[MIT](LICENSE)
