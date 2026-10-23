<p align="center">
  <img src="docs/images/baize-banner.jpg" alt="白泽 Baize" width="100%">
</p>

[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)

# Baize（白泽）

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

[English](README.md) | **中文**

**官网：** [rebornace.github.io/baize](https://rebornace.github.io/baize/)

[![baize - Lightweight AI Agent Framework | Product Hunt](https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1255168&theme=light&t=1789790934744)](https://www.producthunt.com/products/baize?embed=true&utm_source=badge-featured&utm_medium=badge&utm_campaign=badge-baize)

白泽是挂在现有业务旁边跑的团队 AI 助手。它读接口文档、调用你们已有的系统，需要时在网页操作台等人确认，再把结果写回对话或消息渠道。业务代码通常不用改。

命名取自传说中通晓万物之名的白泽。网页操作台给产品、运营和集成同学用。

---

## 能做什么

**接业务。** 上传或指定接口文档（OpenAPI / Swagger 等），每个接口成为助手可调用的一项能力；过程在操作台可见。文档不全时，可自建一个小服务声明能力与调用方式，或让白泽回调你们自己的地址由你们执行。也可接入支持 MCP 的外部工具服务。

**写操作可先审批。** 建单、改状态等可停住，等同事在卡片上批准或驳回后再执行，过程可追溯。能力可随时停用；完整密钥不进对话回放。

**一个助手，多种入口。** 操作台、告警/工单来信、即时消息共用审批、记忆和对外回复。仓库正式支持个人微信私信；飞书、钉钉有参考示例。事件也可推到你们自己的通知地址。

**对方系统要登录。** 对话里用「浏览器登录」，或在设置里配置登录页。密码不经大模型；登录后再调业务。已接好的能力可只读提供给 Cursor、Claude Desktop、豆包等；登录过期时打开返回的链接，不要把密码贴进对话。登录页地址需对对方浏览器可达。

**系统很多时先收窄。** 设置「匹配与决策」和「运行参数 → 智能提速」后，白泽可先判断本轮大概用哪些系统与能力，再把较短列表交给主模型。失败则退回全量列表。只配判断模型、不打开开关，不会省主模型用量。财务、交易等可分组；需要互不影响的审批时，可另开一条子任务。

**接口变了、做过的事能查。** 重新导入接口后可对照变更（也可发通知）。每轮对话旁可查看当时用的模型与能力。设置里有运行指标与近期日志；支持常见监控与可选链路追踪；部分观测选项不必改配置文件重启。出站可配本机证书加密。

**长对话与日程。** 过长对话会自动摘要；可选「长对话保留重点」。特别大的工具结果或粘贴可存成工作文件，对话里留摘要。人同意后可设一次性、每天或每周指令，到点在原对话开新一轮（写操作仍审批）。可记账号级要点、带附件。会话可分叉、回退。

**工作区。** 侧栏切换或新建工作区；新对话落在当前工作区（暂不支持挪旧对话）。同一工作区内共享已登录业务账号；工作区之间、微信等渠道、以及「对外提供能力」的身份分开。内置帮助技能（中英文）会指到操作台对应页，不改配置、不重启服务。

**操作台。** 中英文；对话、调用卡片、设置与运行参数。「匹配与决策」「对外登录」「对外提供能力」「运行指标」「评测语料」等在设置里。提供 HTTP 管理接口，不强制某语言 SDK。默认本地数据库可起步。

试用可用内置模拟模型（不必先有云厂商密钥）；正式环境再配真实模型。单独一个进程，配置在外；停掉后业务侧几乎不留痕迹。

---

## 谁在用这类能力

线上系统暂时不改代码：旁挂白泽，用接口文档交能力，先在内网试查询、建单、审批。

助手可查数、起草、建议；改状态、发指令时由同事在操作台确认。

外部工具、自研插件、来信和即时消息走同一套审批与记忆。

多个业务域要分开：分组管理；需要并行且互不影响时再开子任务。

在 Cursor 等客户端里复用已接能力：在白泽接好并导出，过期打开登录链接。

接口会变、过程要留痕、出站要加密：对照导入差异、查看当轮绑定、配证书与监控，而不另搭一套助手。

---

## 系统很多时：先收窄再交给主模型

每次把全部能力塞进主模型会又慢又贵。打开智能提速后，白泽先做一轮判断，再交较短列表；失败则用全量列表。步骤与参数见 [架构](docs/developers/architecture.md)、[配置](docs/developers/configuration.md)。

### 实测

在 3 个后台、共 390 项能力上，用 37 条真实只读请求测主模型首轮输入量；连跑 5 轮共 185 次。脚本：[`scripts/tool-routing-eval`](scripts/tool-routing-eval/README.md)。

| 每轮交给模型的能力上限 | 成功 | 平均实发能力数 | 平均首轮输入量 | 相对上限 32 |
|---|---|---|---|---|
| 32 | 36/37 | 43.1 | 4,703 | — |
| 24 | 37/37 | 37.4 | 3,980 | −15% |
| **16（默认）** | **37/37** | 30.6 | **3,090** | **−34%** |
| 12 | 37/37 | 27.1 | 3,033 | −36% |
| 8 | 35/37 | 23.6 | 2,600 | −45% |

全部 390 项直接进首轮时，输入量约 8.5 万；默认上限 16 时约 1.4k–3.8k。连跑成功率约 99.5%（184/185）。同机对比：用本地小模型做判断，整批端到端大约比「再用一个大模型判断」快 1.5 倍量级。完整字段见开发者文档与上述脚本目录。

---

## 快速开始

需要 Go 1.25+、Node.js 20+（会构建网页操作台），以及兼容 OpenAI 接口的模型密钥。

```bash
cp .env.example .env        # 填写 BAIZE_API_KEY=sk-...
```

```powershell
.\serve.cmd                 # Windows
```

```bash
./scripts/serve.sh          # macOS / Linux
```

- 操作台：http://127.0.0.1:8080/ui
- 跳过前端重建：`.\serve.cmd -SkipUI` / `BAIZE_SKIP_UI=1`
- 强制重建前端：`.\serve.cmd -ForceUI` / `BAIZE_FORCE_UI=1`

打开操作台即可发消息。换模型或接业务：复制 `configs/config.yaml` 为 `configs/config.local.yaml`（已忽略进库），用 `-config` 启动。

---

## 预编译包部署（不必装 Go）

从 [GitHub Releases](https://github.com/rebornace/baize/releases) 下载对应平台压缩包。

```text
baize/
  baize                 # Windows 为 baize.exe
  weixin-adapter        # 仅用微信渠道时需要
  configs/config.yaml
  .env
  data/
```

```bash
BAIZE_API_KEY=sk-你的模型密钥
BAIZE_SETTINGS_KEY=一串足够长的随机字符
# 可选：BAIZE_LISTEN=:8080
```

```bash
./baize serve
# 或：./baize serve -config configs/config.local.yaml
```

操作台：http://127.0.0.1:8080/ui  

改端口、模型地址等后重启进程。多数业务接入可在操作台完成。Linux 与微信渠道见 [部署说明](docs/developers/deployment.md)。

---

## 开发者文档

构建、测试、配置、部署与接口（含系统名词，中英双语）：

- [开发者文档（中文）](docs/developers/README.md) · [English](docs/developers/README.en.md)
- [移动端](docs/developers/mobile.md)：手机连远端助手，以及机上精简编排 · [EN](docs/developers/mobile.en.md)

---

## 许可证

[MIT](LICENSE)
