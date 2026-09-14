# UI-LOGIN-AT：登录入口 `@`/`/` 直达与 `login_required` 补救

> 状态：实现完成（待合入）
> 日期：2026-09-14
> 史诗：UI-LOGIN-AT（开源 v1 确认清单）
> 前置：会话身份库与登录捕获（`2026-08-15-session-login-tool-gate`）、HTTP 插件 capture、P3-B 连接器页（后继项上调）
> 备注：本批只加直达路径，不替换「模型在对话里帮用户登录并 capture」的既有流程

---

## 1. 背景与成功标准

P3-B 将「对话中 `@` 一下即可登录某系统」列为独立后继项：从连接器 capture 推导登录入口、在提及弹窗作为特殊条目、绕过模型确定性调用登录工具，并在 `login_required` 工具卡片提供「去登录」。

### 1.1 成功标准（本批同等交付）

1. **主动登录：** 用户在 Chat 输入 `@` 或 `/` 时，弹窗出现由 capture 推导的登录入口；选中后确定性调用对应登录工具（不经模型选工具）。
2. **被动补救：** `require_login` 工具返回 `login_required` 时，卡片提供「去登录」，打开同款选择器（按连接器过滤），选中后走同一强制调用路径。
3. **兼容：** 用户仍可忽略直达，在聊天里提供账号密码，由模型调用登录工具并 capture；行为与本批上线前一致。

### 1.2 产品决策摘要

| 议题 | 决策 |
|------|------|
| 交付范围 | 主动 `@`/`/` + 被动「去登录」同等交付 |
| 连接器类型 | 仅 `openapi` 与 `http`（HTTP 插件）；MCP 不做 |
| 多工具匹配 | capture glob 匹配到的每个启用工具各一条入口 |
| 触发符 | `@` 与 `/` 均可（与现有技能提及一致） |
| 已有身份 | 入口仍显示，标注「已登录」，选中仍可再调 |
| 「去登录」多入口 | 打开同款选择器，仅列出该连接器条目 |
| 选中后行为 | **立即**强制调用；有必填参数则先出简易表单 |
| 实现骨架 | 服务端登录目录 + `login-invoke` 强制工具回合 |
| 身份作用域 | **保持会话级**（当前 `conversation_id`）；不做跨会话通用 |
| 跨会话免再登 / 配置档 | **非本批**；后继可立 `CONV-INHERIT-ID`、`WORKSPACE-PROFILES` |

---

## 2. 范围

### 做

- 后端：按连接器 capture 实时推导登录入口；会话级 `login-entries` 与 `login-invoke` API。
- 强制工具回合：跳过模型选工具，直接 `Registry.Invoke`；成功后走既有 capture → 身份 Upsert。
- Chat：`@`/`/` 弹窗混入登录区；必填参数表单；`login_required` 卡片「去登录」。
- 敏感参数：表单字段进入 invoke `arguments`，**不**写入用户消息气泡明文。

### 不做

- MCP 登录入口。
- 渠道（如微信适配器）扫码登录。
- 修改 capture 配置模型或 Tools 页 capture 表单本身。
- 跨会话 / 全局身份池；新建对话自动继承身份；工作区/配置档目录。
- 用直达路径替换或禁用模型帮登流程。

---

## 3. 架构

```
Chat WebUI
  @/ 弹窗 ──┬── 技能（现有）
            └── 登录入口（本批）
  login_required 卡片 →「去登录」→ 同款选择器（可按 connector_id 过滤）
  有必填 → 简易表单 → login-invoke
        │
        ▼
GET  /v0/conversations/{id}/login-entries
POST /v0/conversations/{id}/login-invoke
        │
        ├── Store：仅 type ∈ {openapi, http}
        ├── capture glob → 匹配已启用工具
        ├── Identities + auth resolve → logged_in
        └── 强制工具回合 → Registry.Invoke → capture Upsert
              │
              ▼
对话 run 事件流（与现有 SSE/轮询一致）
设置 → 账号页（同 conversation_id 可见）
```

身份仍按会话存储：登录成功写入当前对话；「账号」页读取浏览器 `baize.conversation_id` 对应会话，与 Chat 一致。新建另一对话不会自动带上身份（本批预期）。

---

## 4. API 契约

### 4.1 `GET /v0/conversations/{id}/login-entries`

可选查询参数：`connector_id`（「去登录」补救时必带，只返回该连接器条目）。

响应示例：

```json
{
  "entries": [
    {
      "id": "crm/login",
      "connector_id": "crm",
      "connector_title": "CRM",
      "connector_type": "openapi",
      "tool_name": "login",
      "title": "登录 · CRM / login",
      "logged_in": true,
      "parameters": {},
      "required": ["username", "password"]
    }
  ]
}
```

字段约定：

- `id`：稳定键，`{connector_id}/{tool_name}`。
- `connector_title`：展示用；无标题时可用 `connector_id`。
- `connector_type`：仅 `openapi` 或 `http`。
- `parameters`：工具参数 JSON Schema（来自工具定义；无参可为 `{}`）。
- `required`：必填参数名列表（可由 schema 推导）。
- `logged_in`：当前会话下，该连接器 auth resolve **会成功**（与 `require_login` 门闸同一谓词）。身份库仍是会话级、无 `connector_id` 字段；规格承认这是尽力标注，不是 per-connector 身份模型。

推导规则：

