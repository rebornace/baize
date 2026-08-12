# Baize 设计规格：会话身份库与可插拔鉴权解析

> 状态：待用户审查（头脑风暴已批准方案 3）  
> 日期：2026-08-12  
> 前置：Demo B（Chat + HITL）、OpenAPI Connector + 静态 `bearer_env`  
> 问题背景：静态 env token 无法支持管理台/用户等多身份；对话内登录成功也不会写入后续 HTTP 调用

---

## 0. 动机

当前 OpenAPI Connector 在注册时用 `auth.bearer_env` 解析一次，写入 `Invoker.Headers`，进程内全局共享。结果：

- 只能代表一个固定身份（例如普通用户 JWT）
- Agent 调用 `*login*` 拿到的 `accessToken` **不会**覆盖后续 invoke
- 企业权限模型各异，不能把「admin path / user role」写进 Runtime

**目标定位：** 会话内记住多个凭证；默认按 OpenAPI security 自动选用；企业可插拔自定义 Resolver；UI 可查看 / 退出 / 设默认。

---

## 1. 目标与成功标准

**目标：** 用「会话身份库 + 可插拔 Auth Resolver」替换「唯一写死 token」作为主路径；`bearer_env` 仅作启动兜底。

**成功标准：**

1. 对话中登录成功后，**同一 `conversation_id` 下后续 Run** 调用受保护接口时自动带上捕获的凭证  
2. 会话可同时记住多个身份；list API/UI 可见（脱敏）；支持设默认与退出  
3. 默认 Resolver 按 OpenAPI `securitySchemes` / operation `security` 选型；无企业硬编码  
4. 无会话身份时行为与现网一致（仍可用 `bearer_env`）  
5. 完整 token 不出现在 events 明文与 identities 列表响应中  

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 会话 | `conversation_id`；身份挂在会话，不挂全局 Connector |
| Identity Store | 多身份 CRUD（内存实现即可；可扩展持久化） |
| 登录捕获 | Connector 可配置：工具名模式 + JSON 路径 → 写入 Identity |
| Auth Resolver | 接口 + 默认 `OpenAPISecurityResolver` |
| Invoke | 每次 HTTP 调用合并「Resolver 选出的 Headers」，覆盖静态默认 |
| API | runs 带 `conversation_id`；identities 列表/默认/删除 |
| UI | 绑定 conversation；已登录账号面板；退出/设默认；新对话换 id |
| 测试 | 捕获→复用、双 scheme、退出、强制 `identity_id`、脱敏、env 兜底 |

### 不做（本版）

- OAuth 浏览器跳转、refresh token 协议全家桶  
- 跨设备漫游的服务端用户账户体系 / 多租户 IAM  
- Runtime 内建「admin/user」业务语义  
- 默认开启「401 自动换下一个 token 重试」（可作为 resolver 可选开关，默认关）  
- 手动粘贴任意 token 的导入 API（可列为后续增强）

---

## 3. 架构

```
UI / API
  │  conversation_id
  │  （可选）identity_id 强制本 Run
  ▼
Session Identity Store
  │  多凭证；登录捕获；退出 / 默认
  ▼
AuthResolver（可插拔）
  │  默认：OpenAPI securitySchemes
  │  可选：企业 path/tag/claim 插件
  ▼
OpenAPI Invoker
  │  本次请求 Headers = 会话凭证 ⊎ connector 默认
  ▼
遗留 HTTP API
```

原则：

- Runtime 不写企业角色词  
- 凭证按 `conversation_id` 隔离  
- env bearer 是 `source=env` 的兜底身份，可被会话身份覆盖或退出（是否允许退出 env 由配置决定，默认：**可隐藏但启动时仍可作最终 fallback**）

---

## 4. 数据模型

### 4.1 Conversation

- 客户端生成或服务端首次分配稳定 `conversation_id`（UUID）  
- 「新对话」→ 新 id；旧会话身份库不再用于新对话（可 GC）  
- `POST /v0/runs` **必须**携带 `conversation_id`（UI 始终带；缺省时服务端可生成并在响应中返回，便于脚本）

### 4.2 Identity

| 字段 | 说明 |
|------|------|
| `id` | 会话内唯一 |
| `label` | 展示名（邮箱、sub、userId、或「env-default」） |
| `scheme` | 绑定的 OpenAPI security scheme 名；可空 |
| `credential_headers` | 服务端私有，如 `Authorization: Bearer …` |
| `source` | `env` \| `login_capture` \| `manual` |
| `claims_summary` | 非敏感摘要（roles、sub、exp 等，能解析则填） |
| `is_default` | 歧义时优先 |
| `last_used_at` | Resolver「最近使用」兜底 |
| `created_at` / `updated_at` | 审计用 |

存储：本版 **进程内** `IdentityStore`（map[conversationID][]Identity）。不把完整凭证写入 `events.data_json`。

### 4.3 登录捕获配置（Connector）

示例（概念配置，落地到 yaml/RegisterOpts）：

```yaml
auth:
  bearer_env: BAIZE_CONNECTOR_TOKEN   # 启动兜底
  capture:
    tool_name_glob: "*login*"
    token_json_paths:
      - "accessToken"
      - "data.accessToken"
      - "data.token"
    header_template: "Bearer {{token}}"   # 或已含 Bearer 则原样
    default_scheme: ""                    # 空则尝试从 OpenAPI 唯一 http bearer scheme 推断
    label_json_paths:
      - "email"
      - "user.email"
      - "data.email"
```

