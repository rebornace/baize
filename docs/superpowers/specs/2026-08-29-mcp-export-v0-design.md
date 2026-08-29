# MCP 导出（X1）v0 设计规格

> 状态：已批准（2026-08-29）  
> 日期：2026-08-29  
> 前置：工具目录、OpenAPI / HTTP 侧车 / MCP 桥（客户端）、会话身份与 `require_login`、控制面 Gate  
> 依据：头脑风暴——白泽作企业数据/工具闸门，个人 Agent 做复杂分析；方案 A 目录门面；专用可撤销 Key + 导出身份  
> 路线：本里程碑（X1）→（排队）X2 UI 切模型 / X3 中间件多源 / F 生产硬化  

---

## 1. 目标与成功标准

**目标：** 白泽作为 **MCP Server（Streamable HTTP）**，将工具目录中符合导出策略的工具暴露给个人 Agent（如 Cursor）；以 **多把可撤销的 MCP 导出专用 Key** 鉴权（与 Gate 分离）；每把 Key **必须**绑定设置中单独维护的 **导出身份**；`tools/call` 经现有 Tool Registry 执行并注入该身份，供 `require_login` 类工具访问企业 API。默认只读（HTTP 方法启发式 + 手动覆盖）；数据库类 MCP **禁止写操作对外**。个人 Agent 侧消耗其自身模型额度；**不**因此消耗白泽绑定的 LLM 额度。

**成功标准：**

1. 兼容客户端使用 Base URL + `Authorization: Bearer <mcp_export_key>` 完成 MCP `initialize` / `tools/list` / `tools/call`。  
2. 仅导出策略允许的工具出现在 list；策略外或库写类 call 被拒绝。  
3. 可创建/列表/撤销多把 Key；创建时必须绑定导出身份；明文 Key 仅创建响应出现一次。  
4. `require_login` 工具在 MCP 路径使用 Key 绑定的导出身份；身份失效返回明确错误（如 `export_identity_expired`）。  
5. `tools/call` **不**创建 Baize Run、**不**调用白泽 LLM。  
6. 假 MCP 客户端单测/集成测绿；README 中英区分「MCP 客户端」与「MCP 导出」；公网 HTTPS 与 Key 保管说明。  
7. Gate token 不能充当 MCP Key；MCP Key 不能调用管理 API / 登录 `/ui`。

**明确不做：**

| 项 | 原因 |
|----|------|
| 复用 Gate admin/operator token 作 MCP 鉴权 | 泄露面过大；与导出专用 Key 分离 |
| 本机 stdio 传输 | 公网个人 Agent 场景；v0 仅 HTTP |
| OAuth 2.1 | v0 过重 |
| 按 Key 的不同工具集 | v0 各 Key 同一全局导出策略 |
| 独立「导出目录」副本（方案 B） | 易漂移；采用方案 A 目录门面 |
| 每次 call 开 Baize Run（方案 C） | 与「个人 Agent 做分析」分工相反，且耗白泽 LLM |
| MCP Resources / Prompts | v0 仅 tools |
| 对外写路径 + HITL | v0 以只读为主；见 §3.7 |
| 微信 allowlist / X2 / X3 | 非本里程碑 |

---

## 2. 架构与调用链

**选定方案 A — 目录门面：** `tools/list` / `tools/call` 挂在现有 Tool 目录与 Registry / Connector invoke 上，按导出策略过滤，call 时注入导出身份。不另建导出工具副本。

```text
个人 Agent (Cursor 等)
        │  HTTPS Streamable HTTP
        │  Authorization: Bearer <mcp_export_key>
        ▼
┌────────────────── Baize MCP Export 门面 ──────────────────┐
│  鉴权：校验 Key → 解析绑定的 export_identity_id            │
│  tools/list：Tool 目录 ∩ 导出策略                           │
│  tools/call：Registry.Invoke + 注入导出身份到 context       │
└───────────────┬───────────────────────┬───────────────────┘
                │                       │
                ▼                       ▼
         OpenAPI / 侧车              已接 MCP（如 DBHub）
         （静态凭证或导出身份）        （仅查询类可导出）
```

