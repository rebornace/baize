# Baize 设计规格：Agent Skills（渐进激活）

> 状态：已批准  
> 日期：2026-08-21  
> 前置：工具目录与 Tools 设置页可读目录已落地  
> 依据：本轮头脑风暴（方案 1 磁盘包；上传/删除可操作；`activate_skill` 渐进加载；默认 skills 空时仍全量已启用工具）

本规格把 Skill 落成**配置形态**（不升格为第六抽象）：`SKILL.md` 流程说明 + 工具名清单。管理员可安装/删除包；Agent 可声明默认激活；对话中模型通过内置工具按需激活更多 Skill。不修改会话身份、HITL、Connector `auth.mode`、工具目录启停语义。

定位仍是企业遗留 API 的 Runtime 侧车。不承诺与 Cursor / Claude 上 `grill-me`、`superpowers` 等原样子技能调度兼容；仅 frontmatter + Markdown 正文形态尽量可对照常见 `SKILL.md`。

---

## 0. 动机

工具目录可启停、可写人话说明之后，模型仍可能一次看见过多工具，或缺少「先登录再查再建单」一类流程约束。管理员需要可选的 Skill 包：收窄（或叠加）工具子集，并注入流程文案。对话场景下仅靠 YAML 死绑不够用，模型须能按用户意图**发现并激活** Skill。

开箱提供一份 mock-ticket 演示 Skill，证明机制；默认路径不强迫用户理解 Skill 概念。

---

## 1. 目标与成功标准

**目标：** Agent 可挂默认 Skill；已安装 Skill 可被发现并经 `activate_skill` 激活；管理员可上传/删除用户 Skill；开箱演示可用。

**成功标准：**

1. 存在 `skills/ticket-triage/SKILL.md`（或等价路径），`default.yaml` 中 `agent.skills` 含 `ticket-triage`
2. Run 时 system 含 Skill 目录（已安装项的 id + description）及默认已激活 Skill 的正文
3. 默认 `skills` 非空时，模型可见工具 = ∪(默认 Skill `tools`) ∩ 目录已启用；激活更多 Skill 后并集扩大
4. 默认 `skills` 为空时，可见工具 = 全部已启用（与今天一致），且仍可 `activate_skill`
5. `activate_skill` 对未知 id 返回工具错误内容，Run 不失败；对已启用工具名求交，不能启用已停用工具
6. `POST /v0/skills` 接受 `.md` 或 `.zip`，写入用户 Skill 目录；同 id 覆盖
7. `DELETE /v0/skills/{id}` 仅用户包；内置 400；删除后从所有 Agent 的 `skills` 数组移除该 id
8. 操作员 Skills 写接口与改 `agent.skills` → 403；管理员可以
9. 现有 mock-ticket 开箱集成路径在挂上演示 Skill 后仍可通过
10. 设置页可列表、上传、删除用户包，并编辑默认 Agent 的 `skills` 勾选

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 包格式 | `SKILL.md` frontmatter（`name`/`description`/`tools`）+ 正文 |
| 加载 | 内置目录 ∪ 用户目录；同 id 用户覆盖内置 |
| Run | 目录段、默认激活正文、`activate_skill`、工具并集求交 |
| API | GET 列表/详情、POST 上传、DELETE；Agent PUT/GET 含 `skills` |
| GUI | 设置 → Skills：列表、上传、删除、默认 Agent 勾选 |
| 开箱 | `ticket-triage` + YAML 引用 |
| 文档 | README 中英；架构草案 §5 补发现/激活 |

### 不做

- 在线富文本编辑 Skill 正文（改包 = 再上传覆盖）
- Skill 市场、自动映射外界工具名
- 会话级跨 Run 永久激活记忆
- 开跑前单独「路由 LLM」选 Skill（本规格采用 `activate_skill`）
- 批量工具目录 API、用模型生成 Skill
- 将 Skill 升格为与 Runtime/Agent/Tool/Connector/Run 并列的第六抽象

---

## 3. 包格式与落盘

### 3.1 目录

| 配置 | 默认 | 含义 |
|------|------|------|
| `skills.builtin_dir` | 相对仓库或工作目录的 `./skills` | 内置包（可随发行附带） |
| `skills.user_dir` | `./data/skills` | 上传写入；可写 |

开机（及上传/删除后）合并：扫描两目录下一级子文件夹；每个须含 `SKILL.md`。同 id：**user 覆盖 builtin**。`source` 标记为 `builtin` 或 `user`。

### 3.2 单包布局

```
<dir>/<id>/
  SKILL.md
```

`SKILL.md` 示例：

```markdown
---
name: ticket-triage
description: 工单分诊与建单流程
tools:
  - login
  - list_tickets
  - get_ticket
  - create_ticket
---

# 工单分诊

1. 需要登录时先调用 login
2. 先 list/get，再 create
```

| 字段 | 规则 |
|------|------|
| 文件夹名 | Skill **id** |
| `name` | 须与 id 一致；上传仅文件时可用 name 作为 id |
| `description` | 列表与目录提示用；可空 |
| `tools` | 字符串数组；与目录已启用求交；未知或未启用名跳过并打日志，不拒绝加载 |
| 正文 | frontmatter 之后全部；激活后注入 system |

### 3.3 上传 zip

- `multipart` 字段名 `file`
- 解压后在根目录或**唯一**一层子目录找到 `SKILL.md`
- 禁止 `..` 与绝对路径逃逸
- 同 id 再上传 = 覆盖用户目录中该包

