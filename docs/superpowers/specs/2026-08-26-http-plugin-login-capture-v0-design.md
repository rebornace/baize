# HTTP 插件登录捕获 v0 设计规格

> 状态：已批准（2026-08-26）  
> 日期：2026-08-26  
> 前置：会话身份与 OpenAPI 登录捕获、HTTP 插件会话 Resolver（无捕获）、Tools 设置页 capture 表单已落地  
> 依据：`2026-08-15-session-login-tool-gate` §8「插件登录捕获列为后续」；头脑风暴锁定「插拔内核更深 → 插件登录捕获 → 复用 Connector `capture` → invoke 闭包镜像 OpenAPI」  
> 分支建议：`feat/http-plugin-login-capture`

---

## 1. 目标与成功标准

**目标：** 无 OpenAPI、仅 HTTP 侧车的遗留系统，也能在对话里调一次登录工具，把凭证写入会话身份；同会话后续 `require_login` 的插件工具自动带上捕获头。配置复用现有 Connector `auth.capture`，不引入新概念、不做工作流产品。

**一句话成功标准：** 侧车 `*login*` 工具返回含 `accessToken` 的 JSON → Identities 出现 `login_capture` → 同会话需登录的插件工具下游收到捕获的 `Authorization`；完整 token 不进 events / GET run / SSE。

### 做

| 项 | 内容 |
|----|------|
| Apply | `type=http` **持久化并回显** `auth.capture`（废除「HTTP 清空 Capture」） |
| Runtime | `httpplugin.RegisterOpts` 增加 `Capture`；invoke 成功后按 OpenAPI 同序捕获 |
| 语义 | `conversation_id`、glob、`ExtractCredential`、`SourceLoginCapture`、`IsDefault`；`__none__` / 空 glob 不捕获 |
| HITL | 匹配 capture glob 的工具名，除非显式 `require_approval`，否则不当业务写操作强行审批 |
| 测试 | 替换 `TestApplyHTTPIgnoresCapture`；覆盖捕获 / `__none__` / 无会话 / `is_error` / 回归 |
| 示例 | `examples/http-plugin` 增加最小 `login` 工具 + README 一句配置 |
| 文档 | 架构草案、README 中英；旧规格交叉引用 |

### 不做

- 工作流 DSL / 画布 / Channel / TS·Python SDK / OTel  
- 引擎级统一捕获；MCP 登录捕获  
- 侧车协议新 annotations；专用「写身份」控制面 API  
- 改变 OpenAPI 捕获语义或 `require_login` 门闸规则  
- 插件专用设置页大改（复用 Tools 页已有 capture 表单）

### 成功标准（可测）

1. Apply/PUT HTTP + capture → `GET /v0/connectors/{id}` 回显 glob 与 paths  
2. 带 `conversation_id` 调匹配 glob 的 login → Identities 非空；再调 `require_login` 工具 → 下游 Authorization 为捕获值  
3. `__none__` / 无会话 / `is_error` 或缺 token 字段 → 不写入身份  
4. 凭证不出现在 Run 事件与 GET run  
5. OpenAPI 捕获与「插件仅消费已有身份」回归仍绿  

---

## 2. 运行时捕获语义

在 `httpplugin` 注册的 invoke 闭包中，侧车返回后、结果交回引擎前：

1. 仅当 `conversation_id` 非空、`Identities != nil`、工具结果 `!is_error`  
2. 且 `identity.MatchToolName(capture.ToolNameGlob, toolName)`  
3. 且 `identity.ExtractCredential(capture, content)` 成功  

则：

```text
Identities.Upsert(conv, Identity{
  Label, Subject, ClaimsSummary,
  Scheme: Capture.DefaultScheme,
  CredentialHeaders: { Authorization: ... },
  Source: SourceLoginCapture,
  IsDefault: true,
})
```

- 捕获失败或不匹配 → **静默**不写身份，工具结果原样返回（不把工具打成错误）。  
- 无 `conversation_id`（机器路径）→ 不捕获；仍可走 Connector 默认头 / passthrough。  
- `CaptureDefaults` / `__none__` 行为与 OpenAPI 路径一致。  
- 已有身份的 `Touch` 顺序：与 OpenAPI 相同（invoke 成功且用过身份则 Touch；捕获在其后）。

### 门闸顺序（不变）

`require_login`（无身份则 `login_required`）→（可选）HITL → 侧车 invoke →（成功则）捕获。

### HITL 特例

工具名匹配 `capture.tool_name_glob` 时：若**未**显式出现在 `require_approval` 列表，则 `needApproval = false`（对齐 OpenAPI：登录是鉴权引导，不是业务写）。插件无 HTTP method，不做 mutating 推断。

