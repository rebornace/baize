# Baize 设计规格：Tools 设置页可读目录

> 状态：待审查  
> 日期：2026-08-17  
> 前置：Connector 工具目录已落地（`2026-08-17-tool-catalog-design.md`）  
> 依据：本轮头脑风暴（方案 1：Connector → 路径前缀树 + 显示名/说明 overlay；页顶添加抽屉）

本规格修订工具目录规格 §7 的 GUI：设置 → Tools 从扁平长列表改为可浏览的树，并让人能读懂、改得动扫出来的文案。不修改会话身份、三种 Connector `auth.mode`、HITL 烘焙、控制面口令、启停与 extra 的目录语义。

---

## 0. 动机

商业 OpenAPI 往往一个 Connector 就有上百个 operation。当前设置页是一条扁平列表，底部再贴手加表单：既扫不清全貌，也找不到单个工具。更糟的是行上只画 `name · METHOD path`。即便 spec 带了 `summary` / `description`，设置页也不展示；spec 没写人话时，服务端自己也记不得每个接口做什么。

管理员需要：按 Connector 和路径前缀看目录、搜索找单个、按组启停、给工具起人看得懂的显示名和说明。说明同时给模型用。换 spec 不得冲掉人改过的文案。

---

## 1. 目标与成功标准

**目标：** 管理员在 `/ui/settings/tools` 能看清、找得到、改得懂工具目录；人改的显示名和说明在换 spec、重启后仍在。

**成功标准：**

1. 工具按 Connector 分组，组内再按路径前缀分组；默认折叠。仅一个 Connector 时自动展开该 Connector
2. 搜索过滤 `title`、`name`、`path`、`description`、HTTP 方法（不区分大小写）；有关键字时只显示命中行并展开祖先
3. 行主标签为 `title`，空则 `name`；展示 `METHOD path` 与截断说明（来自 spec 或人改）
4. 可 PATCH `title` 与 `description`；`name`、方法、路径、`input_schema` 不可改
5. `description_custom=false` 时换 spec 更新说明；`true` 时保留人改的说明（含空字符串）；`title` 永不被 spec 覆盖
6. 启用行改说明后，模型下一轮 tool list 看到新说明；只改 `title` 不改 Registry
7. 页顶「添加」打开抽屉；成功后关闭抽屉；列表不再常驻底部表单
8. 组头「全部启用 / 全部停用」对该组每个工具调用现有逐条 `PATCH enabled`；部分失败时已成功的行保持新状态，页顶汇总失败
9. extra 仍可删；`spec` / `plugin` 不可删；操作员仍进不了本页
10. 开箱 mock-ticket 集成测试仍通过

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| Store | `Tool.Title`、`Tool.DescriptionCustom`；SQLite 加列并迁移旧库 |
| 合并 | `MergeCatalog` 按 §5 保留 `title` 与人改说明 |
| API | `GET` 回显新字段；`PATCH` 增加可选 `title`、`description`；`POST` extra 可选 `title` |
| Registry | 启用行改说明时原地更新描述（见 §6.2） |
| GUI | 树、搜索、行内编辑文案、页顶添加抽屉、组头逐条启停 |
| 测试 | 合并、PATCH、SQLite 缺列、路径前缀与搜索纯函数 |
| 文档 | README 中英设置 → Tools 行为与现实现一致 |

### 不做

- 用模型给工具生成摘要或显示名
- 新的批量目录 HTTP 接口（组启停不是新 API）
- 改 `name`、方法、路径、`input_schema`、`require_approval`
- 可视化 Schema 编辑器、Connector 可视化编辑器
- Agent Skills、MCP、插件设置页
- 恢复为 spec 原文按钮、并发编辑冲突合并
- 组头「需要登录」总开关

---

## 3. 产品行为

### 3.1 树

仍在 `/ui/settings/tools`，不新开路由。仅管理员。

