# README 定位改写 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 中英 README 打开十秒内读到「现有 HTTP 服务旁挂 AI、不改代码、卸下不留痕」；OS/Docker 后移；工单不当产品定义。

**架构：** 只改两个 README。目录锁定为：导语 → 30 秒跑通 → 接到你的服务 → 写操作与身份 → 部署 → 附录。能力事实从现文件搬迁，不发明功能。

**技术栈：** Markdown。无 Go 代码变更。

**规格：** `docs/superpowers/specs/2026-08-14-readme-positioning-design.md`

**全局约束：**
- 不改 `configs/default.yaml`、Dockerfile、compose、Go 代码
- 标题/Why 禁止「工单」；导语禁止强调 Linux/Windows；禁止「任意系统」易混成 OS
- 工具名 `create_ticket`、路径 `examples/mock-ticket`、hostname `mock-ticket` 作为技术标识可保留
- 中英同一目录顺序、平行篇幅
- commit 中文：`type(scope): 说明`；PowerShell 不要 HEREDOC

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `README.zh-CN.md` | 中文主叙述（先写，作为结构源） |
| `README.md` | 英文，章节与中文一一对应 |

---

### 任务 1：改写 README.zh-CN.md

**文件：**
- 修改：`README.zh-CN.md`（整篇重排，徽章与语言切换行保留）

- [ ] **步骤 1：按下列标题顺序重写全文**（不要保留旧的「快速开始」里的 OS 列表和「本机二进制」）

保留文件头徽章 + `[English](README.md) | **中文**`。

**标题（加粗一行）：** `不改你的服务，旁挂一层就能用上 AI。卸掉不留痕。`

**导语：** 规格 §4 中文导语。blockquote 可保留「侧车，不是聊天机器人：OpenAPI → Tools → HTTP」，不要写工单。

**## 为什么需要白泽** — 四行表格用规格 Why。概念表 Agent 改为「提示词 + LLM，驱动一次 Run」。

**## 30 秒跑通**

- 要求：Go 1.22+（无需 C 编译器；SQLite 为纯 Go）
- `git clone` + `go run ./cmd/baize start`（不要在此列出 Linux/macOS/Windows）
- 一句：仓库会同时拉起一个 **mock HTTP 演示服务**，方便你点 UI / 打 curl；不是产品本身
- 端口：Runtime `8080`（`/ui`）、演示 HTTP `18080`、LLM `mock`
- 小节标题不要用「创建工单」。用「调用演示写接口（curl）」
- 说明：`POST /v0/runs` 异步；演示里工具名 `create_ticket` 默认要审批（`waiting_human`）
- curl JSON 的 `input` 改成不出现「工单」二字，例如：`VPN 挂了，请建一条记录`（`agent_id` 仍 `ticket-agent`，因默认配置如此）
- 保留 approve/reject curl；一句「这个演示写接口被标了审批」
- 保留 `18080/tickets` 与 events curl（路径是真实 API，不要改成产品名）
- 启动器、交叉编译、Docker **不要**出现在本节

**## 接到你的服务**

合并三块，顺序：

1. 有 OpenAPI：现「平台接入」的 PUT curl 与替换/坏 spec 说明、Identities 脚注。叙述用「你的 HTTP 服务」，PUT 里的 `ticket-api` / `create_ticket` / mock-ticket 路径保持（技术 id）
2. 无 OpenAPI：HTTP 插件三端点 + 示例 curl；「默认 start 仍是仓库自带的演示 OpenAPI Connector」
3. 真实 LLM：不要密钥进 yaml；`default.local.yaml` + `.env`；yaml 片段保留；`mock_ticket.listen: off` 注释改为「对接你的服务时关闭演示 HTTP」；旧 `demo.local.yaml` 迁移注保留

**## 写操作与身份**

HITL 原则（`require_approval`）一两句，不重复整段 resume curl（已在 30 秒）。然后搬「会话身份」+「对话记忆」全文事实。

