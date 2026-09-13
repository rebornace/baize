# 多模型配置与对话选模型（X2）v0 设计规格

> 状态：已交付（2026-09-13 账本对齐）
> 日期：2026-08-30
> 前置：单一 `llm.Provider` 注入（bootstrap `newLLM`）、会话/Run 持久化、控制面 Gate、设置页落盘先例（P4 存储）
> 依据：头脑风暴——按任务选模型节约成本；多模型 profile 存库、每条消息可选、热切换不重启
> 路线：本里程碑（X2）→（排队）X4 上下文压缩（窗口+滚动摘要）/ X3 中间件多源 / F 生产硬化

---

## 1. 目标与成功标准

**目标：** 管理员在设置页维护多个**命名模型 profile**（存数据库，UI 增删改），其中一个标记为默认。聊天界面在**每条消息发送前**可下拉选择模型，选择仅作用于即将发送的这条消息（不持久化为会话绑定）。模型切换**热生效、不重启进程**。API Key 原文存库（与 DSN 密码同级），管理接口回显脱敏。无人值守入口（渠道 / Inbox / MCP 导出）不传模型时落默认 profile。

**成功标准：**

1. 设置页可新建/编辑/删除多个模型 profile，可设一个默认；profile 持久化到 sqlite/postgres/memory，重启保留。
2. 聊天框下拉列出可选模型；发送消息携带所选 profile；该 run 实际使用对应模型。
3. 未选 / profile 已删 / 渠道等无人值守入口 → 使用默认 profile。
4. 编辑 profile（改 base_url/model/key/flags）后，后续对话立即用新配置，**无需重启**。
5. API Key 原文不通过任何 GET/list 接口返回（脱敏回显）；PATCH 提交脱敏占位或留空不改原 Key。
6. 默认 profile 不可删除；删除非默认 profile 不阻断引用它的历史 run（回退默认）。
7. 首次启动且库中无 profile 时，用 YAML `llm` 段种子出一个默认 profile（Key 走 `api_key_env` 环境变量，不入库）。
8. operator 可只读模型列表（供聊天下拉）；增删改/设默认仅 admin。

**明确不做：**

| 项 | 原因 |
|----|------|
| 多模型负载均衡 / 故障转移 | 属 X3 中间件多源 |
| 「测试连接」按钮 | 用户选择不做；保存即切换，失败由首次对话报错 |
| 每会话/长期绑定模型记忆 | 用户选择「每条消息可选」；前端仅本次会话内记住下拉值 |
| 上下文压缩 / 滚动摘要 | 独立里程碑 X4 |
| UI 暴露 `mock` provider | 避免管理员误切到假模型；本版 profile 固定 `openai_compatible` |
| 模型用量统计 / 成本看板 | YAGNI，后续 |
| 非 OpenAI 兼容 provider（Anthropic 原生等） | 现仅 `openai_compatible`；后续可扩 |

---

## 2. 数据模型与存储

新增 `store.ModelProfile`，新表 `model_profiles`（sqlite + postgres 建表 + 迁移；memory 同步）。

```go
type ModelProfile struct {
    ID              string    `json:"id"`
    Name            string    `json:"name"`                  // 展示名，同库唯一
    Provider        string    `json:"provider"`              // 本版固定 "openai_compatible"
    BaseURL         string    `json:"base_url"`
    Model           string    `json:"model"`
    APIKey          string    `json:"api_key,omitempty"`     // 原文存库；GET 永远脱敏
    APIKeyEnv       string    `json:"api_key_env,omitempty"` // api_key 为空时回退的环境变量名
    DisableThinking bool      `json:"disable_thinking"`
    SupportsVision  bool      `json:"supports_vision"`
    IsDefault       bool      `json:"is_default"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

