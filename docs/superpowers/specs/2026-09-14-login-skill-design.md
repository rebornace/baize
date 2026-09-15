# LOGIN-SKILL：连接器自动维护的登录 Skill（取代登录直达）

> 状态：**已交付**（2026-09-14；已合入 `main`）
> 日期：2026-09-14
> 史诗：LOGIN-SKILL（承接并收敛 UI-LOGIN-AT）
> 前置：会话登录捕获与 `MatchToolName`（含大小写无关）、Agent Skills / Catalog、`activate_skill`、线性 `workflow.yaml`（本批不自动生成）
> 备注：开源开发阶段不保留双轨；产品入口仅为 Skill，可读可控；UI-LOGIN-AT 直达已拆除

---

## 1. 背景与动机

UI-LOGIN-AT 将登录做成「目录 + `login-invoke` 强制调工具」，适合单次填表，但：

- 与开源常见的多步验证、OAuth 引导不匹配；
- 登录能力落在「接口直达」维度，而非已有的 Skill 维度（`@`/`/`、SKILL.md、可选 workflow、设置可管）；
- 与「自动把登录相关工具生成 skill」的产品意图不一致，若再叠声明式 flow 会形成双轨债务。

**本史诗：** 连接器变更时自动生成/刷新 **managed 登录 Skill**；Chat 只通过技能使用登录能力；拆除登录专用区与对外 `login-invoke`。

### 1.1 成功标准

1. 对每个有「登录相关启用工具」的 `openapi`/`http` 连接器，存在 id 为 `login-<connector_id>` 的 managed skill，并出现在 `@`/`/` 技能列表。
2. Skill 的 `tools:` 覆盖 capture 命中工具 ∪ 通用 companion 名模式；无默认 `workflow.yaml`；多步/OAuth 靠 SKILL 说明 + ReAct（或管理员事后手补 workflow）。
3. `login_required` 的「去登录」跳到对应 login skill（写入 `@login-<id>` 或等价带 skills 开跑），不再打开工具选择器 / 强制 invoke。
4. 对外不再提供 `login-entries` / `login-invoke`；Composer 无登录专用区。
5. 模型帮登 + capture 行为保持可用。

### 1.2 产品决策摘要

| 议题 | 决策 |
|------|------|
| 入口维度 | 仅 Skill |
| 生成策略 | 连接器变更全自动同步 |
| 覆盖策略 | managed 包整包覆盖；定制请 fork 为非 managed |
| 标记 | 厂商中立：`managed: true`、`managed_kind: connector_login`；文案「由连接器自动维护」，不出现产品名 |
| tools | capture ∪ companion 模式；不自动 workflow |
| 与 UI-LOGIN-AT | 本史诗拆除直达，规格上取代之 |
| 落盘 | `<data>/skills/managed/login-<id>/` + Catalog 扫描 |

---

## 2. 范围

### 做

- Sync：按连接器生成/更新/删除 managed 登录 skill；启动全量扫描。
- Catalog 加载 managed 根目录；列表与 `@`/`/` 可见。
- `login_required`「去登录」→ 对应 login skill。
- 拆除 Chat 登录直达 UI 与对外 login-entries/login-invoke（及仅为其服务的 ForcedTool 对外路径）。
- 将 capture/companion 工具枚举逻辑收为 Sync 所用内部实现（可自 `loginentry` 迁移/改名）。
- 更新账本：UI-LOGIN-AT 直达收敛为 LOGIN-SKILL。

### 不做

- 自动生成 `workflow.yaml` 或声明式 `login_flows` 配置 UI（可后继）。
- OAuth 浏览器跳转控件、滑块、渠道扫码专用 UI。
- MCP 连接器自动登录 skill。
- 跨会话身份继承（仍为独立后继）。
- 在标记或路径中绑定具体产品名（如「baize 生成」）。

---

## 3. 架构

```
连接器 PUT / 删除 / 工具目录变更 / 进程启动
        │
        ▼
SyncManagedLoginSkills(store, managedRoot)
  - type ∈ {openapi, http}
  - tools = Enabled ∩ (captureMatch ∪ companionMatch)
  - 无工具 → 删除已有 managed 包
  - 写 skills/managed/login-<id>/SKILL.md（整包覆盖）
  - 若存在同名且 managed≠true 的用户包 → 跳过并日志
        │
        ▼
Catalog.Reload（含 managed 根）
        │
        ▼
Chat @/ 技能列表含 login-*
login_required「去登录」→ @login-<connector_id>
```

会话 capture、模型调用登录工具的路径不变。

---

## 4. Managed Skill 包格式

### 4.1 路径与 id

- Skill id：`login-<connector_id>`。
- `connector_id` 规范化：仅保留 `[a-zA-Z0-9_-]`；若规范化后为空则跳过该连接器并日志。
- 目录：`{SkillsManagedRoot}/login-<id>/SKILL.md`。
- `SkillsManagedRoot`：默认 `{DataDir}/skills/managed`（与现有用户技能根并列或为其约定子树；实现与 bootstrap 配置对齐，规格要求 Catalog **额外**加载该根）。

### 4.2 Frontmatter

必填字段：

| 字段 | 值 |
|------|-----|
| `name` | 与 id 相同，`login-<connector_id>` |
| `description` | 人话，含连接器 id；注明由连接器自动维护 |
| `tools` | 字符串列表，见 §5 |
| `managed` | `true` |
| `managed_kind` | `connector_login` |
| `managed_connector_id` | 原连接器 id |

禁止在生成文案中写死某一发行版/产品品牌名。

### 4.3 正文模板（每次覆盖）

