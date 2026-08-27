# Baize 设计规格：Connector 默认凭证模式

> 状态：已批准  
> 日期：2026-08-13  
> 前置：会话身份库与 AuthResolver 已落地；Demo C 接入契约已落地  
> 依据：`docs/architecture-and-plugin-protocol.md` §4.4（`passthrough` / `static` / `vault_ref`）；Demo C 后续候选第 1 条

---

## 0. 动机

会话 Identity 已能在对话内捕获并复用登录凭证。Connector 默认凭证仍绑在隐式字段 `auth.bearer_env` 上：启动时读一个环境变量，写入全局 `Invoker.Headers`。架构草案要求的三种来源没有对应配置面：

- 平台把调用方 Token **透传**到遗留 API
- 显式 **静态** 默认头
- 从 **引用**（环境变量 / 本地文件）取密，YAML 不写明文

本里程碑补齐 Connector 默认凭证层。会话身份选择逻辑保持不变。

项目尚无生产版本：**删除 `bearer_env`**，不做旧字段兼容。

---

## 1. 目标与成功标准

**目标：** Connector `auth.mode` 提供无会话身份时的默认 HTTP Headers；有会话身份时三种模式都不覆盖捕获凭证。

**成功标准：**

1. `static`：配置里的 `${ENV}` 展开后，无身份 invoke 带上对应头
2. `passthrough`：`POST /v0/runs` 白名单请求头记在该 Run 上，无身份 invoke 带上；events 与 GET run 看不到秘密
3. `vault_ref`：`env:` / `file:` 在注册时解析；失败则注册失败，不静默空头
4. 同一 `conversation_id` 下已捕获身份时，三种 mode 的默认头都让路
5. GET connector 可看到 `mode` 与引用字符串，看不到解析后的秘密
6. 代码与文档中不再出现 `bearer_env`

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 配置 | `auth.mode` + `static` / `passthrough` / `vault_ref`；删除 `bearer_env` |
| 解析 | static 的 `${ENV}`；vault_ref 的 `env:` / `file:`；失败即 `invalid_auth` |
| Passthrough | POST run / resume 收下白名单头，写入 Run 私有字段，invoke 回退使用 |
| API | PUT connector 接受同一套 `auth`；GET connector 回显 mode 与引用，不回显秘密 |
| 文档 | README 中英、架构草案 §4.4 与本规格对齐 |
| 测试 | 三模式默认头、身份优先、失败注册、脱敏 |

### 不做

- HashiCorp Vault / 云 KMS HTTP 客户端
- OAuth、refresh、401 自动换马
- Chat UI 粘贴 token 或展示透传头
- 跨用户鉴权网关、多租户 IAM
- 侧车插件协议 v0（仍是下一项后续候选）

---

## 3. 架构

```
YAML / PUT connector
  │  auth.mode + 对应块
  ▼
Default Credential Provider（注册时解析 static / vault_ref）
  │  DefaultHeaders
  ▼
POST /v0/runs · POST .../resume
  │  passthrough：白名单头写入 Run（私有）
  ▼
OpenAPI tool 闭包
  │  AuthResolver：会话身份优先，否则 DefaultHeaders
  │  passthrough 时 DefaultHeaders = 该 Run 记下的头
  ▼
遗留 HTTP API
```

**不变：** `OpenAPISecurityResolver` 选身份的顺序；登录 capture；HITL 门闸。本里程碑只替换「默认 Headers 从哪来」。

**优先级（每次 invoke）：**

1. 强制 `identity_id`（若有且存在）
2. 会话内未过期、scheme 匹配的 Identity
3. Connector 默认 Headers（本里程碑的三种 mode）
4. 空头，由下游 401

---

## 4. 配置契约

缺省 `mode` 为 `static`。YAML 与 `PUT /v0/connectors/{id}` 的 `auth` 对象同一形状。

```yaml
connector:
  auth:
    mode: static          # static | passthrough | vault_ref
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
    passthrough:
      headers: [Authorization]   # 缺省仅 Authorization
    vault_ref:
      headers:
        Authorization: "env:BAIZE_CONNECTOR_TOKEN"
        # Authorization: "file:./secrets/ticket.token"
    capture:                     # 现有登录捕获，语义不变
      tool_name_glob: "*login*"
      token_json_paths: ["accessToken", "data.accessToken", "data.token"]
      label_json_paths: ["email", "data.email"]
      header_template: "Bearer {{token}}"
```

开箱样例 `configs/default.yaml`：`mode: static`，`static.headers.Authorization` 使用 `${BAIZE_CONNECTOR_TOKEN}`。本地密钥只放 `.env` / `file:`，不进 git。

### 4.1 `static`

- 每个 header 值在**注册时**展开 `${ENV}`（仅完整一段环境变量名，如 `${BAIZE_CONNECTOR_TOKEN}`）。
- 未设置或展开结果为空 → 注册失败，`invalid_auth`。
- 不支持嵌套、默认值语法、或把明文 token 写进仓库样例。

### 4.2 `passthrough`