| 组件 | 角色 |
|------|------|
| Tool 目录 / Registry | 唯一真相；`source` 为 `spec` / `plugin` / `mcp` / `extra` 一并适用 |
| 导出策略层 | list/call 前过滤；库写类硬拒绝 |
| 导出 Key 存储 | id、显示名、密钥哈希、身份绑定、创建/撤销时间 |
| 导出身份库 | 设置页单独维护；不绑定聊天 `conversation_id` |
| Gate / LLM / Run 引擎 | **不参与** MCP `tools/call` |

**身份注入：** `/ui` Run 今日经 `conversation_id` 取会话身份。MCP 路径由 Key → 导出身份，在 invoke context 中提供与「已登录会话身份」等价的凭证，复用 OpenAPI/侧车 `require_login` 逻辑，**不**创建聊天会话、**不**开 Run。

**审计（最小）：** Key id（非明文）、tool name、成功/失败、时间；下游 token 不入日志。

**额度：** 个人 Agent 消耗自有模型；白泽侧为下游 API/DB/侧车调用，不走 `llm.provider`。

---

## 3. 导出策略与安全

### 3.1 可见与可调前提

须同时满足：

1. 目录中 `enabled == true`  
2. 通过导出策略（§3.2–§3.4）  
3. Bearer 为未撤销的导出 Key  

否则：list 不可见；call → 4xx（不存在或禁止导出）。

### 3.2 默认启发式（工具带 HTTP `method`）

| method | 默认对外 |
|--------|----------|
| `GET`、`HEAD` | 允许 |
| `POST` / `PUT` / `PATCH` / `DELETE` | 拒绝 |
| 空 / 未知 | 拒绝（侧车、部分 MCP 常见） |

### 3.3 手动覆盖（仅 admin）

每工具导出状态：

| 值 | 含义 |
|----|------|
| `default` | 走 §3.2 |
| `force_allow` | 强制允许导出（如 POST 只读查询） |
| `force_deny` | 强制禁止（如敏感 GET） |

可通过 `PATCH /v0/tools/{name}` 扩展字段与 Tools 设置页修改。

### 3.4 数据库 / MCP 写操作硬规则

对 `source=mcp` 的工具（及文档标明的 DB 类 Connector）：

- **写类永不导出**，即使 `force_allow`  
- v0 判定（实现按此写测，错杀优于误放）：  
  1. Connector 可标 `export_db_readonly=true`（DB 类默认建议开启）：仅允许名称/描述匹配只读模式的工具（如 `query` / `select` / `list_*` 等，计划列具体规则）；或  
  2. 名称/描述命中写信号（`insert` / `update` / `delete` / `drop` / `execute` 写语句等）→ 拒绝导出  

原则：**错杀（少暴露）优于误放（写出库）。**

### 3.5 导出 Key

| 规则 | 说明 |
|------|------|
| 多把 | 可命名；只存哈希；可单独撤销 |
| 创建 | **必须**绑定 `identity_id`；无「未绑定」状态 |
| 工具集 | 各 Key 相同（全局策略） |
| 与 Gate | 分离；Gate token ≠ MCP Key |
| 明文 | 仅创建 API 响应出现一次 |

### 3.6 导出身份

- 设置中单独 CRUD（名称、凭证材料；可走登录捕获写入该档，**不**挂到某个 `/ui` 会话）  
- 供 MCP 路径上 `require_login` 工具使用  
- 过期/失效 → 明确错误，不静默降级匿名  

文档要求：优先使用**只读服务账号**作为导出身份。

### 3.7 `require_approval` 与写工具

v0：带 `require_approval` 的工具 **不得导出**（list 隐藏；call 拒绝），避免 MCP 无人值守审批。  
`force_allow` 的非审批写工具：允许导出，但库写硬规则仍优先。HITL 对外路径留后续。

---

## 4. API、设置页与配置

### 4.1 对外 MCP 端点

| 项 | 约定 |
|----|------|
| 传输 | Streamable HTTP |
| 路径前缀 | `/v0/mcp/export`（协议所需 GET/POST 等均挂此前缀；路径固定） |
| 鉴权 | `Authorization: Bearer <mcp_export_key>` |
| 能力 | `initialize`、`tools/list`、`tools/call`；不做 Resources/Prompts |