### 身份消费（已有，本里程碑不改）

会话内后续插件工具继续经 `Resolver` + `Identities` 选头；`require_login` 无可用头时返回既有 `login_required`。跨 Connector：身份是**会话级**，OpenAPI 捕获的身份仍可供插件使用，反之亦然。

---

## 3. Apply / RegisterOpts / 持久化

### 3.1 废除「HTTP 忽略 Capture」

`connector.Apply` 今日对非 `openapi` 将 `auth.Capture` 置空，并由 `TestApplyHTTPIgnoresCapture` 锁定。本里程碑改为：

| `type` | Capture 持久化 |
|--------|----------------|
| `openapi` | 经 `CaptureDefaults` 后写入 Store（不变） |
| `http` | **同样**经 `CaptureDefaults` 后写入 Store，GET 回显 |
| `mcp` 及其他 | 本里程碑仍清空 / 不启用捕获（保持现状） |

### 3.2 RegisterOpts

`httpplugin.RegisterOpts` 增加：

```go
Capture identity.CaptureConfig
```

Apply、bootstrap、PUT 热更新路径在注册 HTTP 插件时传入与 Store 一致的 Capture。`Auth` 回显形状已含 `capture` 字段时无需新 API。

### 3.3 配置缺省

省略 `capture` 时：与 OpenAPI 相同，`CaptureDefaults` 生效（通常 glob `*login*` 等开箱默认——以现有 `CaptureDefaults` 实现为准，本规格不另定一套默认值）。

显式 `tool_name_glob: "__none__"`：不捕获。

---

## 4. 错误与安全

| 情况 | 行为 |
|------|------|
| 结果无 token / 路径抽不到 | 不 Upsert；工具成功结果照常返回 |
| 需登录且无身份 | 现有 `login_required`（捕获发生在登录成功之后） |
| PUT/Apply capture 非法 | 沿用 OpenAPI 既有校验；不为 http 新增错误码 |
| 凭证泄漏 | events / GET run / SSE / 工具卡片不得含完整 token |

不新增控制面「登记身份」API；不把 Admin token 交给侧车。

---

## 5. 示例与文档

### 5.1 `examples/http-plugin`

增加最小 `login` 工具：成功返回 `content` 含 `accessToken`（可用固定测试 JWT 或占位串）与 `email`；README 注明 Connector `auth.capture` 示例（glob / `token_json_paths`）。

### 5.2 用户向文档

- `docs/architecture-and-plugin-protocol.md`：HTTP 插件 invoke 成功后可按 Connector `auth.capture` 写入会话身份。  
- `README.md` / `README.zh-CN.md`：删除或改写「仅 OpenAPI 支持登录捕获 / HTTP 插件不捕获」类表述。

### 5.3 过程文档交叉引用

在 `docs/superpowers/specs/2026-08-15-session-login-tool-gate-design.md` 文首或 §8 增加说明：插件侧「不捕获」边界已被本规格取代；勿再按旧边界实现。

---

## 6. 测试计划

| # | 用例 |
|---|------|
| 1 | Apply HTTP + Capture → Store/GET 保留字段（替换 `TestApplyHTTPIgnoresCapture`） |
| 2 | conv + 匹配 glob 的 login → Identities 非空；`require_login` 工具下游头为捕获值 |
| 3 | `tool_name_glob: "__none__"` → login 成功不写身份 |
| 4 | 无 `conversation_id` → 不捕获 |
| 5 | `is_error=true` 或缺 token → 不写入 |
| 6 | 返回内容/事件路径不含完整 token（与 OpenAPI 同档） |
| 7 | 回归：插件消费已有身份、OpenAPI 捕获仍通过 |

前端：Tools 设置页已有 capture 编辑；本里程碑以 API/Store 行为为准，不强制新 UI 单测。

---

## 7. 非目标回顾

本里程碑补齐 **HTTP 侧车登录捕获**，服务「插拔内核 / 无干净 OpenAPI 的遗留系统」。  
**不**做成扣子/Dify 式工作流；**不**要求无程序员的运营写 YAML——运营仍只 Chat；capture 由实施方在 Connector 上配置（与今日 OpenAPI 相同）。

---

## 8. 实现提示（非绑定）

- 优先从 `openapi/register.go` 抽出共享「成功后捕获」小函数，供 openapi 与 httpplugin 调用，避免两处漂移。  
- 改 `TestApplyHTTPIgnoresCapture` 为「保留并捕获」正向用例；保留或新增「MCP 仍忽略 Capture」若今日有对称断言。  
- 分支：`feat/http-plugin-login-capture`。

---

*本文档经头脑风暴 §1–§3 批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