- 白名单缺省 `["Authorization"]`。显式空列表合法，表示无默认凭证。
- 只从 **发起该 Run 或 resume 的那次 HTTP 请求** 取头；异步 Execute 从 Run 记录读取。
- 白名单外头一律忽略，不报错。空值丢掉。
- Resume：resume 请求上的白名单头**覆盖**该 Run 已存透传头；没带则沿用创建时记下的。
- `/ui` 不传白名单头时，无身份 invoke 即无默认凭证。

### 4.3 `vault_ref`

- 每个 header 值必须是 `env:NAME` 或 `file:path`。
- **注册时解析一次**，写入该 Connector 的 DefaultHeaders。
- `env:`：`os.Getenv`，去首尾空白；空 → 失败。
- `file:`：读文件全文，去首尾空白（含末尾换行）；相对路径相对进程工作目录；指向目录或不存在 → 失败。
- 未知前缀、解析结果为空 → 失败。不静默空 Headers。
- 值原样作为 header。需要 `Bearer ` 前缀就写在 env/文件里，Runtime 不猜。
- 本轮不做路径 `..` 沙箱、不做 Vault HTTP。

### 4.4 删除

- 配置、bootstrap、README、规格引用中的 `bearer_env` / `BearerEnv` / `resolveBearerHeaders`。
- 不为旧 YAML 做别名或迁移。

---

## 5. 控制面与存储

### 5.1 PUT `/v0/connectors/{id}`

请求增加 `auth`（可省略 → `mode: static` 且无 headers，即无默认凭证）。解析失败 → `400 invalid_auth`，不改 Registry。

成功响应与 GET 一样：可含 `auth.mode` 以及 `static`/`passthrough`/`vault_ref` 的**配置形状**（引用字符串），不含展开后的秘密。

### 5.2 GET `/v0/connectors/{id}`

返回已存的 auth 配置形状（mode + 引用 / 白名单 / `${ENV}` 模板），不返回解析后的 token。

Store 的 `Connector` 需能持久化 auth 配置（内存 + SQLite）。秘密解析结果只留在进程内 Invoker/DefaultHeaders，不写 SQLite 明文。

### 5.3 Run 私有透传头

`Run` 增加仅服务端使用的 `PassthroughHeaders map[string]string`（JSON 字段可省略或 `json:"-"`）。

- CreateRun / resume 写入
- SQLite 可存为独立列或 JSON 列，**不得**出现在 `GET /v0/runs/{id}` 与 events 的 `data` 里
- 完整 token 禁止写入 events

---

## 6. 与 Invoke 的衔接

现有闭包已调用 Resolver，`DefaultHeaders` 来自注册 opts。本里程碑：

- `static` / `vault_ref`：注册时填入 `opts.Headers`
- `passthrough`：`opts.Headers` 在注册时为空；闭包从当前 Run 读 `PassthroughHeaders` 作为本次 `DefaultHeaders`（需能从 ctx 取 `run_id`，或在 injectAuthCtx 时一并注入透传头）

实现约束：异步 `Execute` 不得持有 `*http.Request`。优先把透传头放进 Run，再经已有 `injectAuthCtxFromRun` 注入 context。

---

## 7. 错误处理

| 场景 | 期望 |
|------|------|
| 未知 `mode` | 启动失败或 PUT `400 invalid_auth` |
| `static` 的 `${ENV}` 未设置或为空 | 同上 |
| `vault_ref` 未知前缀 / env 空 / 文件不可读或为空 | 同上 |
| `passthrough` 白名单为空 | 合法，无默认凭证 |
| 白名单外头 | 忽略 |
| 解析失败 | 不得部分注册 Connector |

---

## 8. 测试计划

| 场景 | 断言 |
|------|------|
| static 展开 | 无身份 invoke 的 Authorization 等于展开值 |
| passthrough | POST run 带 Authorization → 该 Run 的 tool HTTP 带上同一值 |
| passthrough 脱敏 | events 与 GET run body 不含该 token |
| vault_ref env | 注册后无身份 invoke 带 env 值 |
| vault_ref file | 临时文件内容（含换行）trim 后作为头 |
| vault_ref 失败 | 缺 env / 缺文件 → 注册错误，既有 Tools 不变（PUT 路径） |
| 身份优先 | 三种 mode 下登录捕获后的后续调用使用捕获 token |
| GET connector | 含 mode；vault_ref 显示 `env:NAME` 而非秘密 |

---

## 9. 文档

- README / README.zh-CN：用三种 mode 替换所有 `bearer_env` 说明；平台接入节补一句默认凭证
- `docs/architecture-and-plugin-protocol.md` §4.4：写明 mode 语义与「身份优先」
- 开源样例不提交真实 token 文件

---

## 10. 非目标回顾

本里程碑之后，平台应能回答：「无会话登录时，默认凭证从透传、静态引用还是本地文件来？」  
不承诺 Vault 集群、OAuth、或 UI 贴 token。侧车协议仍是下一项后续候选。

---

*本文档经头脑风暴分节批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