该端点 **RoleNone + 专用 Key**（非 Gate 中间件的 admin/operator 角色）。

### 4.2 管理 API（仅 admin）

| 方法 | 路径 | 作用 |
|------|------|------|
| GET/POST | `/v0/settings/mcp-export/identities` | 列出 / 创建导出身份 |
| GET/PATCH/DELETE | `/v0/settings/mcp-export/identities/{id}` | 读改删 |
| GET/POST | `/v0/settings/mcp-export/keys` | 列出（无明文）/ 创建（一次性明文） |
| DELETE | `/v0/settings/mcp-export/keys/{id}` | 撤销 |
| GET | `/v0/settings/mcp-export` | 端点 URL 提示、`enabled` 等 |
| PATCH | `/v0/tools/{name}` | 扩展 `export`: `default` \| `force_allow` \| `force_deny` |

创建 Key 缺少 `identity_id` → `400`。

### 4.3 设置 UI

| 页 | 内容 |
|----|------|
| **MCP**（已有） | 白泽当**客户端**注册外部 Server（不变） |
| **MCP 导出**（新） | 导出身份；Key 创建/撤销；Base URL + 客户端配置示例；链到 Tools |
| **Tools** | 每行导出状态；库写不可导出提示 |

仅 admin 可见「MCP 导出」。

### 4.4 进程配置

- `mcp_export.enabled`（或等价）：可关闭整个导出面；默认 **true**（无 Key 则无法调用）  
- Key 与身份存 store（SQLite/PG），不进 git  
- 公网部署文档强制 HTTPS  

### 4.5 客户端配置示例（文档）

```json
{
  "mcpServers": {
    "baize-export": {
      "url": "https://<host>/v0/mcp/export",
      "headers": {
        "Authorization": "Bearer <mcp_export_key>"
      }
    }
  }
}
```

字段名以当时主流客户端（Cursor 等）对 Streamable HTTP MCP 的要求为准，README 跟进。

---

## 5. 测试、文档与验收

### 5.1 测试

- 导出策略矩阵：GET 默许、POST 默认拒、覆盖、无 method、MCP 写硬拒（含 force_allow）  
- Key：必绑身份、哈希、撤销 401、明文一次性  
- 身份注入与失效错误码  
- 假 Streamable HTTP 客户端集成：list 过滤 + call 命中 Registry  
- Gate token ≠ MCP Key；MCP Key 不可调管理 API  
- 回归：既有 MCP **客户端**桥不受影响  

真机 Cursor 为手工验收，不阻塞 CI。

### 5.2 文档

- README 中英：MCP 导出 vs 客户端；HTTPS；Key/身份；不耗白泽 LLM  
- 开源边界笔记：X1 → 规格已批准  

### 5.3 风险

| 风险 | 缓解 |
|------|------|
| 误 `force_allow` 写接口 | 默认偏拒；库写硬拒；admin only |
| 导出身份权限过大 | 文档强调只读服务账号；可撤 Key/身份 |
| Key 泄露 | 多把可撤；哈希落盘；与 Gate 分离 |
| 客户端协议差异 | 协议级假客户端单测；文档跟进 |
| 与 `/settings/mcp` 混淆 | 独立导航「MCP 导出」 |

### 5.4 验收清单

1. 导出 Key + HTTP MCP 完成 list + call  
2. §3 策略与硬规则生效  
3. Key/身份可管理可撤销  
4. call 不开 Run、不调白泽 LLM  
5. 约定测试绿；README 可照做  

### 5.5 后续（非 v0）

按 Key 收窄工具；stdio；OAuth；Resources；对外写 + HITL；更细的 DB 只读分类。

---

## 6. 与 Inbox / 微信的关系

| | Inbox | 微信 Channel | MCP 导出（X1） |
|--|--------|--------------|----------------|
| 调用方 | 企业系统 HMAC | 微信 peer | 个人 Agent MCP 客户端 |
| 是否开 Run | 是（或 resume） | 是 | **否**（只 invoke 工具） |
| 鉴权 | Channel secret | iLink 登录 | 导出专用 Key + 导出身份 |

三者并存，互不替代。

---

*本文件仅存在于 baize_real（`docs/superpowers`）；不进入 public 开源导出。*
