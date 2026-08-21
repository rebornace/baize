# Agent Skills（渐进激活）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 磁盘 Skill 包（`SKILL.md`）可加载/上传/删除；Agent 可挂默认 `skills`；Run 注入目录与正文，并用内置 `activate_skill` 在本 Run 内扩大工具并集。

**架构：** 新增 `internal/skill` 加载器与内存 Catalog。`store.Agent` / `agent.Def` / config 增加 `skills []string`。Engine 为每个 Run 持有激活 overlay（内存）；`runLoop` **每步**按 overlay 过滤 `Specs`，并在 `activate_skill` 成功后重写 `messages[0]` system。API 提供 Skills CRUD；设置页 Skills 管理默认 Agent 勾选。不把 Skill 升格为第六抽象。

**技术栈：** Go 1.22+、现有 httptest、`gopkg.in/yaml.v3`、`archive/zip`、React + Vite、vitest。不新加依赖。

**规格：** `docs/superpowers/specs/2026-08-21-agent-skills-design.md`（已批准）

**全局约束：**
- 不在 `main` 上改实现：先 `git checkout -b feat/agent-skills`
- 不改会话身份、HITL、Connector `auth.mode`、工具目录启停语义、控制面口令
- 不做在线正文编辑、市场、开跑前路由 LLM、跨 Run 永久激活
- commit 中文 `type(scope): 说明`；PowerShell 不要 bash HEREDOC
- Go：`C:\Users\Administrator\sdk\go\bin`；`GOPROXY=https://goproxy.cn,direct`
- 每步 Go 测试：`$env:GOPROXY='https://goproxy.cn,direct'; $env:PATH='C:\Users\Administrator\sdk\go\bin;'+$env:PATH; go test <pkg> -count=1`
- 前端：在 `web/chat` 下 `npm test`；嵌入 UI 后 `npm run build` 并提交 `internal/ui/dist/**`
- 开箱演示 Skill 的 `tools` 使用 mock-ticket 真实名：`list_tickets`、`get_ticket`、`create_ticket`、`update_ticket_status`（当前默认 spec 无 `login`）