规则：

- tool 成功且非 `is_error` 时尝试捕获  
- 解析到 token → upsert：同一 `scheme` + 同一 subject（claims.sub / label）则更新，否则新增  
- 新捕获的身份可自动 `is_default=true`（同 scheme 下取消其他默认）

---

## 5. Auth Resolver

### 5.1 接口

```text
Resolve(ctx, input) → (headers map[string]string, identityID string, ok bool)
```

`ResolveInput` 至少包含：

- conversation 下 identities  
- operation 的 security requirements（scheme 名列表；可空）  
- connector 默认 headers  
- 可选：强制 `identity_id`

### 5.2 默认：`OpenAPISecurityResolver`

顺序：

1. 若指定 `identity_id` 且存在 → 用之  
2. 若 operation 声明了 security scheme → 过滤 `scheme` 匹配且未过期（有 `exp` 则检查）的身份；多条：`is_default` → `last_used_at` 最新  
3. 否则 → `is_default` → 最近使用 → 若仅一条非 env 身份用之  
4. 仍无 → connector 默认 headers（env）；再无则空 headers（由下游 401）  
5. 选中后更新该身份 `last_used_at`

**401 自动换马重试：默认关闭。**

### 5.3 插拔

Connector 配置 `auth.resolver: openapi`（默认）。后续可注册 `path_prefix` 等实现同一接口，参数留在 connector 配置，不进引擎核心。

---

## 6. 与 Run / HITL / Invoke 的衔接

1. `POST /v0/runs` 解析 `conversation_id`（及可选 `identity_id`）写入 Run 元数据（新列或 `meta_json`）  
2. `Execute` / `ContinueFromHITL` 将 `conversation_id`（及强制 identity）放入 `context.Context`  
3. OpenAPI tool 闭包在 `Invoker.Invoke` 前调用 Resolver，得到本次 headers，**不得**写回全局 `Invoker.Headers`  
4. 登录类 tool 返回成功 → Capture → Identity Store  
5. HITL resume 使用同一 `conversation_id` 再 Resolve（若用户已退出该身份，调用应失败并可在消息中说明）

---

## 7. API 表面

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/v0/runs` | body 增加 `conversation_id`；可选 `identity_id` |
| GET | `/v0/conversations/{id}/identities` | 列表，**不含**完整 token |
| POST | `/v0/conversations/{id}/identities/{iid}/default` | 设默认 |
| DELETE | `/v0/conversations/{id}/identities/{iid}` | 退出 |
| DELETE | `/v0/conversations/{id}/identities` | 清空捕获身份（env 兜底是否清除见配置，默认保留 fallback） |

Identities 列表项示例：

```json
{
  "id": "idt_…",
  "label": "admin@miao.com",
  "scheme": "bearer",
  "source": "login_capture",
  "claims_summary": { "roles": ["admin"], "exp": 1786556892 },
  "is_default": true,
  "last_used_at": "…"
}
```

---

## 8. UI

- `localStorage` 保存当前 `conversation_id`；「新对话」换新 id 并刷新身份面板  
- 面板：账号列表、默认标记、退出；不展示 JWT  
- 发送 run 始终带 `conversation_id`  
- 可选（本版可做最小）：本条强制 `identity_id`  
- tool.result 若含 token，UI/事件展示层脱敏（截断或 `[redacted]`）

---

## 9. 安全

- 完整凭证仅存 Identity Store；禁止写入 events 明文  
- Capture 路径可配，避免误捕获  
- 列表 API 与 UI 仅摘要  
- 同进程多对话靠 `conversation_id` 隔离；本版不实现跨用户鉴权网关（假设本地/受信部署）

---

## 10. 测试计划

| 用例 | 期望 |
|------|------|
| 登录捕获后同会话第二次 Run | 受保护 GET 带上捕获 Bearer |
| 两套 scheme 各一身份 | 不同 operation 选中对应 identity |
| DELETE identity 后调用 | 不再带该 token；可回退 env 或 401 |
| `identity_id` 强制 | 忽略默认，使用指定身份 |
| list identities | 无完整 JWT 字符串 |
| 无会话捕获、仅 env | 与改造前行为一致 |
| HITL 批准登录 | 捕获发生在批准执行之后 |

---

## 11. 迁移与兼容

- 现有只配 `bearer_env` 的部署：不配 capture 也能跑；自动有一条 `source=env` 逻辑兜底（可不出现在 UI，或标记为「服务默认」）  
- 旧客户端不传 `conversation_id`：服务端生成并在 `POST /v0/runs` 响应中返回 `conversation_id`，避免硬断  
- Demo / README 增补：会话身份与多账号说明（短文即可）

---

## 12. 开放决策（已锁定）

| 决策 | 选择 |
|------|------|
| 总方案 | 会话身份库 + 可插拔 Resolver（方案 3） |
| 默认自动选用 | OpenAPI security scheme 绑定 |
| 多账号 UX | 同时记住；可查看；退出；设默认 |
| 企业差异 | 自定义 Resolver，不改 Runtime 核心 |
| 静态 env | 仅兜底，非唯一身份来源 |
