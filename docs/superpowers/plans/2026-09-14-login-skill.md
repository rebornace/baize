# LOGIN-SKILL：连接器自动登录 Skill 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 连接器变更时自动维护 `login-<connector_id>` managed Skill；Chat 仅通过 `@`/`/` 技能使用登录；拆除 UI-LOGIN-AT 直达（login-entries / login-invoke / 登录专用区）。

**架构：** `internal/skill/loginmanage` 根据 Store 枚举 tools（capture ∪ companion）读写 `{UserDir}/managed/login-<id>/SKILL.md`；Catalog 增加 managed 扫描根与 `SourceManaged`；bootstrap/连接器 API 触发 Sync+Reload；前端「去登录」写入 `@login-<id>`。

**技术栈：** Go、既有 `skill.Catalog`、React/Vitest；中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-14-login-skill-design.md`

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/skill/loginmanage/select.go` | `SelectTools`、companion 模式表、id 规范化、logout 排除 |
| `internal/skill/loginmanage/select_test.go` | 选择规则单测 |
| `internal/skill/loginmanage/sync.go` | `SyncAll` / `SyncConnector`：写/删 managed 包、冲突跳过 |
| `internal/skill/loginmanage/sync_test.go` | 落盘、覆盖、冲突、空集删除 |
| `internal/skill/loginmanage/render.go` | SKILL.md 模板渲染 |
| `internal/skill/package.go` | frontmatter 解析 `managed` / `managed_kind` / `managed_connector_id`；`Package` 字段 |
| `internal/skill/catalog.go` | `SourceManaged`；`LoadCatalog(..., managedDir)`；Reload 扫描 managed |
| `internal/skill/catalog_test.go` | managed 加载与优先级 |
| `internal/bootstrap/bootstrap.go` | managed 根路径；启动 SyncAll；注入 Server |
| `internal/api/server.go` 等 | PUT/DELETE connector、工具 PATCH/Replace 后 Sync；拆除 login 路由 |
| `internal/api/server_login_entry.go` 等 | **删除** |
| `internal/run/forced_tool.go` 等 | 若无其它引用则删除；否则保留但无 HTTP |
| `internal/middleware/job.go` / reconciler | 移除 KindForcedTool（若仅 login 使用） |
| `internal/store/*` ForcedTool 字段 | 若仅 login-invoke 使用则删除并迁移测试 |
| `internal/loginentry/*` | 删除或瘦身为 loginmanage 依赖后删除 |
| `internal/controlplane/acl.go` | 去掉 login-entries/invoke |
| `web/chat/src/components/Composer.tsx` | 去掉登录区 |
| `web/chat/src/components/ToolCard.tsx` | 「去登录」→ `onGoLoginSkill(skillId)` |
| `web/chat/src/pages/ChatPage.tsx` | 写入 `@login-…`；拆 LoginPicker/invoke |
| 删除 | `LoginPicker*`、`LoginParamsModal*`、`Composer.login.test.tsx` 直达用例、`loginEntry.ts` 中仅直达部分、api loginInvoke |
| `docs/...` 账本 / UI-LOGIN-AT 状态 | 交付注记 |
| `internal/ui/dist/**` | build 嵌入 |

**Catalog 加载顺序（后写覆盖先写）：** builtin → user → managed。这样 managed 可覆盖同名 user **仅当 Sync 未跳过**；Sync 侧若发现 user 非 managed 同名则不写盘，故正常不会踩。若盘上已有 managed 包，Reload 用 managed 源展示。

**Managed 根：** `filepath.Join(cfg.Skills.UserDir, "managed")`（默认 `./data/skills/managed`）。用户根扫描会跳过无 `SKILL.md` 的 `managed` 子目录名（已有 IsNotExist continue）。

---

### 任务 1：tools 选择规则

**文件：** 创建 `internal/skill/loginmanage/select.go`、`select_test.go`

- [ ] **步骤 1：失败测试**

```go
func TestSelectToolsCaptureAndCompanion(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "auth", Type: "openapi"})
	st.ReplaceConnectorTools("auth", []store.Tool{
		{ConnectorID: "auth", Name: "AuthController_phoneLogin", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_sendSms", Enabled: true},
		{ConnectorID: "auth", Name: "AuthController_logout", Enabled: true},
		{ConnectorID: "auth", Name: "listThings", Enabled: true},
		{ConnectorID: "auth", Name: "old_login", Enabled: false},
	})
	got := loginmanage.SelectTools(st, "auth")
	// want phoneLogin + sendSms only (sorted)
}
func TestSkillID(t *testing.T) {
	if loginmanage.SkillID("crm") != "login-crm" { t.Fatal() }
	if loginmanage.SkillID("a/b") == "" && loginmanage.NormalizeConnectorID("a/b") /* 仅保留安全字符 */ {
		// 规范化后非空则 SkillID 有值
	}
}
```

- [ ] **步骤 2：`go test` 确认失败**

- [ ] **步骤 3：实现** `SelectTools`、`CompanionGlobs` 常量表（规格 §5.2）、logout 排除、CaptureDefaults+MatchToolName

- [ ] **步骤 4：测试通过并 commit**

```bash
git commit -m "feat(loginmanage): 登录 skill 工具选择（capture∪companion）"
```

---

### 任务 2：SKILL.md 渲染 + Sync 落盘

**文件：** `render.go`、`sync.go`、`*_test.go`