**## 部署**

开头：单进程静态二进制；可选 `./scripts/start.sh` 与 `.\start.cmd`（未设 GOPROXY 时写入国内代理）。

然后现「本机二进制」三行 `GOOS`（可写 Linux/macOS/Windows，因为这是部署节）。

然后 Docker：标题不要「服务端（Docker）」，用 `### Docker`。试用 compose；端口；**须两服务一起起**；`docker.yaml` 的 hostname `mock-ticket`；**只起 Runtime 时，打到演示 HTTP 的工具会连不上**（禁止写「connector 注册失败」）。试用 token 默认 `dev`。生产 `docker run` 示例；`your.yaml` 关 mock、`base_url` 指向你的 HTTP 服务。

**附录：** Chat UI 构建、命令表（「mock 工单」改为「演示 HTTP 服务」）、文档链接、许可证。

- [ ] **步骤 2：自检 grep（在仓库根目录）**

禁止出现在 **标题后到「## 30 秒跑通」之前**（导语+Why）：`工单`、`任意系统`、`Linux`、`Windows`、`macOS`、`Docker`。

全文件 Why/标题不得把工单当产品。`工单` 若出现，只允许在命令表改写后的残留检查——目标是导语/Why/30 秒标题没有「工单」。curl `input` 已去掉工单。`/tickets` 路径允许。`ticket-agent` / `create_ticket` / `mock-ticket` 允许。

确认有且仅有这些二级标题（顺序）：`为什么需要白泽`、`30 秒跑通`、`接到你的服务`、`写操作与身份`、`部署`，以及附录的 `Chat UI 构建（可选）`、`命令`、`文档`、`许可证`。

- [ ] **步骤 3：Commit** `docs(readme): 中文 README 改为现有服务端旁挂定位`

---

### 任务 2：改写 README.md（与中文平行）

**文件：**
- 修改：`README.md`

- [ ] **步骤 1：** 按任务 1 的同一节序写英文。锁定用词：

- 标题：`Add AI next to your existing HTTP APIs — no rewrite, nothing left behind.`
- Why 四行对应中文四痛点（existing HTTP service / no OpenAPI / write-path approval / unplug cleanly）
- Agent concept: `Prompt + LLM`，不要靠 `System prompt` 抢「system」= OS
- 节标题：`## Try it in 30 seconds` / `## Point it at your APIs` / `## Approvals and identities` / `## Run and deploy`
- 30 秒：`bundled mock HTTP demo service`，不要 `Mock ticket API` 作为产品句；curl `input` 改为 `VPN is down, please file a record`
- Docker 小节 `### Docker`（不要 `## Server (Docker)`）；失败说明：tools that call the demo HTTP will fail to connect — not connector registration
- 命令表：`demo HTTP service` 替代 `mock ticket`

徽章与 **English** | [中文] 保留。

- [ ] **步骤 2：平行检查**

中英二级标题数量与顺序一致（语言不同但角色相同）。英文导语+Why 不含 `ticket` 作为产品词（`create_ticket` / `ticket-agent` / `mock-ticket` / `/tickets` 允许）。不含 `any system` 标题。

- [ ] **步骤 3：** 确认未改 `configs/default.yaml`。无需 `go test`（无代码）。若改了非 README 文件则撤回。

- [ ] **步骤 4：Commit** `docs(readme): 英文 README 与中文旁挂定位对齐`

---

## 自检

| 规格 | 任务 |
|------|------|
| 标题/导语现有 HTTP，无任意系统、无工单产品 | 1、2 |
| Why 四行 | 1、2 |
| 目录顺序 | 1、2 |
| 快速开始无 OS/Docker | 1、2 |
| 事实保留 | 从现 README 搬迁，计划已点名 |
| 注册失败 → 调用连不上 | 1、2 部署节 |

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-14-readme-positioning.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 任务间审查  
2. **内联执行** — 本会话按任务改 README 并设检查点  

选哪种方式？