**实现选定（相对规格 §4.5）：**
- `activate_skill` **不**写入 Connector 工具目录；由 Engine 合成进本 Run 的 tool specs，并在 `runLoop` 里特殊处理调用
- Run 激活集存在 Engine 内存 map；同进程 HITL resume 保持；进程重启后进行中 Run 回退为 Agent 默认 `skills`（第一版可接受）
- 激活成功后：更新 overlay + 重写 `messages[0]`（system）+ tool 结果摘要含新增工具名

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/skill/package.go` | 解析 `SKILL.md` frontmatter + body |
| `internal/skill/catalog.go` | 扫 builtin/user 目录、合并覆盖、InstallMD/InstallZip/DeleteUser、List/Get |
| `internal/skill/compose.go` | 组装目录段 + 激活正文；计算可见工具名并集 |
| `internal/skill/activate_spec.go` | `activate_skill` 的 `llm.ToolSpec` 常量 |
| `internal/skill/*_test.go` | 解析、覆盖、zip 逃逸、compose、可见集 |
| `internal/config/config.go` | `skills.builtin_dir` / `user_dir`；`agent.skills` |
| `internal/store/store.go` | `Agent.Skills`；`ListAgents()` |
| `internal/store/memory.go` / `sqlite.go` | 内存 Agent 读写（与今天一致，Agent 仍不落 SQLite 表） |
| `internal/agent/agent.go` | `Def.Skills` |
| `internal/run/engine.go` | Run overlay、过滤 Specs、处理 `activate_skill`、每步刷新 specs |
| `internal/run/skills_test.go` | Engine 级可见工具与激活 |
| `internal/api/server.go` | Skills 路由；PUT/GET Agent 含 skills；创建 Run 时传入 `Def.Skills` |
| `internal/controlplane/acl.go` | Skills 路由 ACL |
| `internal/bootstrap/bootstrap.go` | 加载 Catalog、挂到 Server/Engine；UpsertAgent 带 skills |
| `skills/ticket-triage/SKILL.md` | 开箱演示包 |
| `configs/default.yaml` | `skills.*` + `agent.skills: [ticket-triage]` |
| `web/chat/src/api.ts` | Skills / Agent API |
| `web/chat/src/pages/SkillsSettings.tsx` | 列表、上传、删除、默认 Agent 勾选 |
| `web/chat/src/settingsNav.ts` / `main.tsx` | 导航与路由 |
| `web/chat/src/style.css` | Skills 页样式（沿用 settings 既有风格） |
| `README.md` / `README.zh-CN.md` / `docs/architecture-and-plugin-protocol.md` | 文档 |
| `internal/ui/dist/**` | 前端构建产物 |

---

### 任务 0：功能分支

**文件：** 无代码

- [ ] **步骤 1：从最新 main 开分支**

```powershell
git checkout main
git pull
git checkout -b feat/agent-skills
```

预期：当前分支 `feat/agent-skills`。

---

### 任务 1：Skill 包解析与 Catalog

**文件：**
- 创建：`internal/skill/package.go`
- 创建：`internal/skill/catalog.go`
- 创建：`internal/skill/package_test.go`
- 创建：`internal/skill/catalog_test.go`

- [ ] **步骤 1：写失败测试（解析）**

```go
package skill_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/skill"
)

func TestParseSKILLMD(t *testing.T) {
	raw := "---\nname: ticket-triage\ndescription: 工单分诊\ntools:\n  - list_tickets\n  - create_ticket\n---\n\n# Body\n\nstep 1\n"
	pkg, err := skill.ParseSKILLMD([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "ticket-triage" || pkg.Description != "工单分诊" {
		t.Fatalf("%+v", pkg)
	}
	if len(pkg.Tools) != 2 || pkg.Tools[0] != "list_tickets" {
		t.Fatalf("tools=%v", pkg.Tools)
	}
	if !strings.Contains(pkg.Body, "# Body") {
		t.Fatalf("body=%q", pkg.Body)
	}
}
```

导入：`strings`、`testing`、`github.com/rebornace/baize/internal/skill`。

- [ ] **步骤 2：运行确认失败**

```powershell
go test ./internal/skill -count=1
```

预期：FAIL，找不到包或符号。

- [ ] **步骤 3：最少实现 ParseSKILLMD**

`package.go`：

```go
package skill

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type Package struct {
	ID          string
	Name        string
	Description string
	Tools       []string
	Body        string
	Source      string // builtin | user
	Dir         string
}

type frontmatter struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
}

func ParseSKILLMD(raw []byte) (Package, error) {
	const delim = "---"
	s := string(raw)
	if !strings.HasPrefix(strings.TrimSpace(s), delim) {
		return Package{}, fmt.Errorf("missing frontmatter")
	}
	rest := strings.TrimSpace(s)
	rest = strings.TrimPrefix(rest, delim)
	end := strings.Index(rest, "\n"+delim)
	if end < 0 {
		return Package{}, fmt.Errorf("unclosed frontmatter")
	}
	yamlPart := rest[:end]
	body := strings.TrimSpace(rest[end+len("\n"+delim):])
	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return Package{}, fmt.Errorf("frontmatter: %w", err)
	}
	if strings.TrimSpace(fm.Name) == "" {
		return Package{}, fmt.Errorf("name required")
	}
	return Package{
		Name:        strings.TrimSpace(fm.Name),
		Description: strings.TrimSpace(fm.Description),
		Tools:       fm.Tools,
		Body:        body,
	}, nil
}
```

不要导入未使用的 `bytes`。

- [ ] **步骤 4：测试通过**

```powershell
go test ./internal/skill -count=1 -run TestParseSKILLMD
```

预期：PASS。

- [ ] **步骤 5：写 Catalog 测试（user 覆盖 builtin、缺 SKILL.md 跳过）**

```go
func TestCatalogUserOverridesBuiltin(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	mustWriteSkill(t, filepath.Join(builtin, "demo"), "demo", "from-builtin", []string{"a"})
	mustWriteSkill(t, filepath.Join(user, "demo"), "demo", "from-user", []string{"b"})
	cat, err := skill.LoadCatalog(builtin, user)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cat.Get("demo")
	if !ok || p.Description != "from-user" || p.Source != "user" {
		t.Fatalf("%+v ok=%v", p, ok)
	}
}

func mustWriteSkill(t *testing.T, dir, name, desc string, tools []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\ndescription: " + desc + "\ntools:\n")
	for _, x := range tools {
		b.WriteString("  - " + x + "\n")
	}
	b.WriteString("---\n\nbody\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **步骤 6：实现 Catalog**

`catalog.go` 要点：
- `type Catalog struct { mu sync.RWMutex; byID map[string]Package; builtinDir, userDir string }`
- `LoadCatalog(builtin, user string) (*Catalog, error)`：扫一级子目录；有 `SKILL.md` 则解析；`ID = 文件夹名`；若 `name != id` 仍以文件夹名为 id（加载时 warn 可打日志）；先 builtin 再 user 覆盖
- `List() []Package` 按 id 排序
- `Get(id) (Package, bool)`
- `Reload()` 重新扫盘
- `InstallMD(filename string, raw []byte) (Package, error)`：Parse；id=`name`；写入 `userDir/<id>/SKILL.md`；Reload
- `InstallZip(raw []byte) (Package, error)`：zip 解压到临时目录；找根或唯一子目录下的 `SKILL.md`；拒绝 `..` 与绝对路径；再落到 userDir
- `DeleteUser(id string) error`：仅 `source==user`；`os.RemoveAll(userDir/id)`；Reload；builtin 返回错误

Zip 逃逸检查：对每个 `zip.File.Name`，`filepath.IsLocal`（Go 1.22）或手动拒绝前缀 `..` 与绝对路径。

- [ ] **步骤 7：Catalog 测试通过 + zip 负例**

追加 `TestInstallZipRejectsTraversal`：构造含 `../evil/SKILL.md` 的 zip，期望 error。

```powershell
go test ./internal/skill -count=1
```

预期：PASS。

- [ ] **步骤 8：Commit**

```powershell
git add internal/skill
git commit -m "feat(skill): 解析 SKILL.md 并合并 builtin/user 目录"
```

---

### 任务 2：Compose 与可见工具计算

**文件：**
- 创建：`internal/skill/compose.go`
- 创建：`internal/skill/compose_test.go`
- 创建：`internal/skill/activate_spec.go`

- [ ] **步骤 1：写失败测试**

```go
func TestComposeSystemAndVisibleTools(t *testing.T) {
	cat := skill.NewTestCatalog([]skill.Package{
		{ID: "a", Name: "a", Description: "A desc", Tools: []string{"t1", "disabled_skip"}, Body: "body-a"},
		{ID: "b", Name: "b", Description: "", Tools: []string{"t2"}, Body: "body-b"},
	})
	enabled := map[string]bool{"t1": true, "t2": true}
	sys := skill.ComposeSystem("base system", cat, []string{"a"})
	if !strings.Contains(sys, "base system") || !strings.Contains(sys, "a — A desc") || !strings.Contains(sys, "## Skill: a") {
		t.Fatalf("%s", sys)
	}
	vis := skill.VisibleTools(cat, []string{"a"}, enabled)
	if len(vis) != 1 || vis[0] != "t1" {
		t.Fatalf("%v", vis)
	}
	visEmptyDefault := skill.VisibleTools(cat, nil, enabled)
	if len(visEmptyDefault) != 2 {
		t.Fatalf("empty default skills => all enabled, got %v", visEmptyDefault)
	}
}
```

`NewTestCatalog` 可在 `_test` 同包用 map 注入，或 `compose_test` 与实现同包 `skill`。

- [ ] **步骤 2：实现**

```go
func ComposeSystem(base string, cat *Catalog, activated []string) string {
	var b strings.Builder
	b.WriteString(base)
	pkgs := cat.List()
	if len(pkgs) == 0 {
		return b.String()
	}
	b.WriteString("\n\n## Available skills\n")
	for _, p := range pkgs {
		desc := p.Description
		if desc == "" {
			desc = p.Name
		}
		b.WriteString("- ")
		b.WriteString(p.ID)
		b.WriteString(" — ")
		b.WriteString(desc)
		b.WriteString("\n")
	}
	for _, id := range activated {
		p, ok := cat.Get(id)
		if !ok {
			continue
		}
		b.WriteString("\n## Skill: ")
		b.WriteString(id)
		b.WriteString("\n")
		b.WriteString(p.Body)
		b.WriteString("\n")
	}
	return b.String()
}

// VisibleTools: defaultIDs 非空 => ∪ tools ∩ enabled；空 => 全部 enabled 名（排序）
func VisibleTools(cat *Catalog, defaultOrActivated []string, enabled map[string]bool) []string

func ActivateToolSpec() llm.ToolSpec // name=activate_skill；parameters id 或 ids
```

`activate_spec.go` 的 schema：

```json
{
  "type": "object",
  "properties": {
    "id": { "type": "string" },
    "ids": { "type": "array", "items": { "type": "string" } }
  }
}
```

描述文案写清：根据 Available skills 激活流程与工具。

- [ ] **步骤 3：测试通过并 Commit**

```powershell
go test ./internal/skill -count=1
git add internal/skill
git commit -m "feat(skill): system 组装与可见工具并集"
```

---

### 任务 3：Config / Store / Agent.Def 增加 skills

**文件：**
- 修改：`internal/config/config.go`
- 修改：`internal/config/config_test.go`（若无则加）
- 修改：`internal/store/store.go`
- 修改：`internal/store/memory.go`
- 修改：`internal/store/sqlite.go`
- 修改：`internal/store/store_test.go`
- 修改：`internal/agent/agent.go`
- 修改：`internal/bootstrap/bootstrap.go`

- [ ] **步骤 1：写 Store 失败测试**

在现有 Agent 测试旁：

```go
func TestAgentSkillsRoundTrip(t *testing.T) {
	s := store.NewMemory()
	s.UpsertAgent(store.Agent{ID: "ticket-agent", System: "sys", Skills: []string{"ticket-triage"}})
	got, err := s.GetAgent("ticket-agent")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Skills) != 1 || got.Skills[0] != "ticket-triage" {
		t.Fatalf("%+v", got)
	}
	all := s.ListAgents()
	if len(all) != 1 {
		t.Fatalf("list=%v", all)
	}
}
```

- [ ] **步骤 2：改类型与实现**

```go
// store.Agent
type Agent struct {
	ID     string   `json:"id"`
	System string   `json:"system"`
	Skills []string `json:"skills,omitempty"`
}

// Store 增加
ListAgents() []Agent
```

Memory / SQLite 的 agents map 读写带上 Skills；`ListAgents` 按 id 排序。

```go
// agent.Def
type Def struct {
	ID           string
	System       string
	Skills       []string
	ConnectorIDs []string
}
```

```go
// config.Config
Skills struct {
	BuiltinDir string `yaml:"builtin_dir"`
	UserDir    string `yaml:"user_dir"`
} `yaml:"skills"`

// Agent 段
Agent struct {
	ID     string   `yaml:"id"`
	System string   `yaml:"system"`
	Skills []string `yaml:"skills"`
} `yaml:"agent"`
```

`Load` 默认：`BuiltinDir` 空 → `./skills`；`UserDir` 空 → `./data/skills`。

Bootstrap：`st.UpsertAgent(store.Agent{ID: cfg.Agent.ID, System: cfg.Agent.System, Skills: append([]string(nil), cfg.Agent.Skills...)})`；创建 `skill.LoadCatalog`；把 `*skill.Catalog` 挂到将传到 API/Engine 的字段（可先放 `bootstrap.App` / `api.Server` 结构体，任务 4–5 接上）。

- [ ] **步骤 3：测试通过并 Commit**

```powershell
go test ./internal/store ./internal/config ./internal/agent -count=1
git add internal/store internal/config internal/agent internal/bootstrap
git commit -m "feat(agent): Agent.skills 与 skills 目录配置"
```

---

### 任务 4：Engine Run overlay 与 activate_skill

**文件：**
- 修改：`internal/run/engine.go`
- 创建：`internal/run/skills_overlay.go`（可选，保持 engine.go 不膨胀）
- 创建：`internal/run/skills_test.go`
- 修改：调用 `Execute` 处传入 `Def.Skills`（`internal/api/server.go` 最小改动可本任务一起做）

- [ ] **步骤 1：写失败测试**

用假 Registry：注册 `list_tickets`、`create_ticket`；Catalog 装一个 skill 仅含 `list_tickets`；Agent `Skills:[demo]`。

```go
func TestExecuteFiltersToolsByDefaultSkills(t *testing.T) {
	// mock LLM：第一步读到的 specs 名集合不得含 create_ticket；须含 list_tickets 与 activate_skill
}

func TestActivateSkillExpandsTools(t *testing.T) {
	// mock LLM：第一步调用 activate_skill{id:demo2}；第二步 specs 含 demo2 的工具
}

func TestActivateSkillUnknownIDIsToolError(t *testing.T) {
	// 未知 id → tool result is_error；Run 最终仍可 succeeded（LLM 第二步不调工具）
}
```

参照现有 `engine_test.go` 的 mock LLM 写法。

- [ ] **步骤 2：Engine 字段与 overlay API**

```go
type Engine struct {
	// ...existing...
	Skills *skill.Catalog
	runMu  sync.Mutex
	runs   map[string]*runSkillState
}

type runSkillState struct {
	activated []string
	// defaultNonEmpty 标记初始 Agent.skills 是否非空（决定空激活时是否全量工具）
	defaultNonEmpty bool
}
```

- `beginRunSkills(runID string, defaultSkills []string)`：过滤能 Get 到的 id；写入 state
- `specsForRun(runID string) []llm.ToolSpec`：从 `Tools.Specs()` 按 `VisibleTools` 过滤；若 `Skills.List()` 非空则追加 `ActivateToolSpec()`
- `handleActivateSkill(runID string, args map[string]any) (content map[string]any, isError bool)`
- `composeSystem(base string, runID string) string`

- [ ] **步骤 3：改 Execute / ContinueFromHITL / runLoop**

`Execute`：
```go
e.beginRunSkills(runID, ag.Skills)
sys := e.composeSystem(ag.System, runID)
messages := e.buildMessages(sys, ...)
```

`ContinueFromHITL`：若 `runs[runID]` 不存在则 `beginRunSkills(runID, ag.Skills)`（需把 `ag` 传入；若签名已有 `ag` 则用；否则从 Store.GetAgent(run.AgentID)）。

`runLoop`：**删除**循环外一次性 `specs := e.Tools.Specs()`；改为循环内：
```go
specs := e.specsForRun(runID)
```

工具调用分支，在 login/HITL 之前：
```go
if tc.Name == skill.ActivateToolName {
	content, isError := e.handleActivateSkill(runID, tc.Arguments)
	// 成功则 messages[0].Content = e.composeSystem(originalBase, runID)
	// 问题：base system 要保存。在 runSkillState 里存 baseSystem string。
	...
	continue 前走统一 tool result 追加逻辑
}
```

`handleActivateSkill`：解析 `id` / `ids`；未知 → `is_error`；已知 → append 去重到 activated；返回 `{activated, added_tools}`。

注意：`RequiresApproval("activate_skill")` 必须为 false（未注册则 false）。

- [ ] **步骤 4：API 创建 Run 传入 Skills**

```go
def := agent.Def{ID: ag.ID, System: ag.System, Skills: append([]string(nil), ag.Skills...)}
```

Resume 路径同样带上 `GetAgent` 的 Skills。

- [ ] **步骤 5：测试通过并 Commit**

```powershell
go test ./internal/run -count=1
git add internal/run internal/api/server.go
git commit -m "feat(run): Skill 目录注入与 activate_skill"
```

---

### 任务 5：Skills HTTP API + ACL + Agent GET/PUT

**文件：**
- 修改：`internal/controlplane/acl.go`
- 修改：`internal/controlplane/acl_test.go`
- 修改：`internal/api/server.go`
- 创建：`internal/api/server_skills_test.go`
- 修改：`internal/api` 中 Agent 相关测试（若有）
- 修改：`internal/bootstrap/bootstrap.go`（Server.Skills = catalog）

- [ ] **步骤 1：ACL 测试**

```go
{"GET", "/v0/skills", RoleAdmin},
{"GET", "/v0/skills/x", RoleAdmin},
{"POST", "/v0/skills", RoleAdmin},
{"DELETE", "/v0/skills/x", RoleAdmin},
{"GET", "/v0/agents/ticket-agent", RoleAdmin},
```

PUT agents 已是 Admin。操作员对 POST skills 期望 MinRole=Admin（门禁下 403 测放在 server 测）。

- [ ] **步骤 2：实现路由**

```go
s.mux.HandleFunc("GET /v0/skills", s.handleListSkills)
s.mux.HandleFunc("GET /v0/skills/{id}", s.handleGetSkill)
s.mux.HandleFunc("POST /v0/skills", s.handlePostSkill)
s.mux.HandleFunc("DELETE /v0/skills/{id}", s.handleDeleteSkill)
s.mux.HandleFunc("GET /v0/agents/{id}", s.handleGetAgent)
```

`handlePutAgent` body：
```go
var body struct {
	System string   `json:"system"`
	Skills []string `json:"skills"`
}
s.Store.UpsertAgent(store.Agent{ID: id, System: body.System, Skills: body.Skills})
```

`handlePostSkill`：`r.ParseMultipartForm`；取 `file`；按扩展名 `.md` / `.zip` 调用 Catalog；返回摘要 JSON。

`handleDeleteSkill`：`DeleteUser`；然后 `for _, a := range s.Store.ListAgents()` 过滤掉该 id 再 UpsertAgent。

- [ ] **步骤 3：API 测试**

- 上传 md → GET 列表含 user source  
- zip 上传  
- 删 builtin → 400  
- 删 user → Agent.skills 去掉 id  
- 操作员 token POST → 403（复用现有 gate 测试脚手架）

- [ ] **步骤 4：通过并 Commit**

```powershell
go test ./internal/controlplane ./internal/api -count=1
git add internal/controlplane internal/api internal/bootstrap
git commit -m "feat(api): Skills 上传删除与 Agent.skills"
```

---

### 任务 6：开箱 ticket-triage + default.yaml

**文件：**
- 创建：`skills/ticket-triage/SKILL.md`
- 修改：`configs/default.yaml`

- [ ] **步骤 1：写入演示包**

```markdown
---
name: ticket-triage
description: 工单分诊与建单流程（mock-ticket）
tools:
  - list_tickets
  - get_ticket
  - create_ticket
  - update_ticket_status
---

# 工单分诊

1. 先用 list_tickets / get_ticket 了解现状
2. 需要新建时再 create_ticket（可能触发人工审批）
3. 改状态用 update_ticket_status（可能触发人工审批）
```

`default.yaml` 增加：

```yaml
skills:
  builtin_dir: ./skills
  user_dir: ./data/skills
agent:
  id: ticket-agent
  system: "你是企业工单助手，只能通过工具访问工单系统。"
  skills:
    - ticket-triage
```

- [ ] **步骤 2：冒烟——Catalog 能加载仓库内包**

小测试或手工：

```powershell
go test ./internal/skill -count=1 -run TestLoadRepoTicketTriage
```

测试把 builtin 指到仓库 `skills` 相对路径（或 `filepath` 从 testdata 复制）。更稳：单元测试用 TempDir 已覆盖；本任务用集成测试在任务 8。

- [ ] **步骤 3：Commit**

```powershell
git add skills/ticket-triage/SKILL.md configs/default.yaml
git commit -m "feat(skills): 开箱 ticket-triage 演示包"
```

---

### 任务 7：设置页 Skills GUI

**文件：**
- 修改：`web/chat/src/api.ts`
- 修改：`web/chat/src/settingsNav.ts`
- 修改：`web/chat/src/main.tsx`
- 创建：`web/chat/src/pages/SkillsSettings.tsx`
- 修改：`web/chat/src/style.css`
- 可选：`web/chat/src/pages/SkillsSettings.test.tsx` 或纯函数测勾选合并

- [ ] **步骤 1：api.ts**

```ts
export type SkillSummary = {
  id: string
  name: string
  description: string
  tools: string[]
  source: 'builtin' | 'user'
}

export async function listSkills(): Promise<{ skills: SkillSummary[] }>
export async function uploadSkill(file: File): Promise<SkillSummary>
export async function deleteSkill(id: string): Promise<void>
export async function getAgent(id: string): Promise<{ id: string; system: string; skills?: string[] }>
export async function putAgent(id: string, body: { system: string; skills: string[] }): Promise<void>
```

沿用现有 `authFetch` / 错误处理模式。

- [ ] **步骤 2：导航与路由**

`settingsNavItems` admin 在 Tools 后插入 `{ to: '/settings/skills', label: 'Skills' }`。

`main.tsx` 增加 AdminOnly 路由 `skills` → `SkillsSettings`。

- [ ] **步骤 3：SkillsSettings 页面行为**

- 挂载：`listSkills()` + `getAgent`（agent id 来自 `ui-config` 若已有，否则写死读取 `/v0/ui-config` 或现有 chat 用的 default agent；查 `ui-config` 是否已返回 `agent_id`，有则用，无则 `ticket-agent`）
- 表格/列表：id、description、source 徽章、tools 逗号摘要
- 上传：`<input type="file" accept=".md,.zip">` → `uploadSkill` → 刷新列表
- 删除：仅 `source==='user'` 显示按钮 → `deleteSkill`
- 多选：已安装 skills 勾选 = 默认 Agent.skills；保存 → `putAgent({ system: 当前 system, skills: 勾选 ids })`（先 GET 保留 system）

不做在线编辑器。样式类名跟 `ToolsSettings` 同一套 settings-*，避免新视觉体系。

- [ ] **步骤 4：前端测试 + build**

```powershell
cd web/chat; npm test; npm run build
```

将 `internal/ui/dist` 更新纳入提交。

- [ ] **步骤 5：Commit**

```powershell
git add web/chat internal/ui/dist
git commit -m "feat(ui): Skills 设置页上传删除与默认勾选"
```

---

### 任务 8：文档 + 开箱集成回归

**文件：**
- 修改：`README.md`、`README.zh-CN.md`
- 修改：`docs/architecture-and-plugin-protocol.md` §5
- 修改或追加：`tests/integration/starter_test.go`（断言默认挂 skill 时 mock 路径仍绿；或新 `skills_test.go` 只测加载）

- [ ] **步骤 1：文档要点（中英一致）**

- Skills 可选：流程 Markdown + tools 列表  
- 目录 `./skills` 与 `./data/skills`；上传/删除；同 id 用户覆盖内置  
- `agent.skills` 默认激活；对话中 `activate_skill`；空 skills = 全量已启用工具  
- 与工具目录 `enabled` 求交；不能启用已停用工具  
- **不是** Cursor 个人编码 Skill 市场 / 不保证 `grill-me` 原样子兼容  

架构草案：发现（目录段）/ 激活（Run overlay）语义；强调配置形态不升格。

- [ ] **步骤 2：全量测试**

```powershell
go test ./... -count=1
cd web/chat; npm test
```

预期：全绿。`tests/integration` 开箱路径不因默认 `ticket-triage` 失败（可见工具仍含 create_ticket 等）。

- [ ] **步骤 3：Commit**

```powershell
git add README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md tests/integration
git commit -m "docs(skills): Agent Skills 渐进激活说明"
```

---

## 自检对照规格

| 规格 | 任务 |
|------|------|
| §1 成功标准 1 开箱包 + YAML | 6 |
| §1.2–1.3 目录段、默认正文、工具并集 | 2、4 |
| §1.4 默认 skills 空 → 全量 enabled | 2、4 |
| §1.5 activate 未知 id / 求交 | 4 |
| §1.6–1.7 上传 md/zip、删 user、摘 Agent | 1、5 |
| §1.8 ACL 操作员 403 | 5 |
| §1.9 开箱集成仍绿 | 8 |
| §1.10 设置页 | 7 |
| §3 包格式 / 覆盖 / zip 逃逸 | 1 |
| §4.4 Run 作用域激活 | 4 |
| §4.5 内置 activate_skill | 2、4 |
| §5 API | 5 |
| §6 GUI | 7 |
| §7 热更新（POST 后新 Run） | 1 Reload + 5 |
| §8 测试清单 | 各任务 |
| §9 文档 | 8 |
| 不做项（编辑器/市场/路由 LLM/升格） | 全局约束 |

**占位符扫描：** 无 TODO/待定步骤。  
**类型一致性：** `skills []string`、Catalog `Package.ID`、工具名 `activate_skill`、source `builtin`/`user` 全程统一。

---

## 执行交接

计划已完成并保存到 `docs/superpowers/plans/2026-08-21-agent-skills.md`。

规格状态已改为 **已批准**。实现前请再确认本计划；确认后两种执行方式：

**1. 子代理驱动（推荐）** — 每任务新子代理 + 任务间审查  

**2. 内联执行** — 本会话用 executing-plans 批量推进并设检查点  

选哪种？确认计划后我会从任务 0 开 `feat/agent-skills` 分支开始做（不在 `main` 上改实现）。