生成结构化短文，至少包含：

1. 如何用 `@login-<id>` 或设置激活本技能；
2. 列出可用工具名，说明可能需多步（发码、校验、OAuth 回调信息等），按工具结果继续；
3. 登录成功后凭证留在**当前会话**；
4. 不要把密码写进无关用户气泡的最佳实践一句（可选）。

不生成 `workflow.yaml`。管理员可在 fork 的非 managed 副本中自行添加。

### 4.4 覆盖与冲突

- 目标目录已存在且 frontmatter `managed: true` 且 `managed_kind: connector_login` → **整包覆盖**（删除目录后重写或等价原子替换）。
- 目标 id 已被 **非 managed** 包占用（用户技能根或其它根）→ **不覆盖**，打警告日志；不改用户包。
- 连接器删除或 tools 并集为空 → 删除对应 managed 目录（仅当其仍为 managed connector_login）。

---

## 5. tools 并集规则

对连接器 `ListToolsByConnector`，工具必须 `Enabled`，且满足其一：

### 5.1 Capture 命中

`MatchToolName(effectiveCaptureGlob, tool.Name)`  
`effectiveCaptureGlob` = `CaptureDefaults` 作用于该连接器 auth.capture（与现网一致）。  
`MatchToolName` 保持大小写无关（已落地）。

### 5.2 Companion 名模式（首版固定表）

大小写无关，使用与 `MatchToolName` 相同的 glob 语义。命中任一即纳入：

| 模式 | 说明 |
|------|------|
| `*sms*` | 短信发码等 |
| `*otp*` | 一次性密码 |
| `*verify*` | 校验类 |
| `*oauth*` | OAuth 相关 |
| `*token*` | 换票/刷新等 |
| `*auth*` | 广义鉴权（可能偏宽，靠启用集约束） |

**排除：** 名称匹配 `*logout*` 的工具不纳入（即使命中 `*auth*` 等——实现时先排除 logout，再做 companion/capture 判断）。

后继可将该表做成配置；本批不要求配置 UI。

### 5.3 空集

并集为空 → 不创建包；删除已有 managed 包（§4.4）。

---

## 6. 同步时机

1. 进程启动、Catalog 首次加载前或后：对 Store 全部 openapi/http 执行 Sync。
2. 连接器成功 Upsert / Delete 之后。
3. 连接器工具目录 Replace / 单工具 PATCH 导致 Enabled 或集合变化之后。

Sync 成功后 `Catalog.Reload`（或增量替换 Package）。失败打日志，不导致连接器 PUT 整体失败（连接器写入已成功；技能滞后可下一轮启动修复）。若实现选择「Sync 失败则 PUT 返回 5xx」须在计划中写明并测死——**默认推荐 Sync 失败不回滚连接器**。

---

## 7. Chat 与「去登录」

### 7.1 拆除直达

删除或停止注册：

- 前端：Composer 登录分区、`onPickLogin` 直达、`LoginParamsModal` 作为 login-invoke 入口、`LoginPicker`、相关 api 封装与文案。
- API：`GET /v0/conversations/{id}/login-entries`、`POST /v0/conversations/{id}/login-invoke` 及 ACL 条目。
- 仅被 login-invoke 使用的 `KindForcedTool` / `ExecuteForcedTool` 对外暴露：若无其它调用方则删除；若保留引擎能力须无 HTTP 入口。

### 7.2 「去登录」

- 仍识别 `code === "login_required"`。
- 按钮：解析工具的 `connector_id` → skill id `login-<id>`。
- 若 Catalog 存在该 skill：将 `@login-<id> ` 写入 Composer（清除不当 mention 状态），聚焦输入框；**不**自动发送（用户可补一句再发，或产品选择自动发送——**本规格定为不自动发送**，减少误跑）。
- 若无 skill：toast/文案「暂无可用的登录技能（请确认连接器已启用登录相关工具）」。

### 7.3 技能使用

激活后工具可见性遵循现有 Skill 规则（`tools:` 求交启用目录）。登录成功仍走既有 capture。

---

## 8. 测试与验收

### 后端

- 含 PascalCase `*Login*` 与 `*sms*` 工具的 openapi 连接器 → managed 包 tools 含二者。
- 停用全部相关工具 → 包删除。
- 删除连接器 → 包删除。
- 非 managed 占位同名 → 不覆盖。
- mcp → 不生成。
- 启动扫描补齐。
- 注销后 login-entries/login-invoke 不可用。
- 回归：对话 login → capture → require_login 工具。

### 前端

- `@`/`/` 可见 `login-*`。
- 「去登录」写入 `@login-<id>`；无 skill 有反馈。
- 无登录专用弹窗区。

### 人工

- 验证码：激活 skill 后多轮调发码+登录并 capture。
- 忽略「去登录」、纯模型帮登仍可用。

---

## 9. 文档与账本

- 本规格取代 UI-LOGIN-AT 作为登录入口产品方案；`2026-09-14-ui-login-at-design.md` 文首改为「已由 LOGIN-SKILL 取代（直达拆除）」并链到本文。
- 更新 `spec-ledger` / v1 确认清单：UI-LOGIN-AT 收敛项 + LOGIN-SKILL 状态。
- README 一句：连接器可自动维护登录类 Skill；多步/OAuth 用技能说明 + ReAct。

---

## 10. 后继（非本批）

- 可选 companion 模式配置化。
- 可选 `login_flows` → 生成 `workflow.yaml`。
- OAuth 浏览器/设备码专用 UX。
- CONV-INHERIT-ID / WORKSPACE-PROFILES。