1. 列出 Store 中 `type` 为 `openapi` 或 `http` 的连接器（`mcp` 等跳过）。
2. 若请求带 `connector_id`，只处理该连接器；连接器不存在或类型不符时仍返回 `200` 与 `entries: []`（避免前端为 404 分支）。
3. 对该连接器每个**已启用**工具：若名称匹配该连接器生效 capture 的 `tool_name_glob`（含缺省 `CaptureDefaults`），则生成一条 entry。
4. 排序：先 `connector_id`，再 `tool_name`（稳定、可测）。

鉴权：与同会话 `identities` 路由同一控制面角色（运营可读本会话登录目录）。

### 4.2 `POST /v0/conversations/{id}/login-invoke`

请求体：

```json
{
  "connector_id": "crm",
  "tool_name": "login",
  "arguments": { "username": "…", "password": "…" }
}
```

行为：

1. 校验连接器类型 ∈ {`openapi`,`http`}；工具已启用且匹配该连接器 capture；否则 `400`。
2. 按工具 schema 校验必填参数；缺参 `400`。
3. 若该会话已有进行中 Run：`409 conversation_busy`（与现有忙碌语义对齐）。
4. 创建绑定该 `conversation_id` 的强制工具回合：**不**调用模型做 tool 选择；直接 `Registry.Invoke`。
5. 成功响应至少包含 `run_id`；工具结果经既有 events/stream 推送。HTTP 在工具业务失败时仍可返回 `200` + `run_id`，由 Chat 展示错误卡片（与普通 run 一致）。
6. capture：与模型调用登录工具相同——成功且内容可解析则 Upsert `login_capture` 身份。
7. 若该工具配置了 `require_approval`：不绕过 HITL，进入既有审批等待。
8. 登录工具不得被自身 `require_login` 门闸挡住（与现网「先 login」一致）。`login-invoke` 只允许调用「出现在该会话 login-entries 中的工具」；若不在目录中（含被误标 require_login 且无法作为登录入口推导出的工具），返回 `400` `not_a_login_entry`，禁止客户端盲重试死循环。

---

## 5. Chat UI

### 5.1 `@` / `/` 弹窗

- 触发规则与现有 `skillMention` 一致（行首或空白后的 `@`/`/`）。
- 弹窗分区：**登录**在上，**技能**在下。查询字符串同时过滤两区。
- 登录条目来自 `GET login-entries`（当前会话 id）；展示 `title`，`logged_in` 时附加「已登录」。
- 选中登录条目：
  - `required` 为空 → 立即 `POST login-invoke`（`arguments` 可为 `{}`）。
  - `required` 非空 → 打开简易参数表单（按 schema/required 渲染）；提交后再 invoke。
- 选中技能：保持现有「写入 `@id` + 空格」行为，不变。

### 5.2 `login_required` 卡片

- 识别工具结果 `code === "login_required"`（既有 `tool.LoginRequiredContent()`）。
- 展示人话说明 +「去登录」按钮。
- 点击：打开与弹窗同款的登录选择器，请求带该工具所属 `connector_id`；其后与 §5.1 选中路径相同。
- 用户可忽略按钮，继续在输入框发账号密码走模型帮登。

### 5.3 对话呈现与敏感信息

- 发起直达登录时，对话中展示可见的发起/工具行（例如「已发起登录 · CRM/login」+ 工具结果），**不**把密码等表单字段写入用户消息气泡。
- 账号页：同会话 capture 成功后刷新可见（既有 `listIdentities`）；本批不改账号页 IA。

---

## 6. 错误处理

| 情况 | 行为 |
|------|------|
| 类型/capture/未启用不匹配 | `400` |
| 缺必填参数 | `400`（前端应先拦；API 再校验） |
| 会话 Run 忙碌 | `409 conversation_busy` |
| 工具执行失败 | Run 内 `is_error` 结果；Chat 错误卡片 |
| 需审批 | 既有 HITL；直达不绕过 |

---

## 7. 测试与验收

### 后端

- entries：openapi/http 匹配出条；mcp 不出；`connector_id` 过滤；多工具多条；禁用工具不出；排序稳定。
- `logged_in` 与 require_login resolve 谓词一致。
- invoke：不经模型；capture 后 identities 可见；busy → 409；非法请求 → 400。
- 回归：模型路径 login → capture → require_login 工具仍通。
- `require_approval` 登录工具：直达仍进入 HITL。

### 前端

- `@` 与 `/` 均出现登录区；「已登录」标注；无参立即 invoke；有参出表单。
- `login_required` →「去登录」→ 过滤选择器。
- 密码不进用户消息气泡。

### 人工走查

- 直达登录后，设置 → 账号（同会话）出现登录捕获身份。
- 新建另一对话：无身份，需再登（本批预期）。
- 忽略「去登录」，聊天给账号密码由模型帮登仍成功。

---

## 8. 后继项（非本批）

- **CONV-INHERIT-ID：** 新建对话继承当前会话身份、不继承消息（缓解长上下文换对话的再登成本）。
- **WORKSPACE-PROFILES：** 配置档/分组 + 新建目录，隔离多账号并共享组内登录态。
- MCP 对等登录入口（若 MCP 具备 capture / require_login 对等模型）。

---

## 9. 文档与账本

- 实现计划落地后更新 `docs/superpowers/notes/2026-09-13-spec-ledger.md` 与 v1 确认清单中 UI-LOGIN-AT 状态。
- P3-B 规格 §后继项可加一句「见 `2026-09-14-ui-login-at-design.md`」。
