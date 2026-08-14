# Baize 设计规格：README 定位（现有服务端旁挂 AI）

> 状态：待用户审查  
> 日期：2026-08-14  
> 前置：OpenAPI Connector、HTTP 插件 v0、HITL、会话身份、跨平台/Docker 文档已落地  
> 依据：首页「系统」易被读成操作系统；产品方向是让**没有 AI 的现有服务端**快速、无污染地接上 AI

---

## 0. 动机

当前 README 标题「旁挂任意系统」+ 快速开始前部的 Linux/macOS/Windows 交叉编译，会让人以为卖点是跨 OS。工单样板在标题和 Why 里出现，会把白泽收成工单机器人。

实际方向：已有 HTTP 服务不用改业务代码，旁挂 Runtime 即可获得 Agent / 工具 / 审批；停掉进程不留痕。工单只是仓库里的演示 HTTP 服务。

本里程碑只改 `README.md` 与 `README.zh-CN.md`，中英同一结构。不改代码、不改 `configs/default.yaml`。

---

## 1. 目标与成功标准

**目标：** 打开 README 十秒内读到「现有服务端旁挂 AI、不改代码、卸下不留痕」；操作系统与 Docker 在后部「部署」；工单不作为产品定义。

**成功标准：**

1. 标题与导语使用「现有 HTTP / 服务端」，不出现「任意系统」易混表述；不出现工单作为产品场景  
2. Why 四行对准「已有服务、无 OpenAPI、写操作风险、拆得干净」  
3. 目录顺序：导语 → 30 秒跑通 → 接到你的服务 → 写操作与身份 → 部署 → 附录  
4. 快速开始不含 OS 列表、交叉编译、Docker  
5. 现有能力事实全部保留（见 §5），仅调整位置与称呼  
6. 中英平行；「只起 baize 导致注册失败」改为调用阶段连不上演示 HTTP

---

## 2. 范围

### 做

- 重写中英 README 的标题、导语、Why、概念一行（Agent：提示词 + LLM）
- 按 §3 重排章节
- 演示 curl 保留，改称呼为「演示服务上的写接口」；工具名仍为 `create_ticket`
- 修正 compose「只起 baize」的失败时机说明

### 不做

- 改 Runtime / 配置 / Docker 文件
- 把演示服务改名（代码与默认 yaml 仍是 mock-ticket / `create_ticket`）
- 新增截图、官网、changelog
- 把 HTTP 插件或 HITL 从 README 删掉

---

## 3. 目录（中英同一顺序）

| 顺序 | 中文 | English |
|------|------|---------|
| 1 | 导语 + 为什么需要白泽 + 五个概念 | Hero + Why Baize + Core concepts |
| 2 | 30 秒跑通 | Try it in 30 seconds |
| 3 | 接到你的服务 | Point it at your APIs |
| 4 | 写操作与身份 | Approvals and identities |
| 5 | 部署 | Run and deploy |
| 6 | 附录（UI 构建、命令、文档、许可证） | Appendix |

第 3 节合并：OpenAPI PUT、HTTP 插件、真实 LLM/`default.local.yaml`/关 mock。  
第 4 节：HITL 说明（演示 curl 若已在第 2 节则此处不重复大段）、会话身份、对话记忆。  
第 5 节：`go run` 补充启动器、`CGO_ENABLED=0` 三 OS 二进制、Docker compose 与 `docker run`（现「服务端（Docker）」整段改挂此标题，避免「服务端」= Linux 主机 与「现有服务端业务」撞车）。

---

## 4. 导语文案（锁定语义，实现时可微调用词）

**中文标题：** 不改你的服务，旁挂一层就能用上 AI。卸掉不留痕。

**中文导语：** 白泽是旁挂在现有 HTTP API 旁边的 Agent Runtime：业务进程不用改、不用嵌 SDK。有 OpenAPI 就自动变成工具；没有就加一个小 HTTP 插件。写操作可人工审批，用完把进程停掉即可。

**英文标题：** Add AI next to your existing HTTP APIs — no rewrite, nothing left behind.

**Why（语义）：**

| 痛点 | 做法 |
|------|------|
| 服务端已经在跑，没有 AI，也不想动代码 | 旁挂 Runtime，Connector 把接口变成 Tool |
| 没有 Swagger / OpenAPI | HTTP 插件侧车 |
| LLM 直接打写接口不放心 | `require_approval` |
| 接完拆不干净、绑死平台 | 单进程、配置在外，停掉即走 |

禁止：标题/Why 使用「工单」；禁止在导语强调 Linux/Windows。

---

## 5. 必须保留的事实

端口 8080 / 18080、mock LLM、`/ui`；`go run ./cmd/baize start`。  
OpenAPI PUT、同 id 替换、坏 spec 不污染；PUT 不挂 Identities 的说明。  
HTTP 插件三端点与 `X-Baize-Protocol: v0`；默认 start 仍是 OpenAPI 样板。  
`auth.mode` 三值、身份优先、捕获 `*login*`。  
SQLite 对话、`max_messages`、清空聊天不退出。  
`default.local.yaml`、`.env`、关 mock、`base_url` 指向真实服务。  
`start.sh` / `start.cmd`、交叉编译、compose 两服务、试用 token 默认 `dev`、`docker run` 挂 yaml。

演示 curl 可保留 `create_ticket` 字符串；叙述用「演示服务的写接口」，不用「工单产品」。

Compose：试用请两服务一起起；只起 Runtime 时，打到演示 HTTP 的工具会连不上（不是注册失败）。

---

## 6. 非目标

不承诺改演示域名、不重画架构图、不把 README 写成营销落地页（无虚假指标）。篇幅与现在相当或略短，靠重排而不是加长。

---

*本文档经头脑风暴分节批准后落盘。*