- [ ] **步骤 1：失败测试**

```go
func TestSyncWritesManagedPackage(t *testing.T) {
	root := t.TempDir()
	managed := filepath.Join(root, "managed")
	st := /* auth + phoneLogin + sendSms */
	userSkills := filepath.Join(root, "user") // 空
	err := loginmanage.SyncConnector(st, managed, userSkills, "auth")
	// read managed/login-auth/SKILL.md
	// assert managed: true, managed_kind, tools 含二者, 无 workflow.yaml
}
func TestSyncSkipsNonManagedConflict(t *testing.T) {
	// userSkills/login-auth/SKILL.md 无 managed:true
	// Sync 不覆盖；内容不变
}
func TestSyncDeletesWhenNoTools(t *testing.T) {
	// 先写入包，再清空工具，Sync → 目录删除
}
```

- [ ] **步骤 2–4：** 实现 `RenderSKILLMD`、`SyncConnector`、`SyncAll`（扫 openapi/http）；冲突检测读 userDir 与 managedDir；commit

```bash
git commit -m "feat(loginmanage): 同步 managed 登录 SKILL.md 落盘"
```

---

### 任务 3：Catalog 加载 managed

**文件：** `internal/skill/package.go`、`catalog.go`、`catalog_test.go`；所有 `LoadCatalog` 调用点加 managed 参数或 Options

**推荐签名（破坏性但开发阶段可接受）：**

```go
func LoadCatalog(builtinDirs []string, userDir, managedDir string) (*Catalog, error)
```

- `SourceManaged = "managed"`
- Reload：builtin → user → managed（后覆盖前）
- ParseSKILLMD 解析 managed 字段到 Package
- DeleteUser：禁止删 managed（或仅允许删 user）；`ErrManaged` 可选

- [ ] **步骤 1：** 测 managed 目录下包可 List/Get；Source=managed  
- [ ] **步骤 2：** 更新 bootstrap + 全仓库 LoadCatalog 调用（编译通过）  
- [ ] **步骤 3：** commit

```bash
git commit -m "feat(skill): Catalog 加载 managed 技能根"
```

---

### 任务 4：接线 Sync（启动 + 连接器 API）

**文件：** `bootstrap.go`、`server.go`（及 delete connector / tools PATCH 路径）

- [ ] **步骤 1：** API 测：PUT connector + tools → 磁盘出现 managed 包且 GET `/v0/skills` 含 `login-<id>`（需 Server 挂 Skills catalog）  
- [ ] **步骤 2：** 在 `handlePutConnector` 成功路径、删除连接器、工具变更成功路径调用 `SyncConnector` + `Skills.Reload`；Sync 失败只 log  
- [ ] **步骤 3：** bootstrap 在 LoadCatalog 前或后 `SyncAll` 再 Reload  
- [ ] **步骤 4：** commit

```bash
git commit -m "feat(api): 连接器变更同步登录 managed skill"
```

---

### 任务 5：拆除 login-invoke 后端

**文件：** 删除 `server_login_entry.go`、`server_login_entry_test.go`；acl；`KindForcedTool` 全链路；store ForcedTool 字段（若仅此用途）；`internal/loginentry` 若已无引用则删

- [ ] **步骤 1：** 测路由不存在 / acl 无条目  
- [ ] **步骤 2：** 删代码至 `go test ./internal/api/ ./internal/middleware/ ./internal/run/ ./internal/store/ ./internal/controlplane/` 绿  
- [ ] **步骤 3：** commit

```bash
git commit -m "refactor(api): 拆除 login-entries/login-invoke 直达"
```

---

### 任务 6：前端收敛到 Skill

**文件：** ChatPage、Composer、ToolCard、strings、api；删除 LoginPicker/LoginParamsModal/直达测试；改 ToolCard「去登录」

- [ ] **步骤 1：** ToolCard 测：`onGoLoginSkill?.('login-crm')` 在 login_required 时触发（有 connector_id）  
- [ ] **步骤 2：** ChatPage：`onGoLoginSkill` → composer 写入 `@login-crm `（用现有 replaceMention/setDraft）；无 skill 时 toast  
- [ ] **步骤 3：** Composer 移除 loginEntries/onPickLogin 分区  
- [ ] **步骤 4：** 删除死代码；vitest 相关套件绿；`npm run build` + dist  
- [ ] **步骤 5：** commit

```bash
git commit -m "feat(webui): 去登录跳转 login skill；拆除登录直达 UI"
```

---

### 任务 7：账本与回归

- [x] 更新 ledger / 确认清单：LOGIN-SKILL 已交付（待合入）；UI-LOGIN-AT 直达已拆除  
- [x] `go test` 相关包 + vitest 登录/技能相关  
- [x] commit

```bash
git commit -m "docs(LOGIN-SKILL): 账本与直达拆除注记"
```

---

## 自检（对照规格）

| 规格 | 任务 |
|------|------|
| Sync 生成 managed 包 | 1–2、4 |
| companion ∪ capture、无 workflow | 1–2 |
| 厂商中立 managed 字段 | 2–3 |
| Catalog 可见 | 3–4 |
| 拆除 login-invoke/UI | 5–6 |
| 去登录 → @login- | 6 |
| 非 managed 不覆盖 | 2 |
| 模型帮登保留 | 不改 capture 路径 |

无「待定」占位；`LoadCatalog` 三参数变更须一次改全调用点。