- `id` 主键；`name` 唯一；任一时点最多一个 `is_default`（设默认在事务内清旧默认）。
- Store 接口新增：`UpsertModelProfile(ModelProfile) (ModelProfile, error)`、`GetModelProfile(id) (ModelProfile, error)`、`ListModelProfiles() []ModelProfile`、`DeleteModelProfile(id) error`、`SetDefaultModelProfile(id) error`。
- **脱敏：** 所有经 API 返回的 profile，`api_key` 经 `RedactAPIKey`（保留前 3 后 4，中间 `…`，空则空）。store 内部保存原文。
- **Key 往返：** PATCH 时若提交的 `api_key` 为空或等于脱敏占位形态 → 保留原 Key；否则覆盖。
- **种子：** bootstrap 启动后若 `ListModelProfiles()` 为空，用 YAML `llm` 段构造一个 profile（name 如「默认模型」，`is_default=true`），`api_key` 不入库、只记 `api_key_env`。之后以 DB 为唯一真相，YAML 仅首次种子。

`Run` 与 `CreateRunInput` 各新增字段：

```go
ModelProfileID string `json:"model_profile_id,omitempty"` // 该 run 使用的模型 profile
```

随 run 持久化（sqlite/postgres/memory 加列 + 迁移），对既有调用方可选、向后兼容。

---

## 3. 运行时模型解析（按 run 选模型 + 热切换）

新增 `llm.Switch`，实现 `llm.Provider` 接口，替代现在直接注入的单个 Provider。bootstrap 把**同一个** Switch 注入 engine、`srv.LLM`、微信 channel。

```go
type Switch struct {
    profiles ProfileSource          // 接口：按 id 取 profile / 取默认 profile
    cache    sync.Map               // profileID -> *cachedProvider{prov, updatedAt}
}

func (s *Switch) Chat(ctx, messages, tools) (Message, error)
func (s *Switch) SupportsVision() bool   // 转发默认 profile 的 Provider
```

**解析链（引擎 `runLoop` 调 `e.LLM.Chat` 处，`engine.go:417`）：**

1. `runLoop` 调 Chat 前，将当前 run 的 `ModelProfileID`（连同 runID）写入 `ctx`。
2. `Switch.Chat` 从 ctx 读 profile ID：
   - 指定且 profile 存在 → 用该 profile 的 Provider；
   - 未指定 / profile 已删 → 用默认 profile 的 Provider；
   - 一个 profile 都解析不到 → 返回明确错误「未配置可用模型」。
3. Provider 实例按 profile ID 缓存；缓存项记录 profile `UpdatedAt`，读取时若 profile 的 `UpdatedAt` 更新则重建（编辑后热生效）。旧实例服务完在途请求后被替换，无需重启、无锁热路径。

**构建 Provider：** 复用 `llm.NewOpenAI(baseURL, key, model)`；key 取值优先级 = profile 原文 `api_key`（非空）> `os.Getenv(api_key_env)`。`DisableThinking` / `SupportsVision` 从 profile 带入。

**微信 channel：** `SupportsVision` 改由 Switch 转发默认 profile（不再在 wiring 时缓存布尔），使默认模型变更后能力实时跟随。

---

## 4. 管理 API

全部挂 `/v0/settings/models`，角色见 §5。

| 方法 | 路径 | 作用 | 最低角色 |
|------|------|------|----------|
| GET | `/v0/settings/models` | 列出全部 profile（Key 脱敏，含 is_default） | operator（只读，供下拉） |
| POST | `/v0/settings/models` | 新建 profile | admin |
| PATCH | `/v0/settings/models/{id}` | 编辑 profile | admin |
| DELETE | `/v0/settings/models/{id}` | 删除 profile（默认拒绝） | admin |
| POST | `/v0/settings/models/{id}/default` | 设为默认（事务清旧默认） | admin |

**校验：**

- `provider` 本版只接受 `openai_compatible`。
- `name`、`base_url`、`model` 必填；`name` 同库唯一。
- `api_key` 与 `api_key_env` 不能同时为空（保存时即给出明确错误，避免保存后必失败；不做连通性测试）。
- 删除默认 profile → `400 default_cannot_delete`（须先把别的设为默认）。
- PATCH 的 `api_key` 为空或为脱敏占位 → 保留原 Key。