- **一级：** Connector。组头：id、工具数、已启用数；「全部启用 / 全部停用」
- **二级：** 路径前缀。组头同样可整组启停
- 两级默认折叠。进程里只有一个 Connector 时，展开该 Connector（开箱四条工具不必先点）
- 组头没有「需要登录」总开关；登录仍按行

路径前缀：把 `path` 按 `/` 切开，丢掉空段，再跳过大小写不敏感的 `api` 以及匹配 `^v\d+(\.\d+)*$` 的版本段（如 `v1`、`v2`、`v1.0`），取下一个段，显示为 `/tickets`。没有剩余段、或没有 path（HTTP 插件）→ 分组名 **「其他」**。同 Connector 内前缀按字典序，「其他」永远在最后。

组头启停：对该组当前可见工具（受搜索过滤）逐个调用已有 `PATCH /v0/tools/{name}`，可并行。没有新接口，没有整组事务。

### 3.2 搜索

页顶输入框。匹配 `title`、`name`、`path`、`description`、方法；不区分大小写。有关键字：只保留命中行并展开其 Connector 与前缀。无命中：树区「无匹配」，不是错误。清空：回到 §3.1 默认折叠。

### 3.3 行

- 主标签：`title`，空则 `name`
- 次行：`METHOD path`（无 path 则省略 path）
- 说明一行截断；点编辑可看全文
- 开关：启用、需要登录；`extra` 可删；「需审批」徽章只读
- **编辑文案：** 行内展开 `title` 与 `description` 输入框，保存 PATCH。不另开抽屉。方法、路径、schema 只读展示

### 3.4 添加

页顶「添加」打开右侧抽屉，列表保持干净。字段：Connector（多个 OpenAPI Connector 时下拉）、名称、方法、路径、显示名（可选）、说明、`input_schema`（JSON，空则 `{}`）。提交现有 `POST /v0/connectors/{id}/tools`。仅 plugin 工具、没有 OpenAPI Connector 时不显示「添加」（与今天隐藏表单相同）。

成功：关抽屉、清空表单、树中出现新行。失败：抽屉不关。

---

## 4. 数据模型

`store.Tool` 增加：

| 字段 | JSON | 含义 |
|------|------|------|
| `Title` | `title` | 显示名。只给人看。空则 GUI 显示 `name`。永不进入模型 tool list |
| `DescriptionCustom` | `description_custom` | 人是否改过 `description`。默认 `false` |

`name` 仍是调用主键，PATCH 不可改。`description` 仍人和模型共用。

SQLite `tools` 表：`CREATE TABLE` 含 `title TEXT`、`description_custom INTEGER`。已有库用与 `migrateRunsColumns` 相同的 `ALTER TABLE ... ADD COLUMN`，重复列错误忽略。缺列读入时 `title=""`、`description_custom=false`。

---

## 5. 合并

`MergeCatalog` 对再次发现的 `spec` / `plugin` 行：

| 字段 | 规则 |
|------|------|
| `title` | 始终保留已有值；新发现行空 |
| `description` | 已有 `description_custom=true` → 保留已有说明（含空）；否则用本次发现的 spec/插件说明 |
| `description_custom` | 保留已有；新行 `false` |
| `method` / `path` / `operation_id` / `source` | 跟发现结果 |
| `enabled` / `require_login` | 与今天相同（含 YAML 名单改写） |

extra 仍整行原样保留（含 `title` 与 `description_custom`）。

第一次发现：`description` 来自 OpenAPI `description`，否则 `summary`（现有 `LoadTools`）；`title` 空；`description_custom=false`。

人把说明改回与 spec 相同的字，仍算 custom，之后换 spec 不再跟 spec。本规格不提供「恢复为 spec」。

---

## 6. API

ACL、404、extra 删除、插件上 POST 400：不变。

### 6.1 `GET /v0/tools`

每行多返回 `title`、`description_custom`。`description` 仍返回。

### 6.2 `PATCH /v0/tools/{name}`

JSON 可选字段：`enabled`、`require_login`、`title`、`description`。至少出现其中一个，否则 400 `invalid_request`。`title` 与 `description` 用指针：省略表示不动；显式 `""` 表示清空。