### 3.4 开箱

仓库提供 `skills/ticket-triage/`。`configs/default.yaml`：

```yaml
agent:
  id: ticket-agent
  system: "…"
  skills:
    - ticket-triage
```

---

## 4. Agent 绑定与 Run 行为

### 4.1 Agent 模型

`store.Agent` / YAML / PUT body 增加可选 `skills []string`（默认已激活的 id，顺序保留）。省略或空 = 无默认激活正文；工具策略见 §4.3。

### 4.2 System 组装（每次 Run / resume 建 messages）

1. Agent `system`
2. 若安装集非空：固定 **Skill 目录**段——每个已安装 Skill 一行：`{id} — {description}`（description 空则用 name/id）
3. 对**当前激活集**中每个 id（见 §4.4）：追加

```text
## Skill: {id}
{正文}
```

某默认 id 无法加载：跳过并 warn，不使 Run 创建失败。

### 4.3 可见工具

```text
初始可见 =
  若默认 skills 非空：∪(各默认 Skill.tools) ∩ Registry.enabled
  若默认 skills 为空：全部 Registry.enabled
每次 activate_skill 成功后：
  可见 ∪= (该 Skill.tools ∩ Registry.enabled)
```

另：当安装集非空时，Registry 暴露内置工具 `activate_skill`（见 §4.5）。Skill 不能把已停用目录行变启用。

### 4.4 激活集作用域

- **初始激活集** = Agent.`skills` 中能加载到的 id
- `activate_skill` 成功则加入**本 Run** 激活集（resume 同一 Run 保持）
- **新 Run** 从 Agent 默认 `skills` 重新开始；第一版不做会话级永久激活

### 4.5 `activate_skill`

| 项 | 约定 |
|----|------|
| 注册条件 | 安装集非空时注册 |
| 参数 | JSON：`id`（string）或 `ids`（string 数组）；至少一者 |
| 成功 | 更新本 Run 激活集；工具可见集按 §4.3 扩大；返回已激活 id 与新增工具名摘要 |
| 未知 id | 工具结果 `is_error` 或等价错误文案，Run 继续 |
| 已激活 | 幂等成功提示 |
| HITL / require_login | 不适用（非下游业务写）；不要求审批 |

实现可将「正文已激活」作为 tool 结果说明，并保证**后续** LLM 轮次的 system/工具列表已包含该 Skill（具体是重建 system 还是在引擎侧持有 Run 级 overlay，由实现计划选定，语义须满足本规格）。

### 4.6 多 Skill

默认可多个；可多次激活。提示按激活顺序追加；工具名并集去重。

---

## 5. API 与 ACL

与改 Agent / Connector 相同：管理员可写；操作员 403；控制面门开着时与今天一致。

| 方法 | 路径 | 行为 |
|------|------|------|
| GET | `/v0/skills` | `{ skills: [{ id, name, description, tools, source }] }` |
| GET | `/v0/skills/{id}` | 元数据 + `body`（Markdown 正文） |
| POST | `/v0/skills` | multipart `file`；200 + 摘要 |
| DELETE | `/v0/skills/{id}` | 仅 `source=user`；内置 400；摘掉各 Agent.`skills` 中的 id 并持久化 Agent |
| PUT/GET | `/v0/agents/{id}` | body/响应含可选 `skills` |

不提供 PATCH 改正文。

错误码：`400 invalid_request`（坏包、删内置）、`404 not_found`、`403 forbidden`、`401 unauthorized`（门禁与今天一致）。

---

## 6. GUI

设置导航增加 **Skills**（仅管理员）：

- 列表：id、description、source 徽章、tools 摘要
- 上传（md/zip）
- 删除（仅 user）
- 编辑默认 Agent（开箱为 `ticket-agent`，若仅一个 Agent 则用它）的 `skills` 多选，保存 PUT Agent

不改聊天主区信息架构（无强制「先选 Skill」下拉）。不做在线编辑器。

---

## 7. 热更新

- POST/DELETE 后安装集立即刷新；之后新建的 Run 用新目录
- 进行中 Run：已注入正文与已激活集不回滚；不强制热替换
- PUT Agent.`skills` 只影响之后的新 Run

---

## 8. 测试

- 解析 frontmatter；user 覆盖 builtin
- 默认 skills 注入正文；目录含全部已安装；默认空 → 全量 enabled
- `activate_skill`：扩大工具；幂等；未知 id
- 上传 md/zip、覆盖、删 user、删 builtin→400；DELETE 后 Agent 摘 id
- ACL 操作员 403
- 开箱集成：挂 `ticket-triage` 时 mock-ticket 路径仍绿

---

## 9. 文档

- `README.md` / `README.zh-CN.md`：Skills 可选；上传删除；默认激活与 `activate_skill`；与工具目录启停关系；非个人编码 Agent Skill 市场
- `docs/architecture-and-plugin-protocol.md` §5：发现/激活语义；强调不升格
- 配置示例：`skills.user_dir` / `agent.skills`

---

## 10. 实现时注意

| 名称 | 作用 |
|------|------|
| Skill 加载器 | 扫目录、解析 YAML frontmatter |
| Run 级激活集 / 工具过滤 | Engine 或 Registry 视图 |
| `activate_skill` Invoker | 内置工具 |
| API + Settings Skills 页 | 操作闭环 |
| `skills/ticket-triage` | 开箱演示 |

Git：不在 `main` 上改实现代码。规格与计划批准后再开功能分支。

---

*本文档经头脑风暴分节批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