**发消息侧：** 创建 run 的入口（chat 发消息、渠道、Inbox、MCP 导出）在 `CreateRunInput` 可选带 `model_profile_id`。Chat UI 传下拉所选；其余入口不传 → 落默认。字段可选、向后兼容。

---

## 5. 权限

- 模型 profile 增删改 / 设默认：**admin**（写入 `acl.go` 路由最低角色表，与存储页、MCP 导出页一致）。
- `GET /v0/settings/models`：**operator 可读**（脱敏列表），供聊天下拉；与「账号」页 operator 可读自身同模式。
- `/v0/mcp/export` 等 RoleNone 路径不受影响。

---

## 6. 前端

**设置页 `/settings/models`（admin）：**

- `settingsNav.ts` admin 分支在「存储」附近加「模型」。
- 列表：名称、provider、base_url、model、能力标志（vision/thinking）、Key 脱敏值、默认徽章。
- 新建/编辑表单：name、base_url、model、API Key（密码框，留空=不改）、api_key_env（可选）、supports_vision、disable_thinking 开关、设为默认。
- 删除按钮：默认 profile 禁用并提示先改默认。
- 复用现有设置页卡片/表格/toast 与请求封装（参考 `McpExportSettings.tsx`、存储页）。

**聊天框模型下拉：**

- 输入区附近加下拉，选项来自 `GET /v0/settings/models`，默认选中 `is_default`。
- 每条消息发送前可切换；选择仅作用于即将发送的这条消息；前端在本次会话内记住下拉当前值，刷新后回到默认。
- 发消息请求体带 `model_profile_id`。

---

## 7. 边界与错误处理

- **删除被 run 引用的非默认 profile：** 允许；历史 run 的 `ModelProfileID` 指向已删 profile 时，Switch 回退默认，不报错、不阻断续跑。
- **凭证无效 / 模型名错误：** 不做测试连接，保存成功；首次对话失败走现有 LLM 错误路径，run 标记 failed 并带上游错误，UI 不回滚配置。
- **无任何 profile：** 种子逻辑保证启动后有默认；Switch 解析不到时返回「未配置可用模型」。
- **并发编辑：** profile upsert 走 store；Switch 缓存以 profile ID + UpdatedAt 为键，更新后下次 Chat 重建。
- **Key 脱敏往返：** 见 §2/§4，脱敏占位不覆盖真 Key。

---

## 8. 测试（TDD）

- **store：** profile CRUD、默认唯一约束、删默认被拒、设默认原子、Key 脱敏往返、sqlite/postgres/memory 三驱动 round-trip + 迁移；`Run.ModelProfileID` 持久化。
- **llm/switch：** 按 ctx profile ID 解析到对应 Provider；未指定/已删回退默认；profile 更新（UpdatedAt 变化）后重建；`SupportsVision` 跟随默认；无 profile 报错。
- **api：** CRUD 权限（operator 只读、admin 可写）；Key 不回显原文；删默认被拒；设默认原子；发消息带 `model_profile_id` 落库；api_key 与 api_key_env 同空被拒。
- **engine：** run 指定 profile 时 Switch 收到正确 profile；渠道/未指定走默认。
- **前端（vitest）：** 模型设置页增删改、默认徽章/删除禁用；聊天下拉发送携带 `model_profile_id`；导航项 admin 可见。

---

## 9. 文件影响面（概要）

- 新增：`internal/store/model_profiles.go`（+`_test.go`）、`internal/llm/switch.go`（+`_test.go`）、`internal/api/server_models.go`（+`_test.go`）、`web/chat/src/pages/ModelSettings.tsx`（+`_test.ts`）。
- 修改：`internal/store/{store.go,sqlite.go,postgres.go,memory.go}`（profile 表 + Run 列 + 迁移）、`internal/run/engine.go`（ctx 注入 profile ID）、`internal/bootstrap/bootstrap.go`（种子 + 注入 Switch）、`internal/api/server.go`（路由 + 发消息带 model_profile_id）、`internal/controlplane/acl.go`（路由角色）、`web/chat/src/{api.ts,settingsNav.ts,main.tsx}` 与聊天输入组件（模型下拉）。