- 写入 `title`：只改 Store，不改 Registry
- 写入 `description`：Store 更新说明且 `description_custom=true`。若该行 `enabled`：在 Registry **原地更新** 该工具的 `ToolSpec.Description`（与 `SetRequireLogin` 同模式，避免整工具重注册碰到 mutating HITL）。若该名尚未注册，再走现有 `registerOne`
- `enabled` / `require_login` 路径保持今天的 Unregister / `registerOne` / `SetRequireLogin` 分流，不得为了改文案而改这条分流

### 6.3 `POST /v0/connectors/{id}/tools`

可选 `title`。手加行的说明本就是人写的，故 `description_custom` 恒为 `true`。其余与今天相同。

---

## 7. GUI 实现要点

- 路径前缀分组与搜索过滤做成纯函数（与 `canDeleteCatalogTool` 同文件或同级），供页面与单测共用
- 组启停进行中禁用该组按钮，避免连点；全部请求结束后用 §8 汇总
- 单行 PATCH 失败：该行控件回到请求前的值；编辑区不关
- 不改聊天主区、账号页、MCP/插件空页

本规格取代目录规格 §7 中「扁平列表 + 页内常驻添加表单」的 GUI 描述；目录规格的 Store 启停、extra、ACL 仍有效。

---

## 8. 错误

| 情况 | 表现 |
|------|------|
| `GET /v0/tools` 失败 | 页顶错误，不画树 |
| 单行启停 / 登录 / 保存文案失败 | 页顶写出工具名和原因；该行回到请求前 |
| 组启停部分失败 | 页顶「已更新 k/n」，并列出失败工具名与原因（超过 5 条只列前 5 条并注明其余省略）；成功行保持新状态，不整组回滚 |
| 添加 400 / 409 / 非法 JSON schema | 错误在抽屉内；不关闭、不清空 |
| 搜索无命中 | 「无匹配」 |
| 换 spec 与正在编辑并发 | 后写赢；不做人侧三路合并 |
| PATCH 四个可选字段都缺 | 400 `invalid_request` |

操作员 403、门开着 401：与今天相同。

---

## 9. 测试

Go：

- `MergeCatalog`：`description_custom=true` 时说明和 `title` 不被发现结果覆盖；`false` 时说明跟 spec、`title` 仍保留；新行 custom 为 false
- PATCH `title`：GET 可见；启用行 Registry 描述不变
- PATCH `description`：GET 与启用行 Registry 描述更新；`description_custom=true`；随后 Apply 换 spec 说明仍为人改文案
- SQLite 旧库无新列：Open 后可读写，缺列当空 / false
- 现有启停、mutating HITL、omit `require_login`、extra、开箱集成不得回退

前端：路径前缀（`/api/v1/tickets/{id}` → `/tickets`、`/tickets` → `/tickets`、无 path → 「其他」）与搜索命中 `title` 的纯函数单测。不测点击、抽屉、组头网络。

---

## 10. 文档

- `README.md` / `README.zh-CN.md`：设置 → Tools 为按 Connector / 路径前缀折叠的目录；可搜索；可改显示名和说明（换 spec 保留人改）；添加在抽屉。写清与下游登录、控制面口令不是一回事
- `docs/architecture-and-plugin-protocol.md`：目录行可有给人看的 `title`；`description` 可被人覆盖且不再被 spec 冲掉
- 不增加 YAML 必填字段

---

## 11. 实现时注意的类型与包

| 名称 | 作用 |
|------|------|
| `store.Tool.Title` / `DescriptionCustom` | overlay |
| `MergeCatalog` | §5 |
| `tool.Registry` 原地更新描述 | 启用行改说明 |
| SQLite `ALTER` 加列 | 旧库 |
| `web/chat` 分组/搜索纯函数 + `ToolsSettings` | 树、抽屉、行内文案 |

Git：不在 `main` 上改实现代码。规格与计划批准后再开功能分支。

---

*本文档经头脑风暴分节批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
