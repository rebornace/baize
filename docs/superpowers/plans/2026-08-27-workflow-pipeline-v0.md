# 工作流线性流水线 v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** Skill 包可选附带 `workflow.yaml` 线性流水线；Run 激活该 skill 时引擎按序执行工具步骤（不调 LLM），审批复用 HITL，新增 3 个 workflow 事件，前端最小渲染。

**架构：** 新建 `internal/workflow`（model/parse/template/exec）；skill Catalog 加载时挂载 Workflow 到 Package；engine 在 activate_skill 特判处发现带 Workflow 的 skill 即进入 `runWorkflow`，跑完后终止本轮 ReAct 循环；结果表可从事件流重建支持冷恢复。

**技术栈：** Go 1.25 / yaml.v3 / 既有 store 事件通道 / vitest。

**规格：** `docs/superpowers/specs/2026-08-27-workflow-pipeline-v0-design.md`

**Git：** 分支 `feat/workflow-pipeline-v0`

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `internal/workflow/model.go` | Workflow/Step 类型 + 校验 |
| `internal/workflow/parse.go` | yaml.v3 解析 → Workflow |
| `internal/workflow/template.go` | `{{路径}}` 渲染器 |
| `internal/workflow/exec.go` | Executor：顺序执行 + HITL + 事件 |
| 各自 `_test.go` | 单测 |
| `internal/skill/package.go` / `catalog.go` | Package 增加 `Workflow *Workflow`；scanDir 读文件 |
| `internal/run/engine.go` | 激活切模式 + runWorkflow 入口 + 冷恢复重建 |
| `web/chat/src/foldEvents.ts` + test | 3 种 block |
| `examples/skills/ticket-triage/workflow.yaml` | 示例 |
| README ×2、docs/architecture-and-plugin-protocol.md | 一句话说明 |

### 计划级裁定（实现者必须遵守）

1. **input 形状**：Run 输入是 string；模板数据树中 `input = {"text": <用户输入字符串>}`。v0 唯一合法引用根为 `{{input.text}}`。
2. **step.result 形状**：直接存 `Registry.Invoke` 返回的 `(content map[string]any, isError bool, err error)` 中的 content；isError=true 或 err!=nil → 步骤失败。
3. **工作流完成后的终态**：所有步骤成功 → run succeeded（不发 `llm.message` 总结）；事件 `run.ended` 由既有合成逻辑生成。
4. **多 skill 同时带 workflow**：先激活者生效；后续再激活带 workflow 的 skill 仅提示 available（不切换）。判定字段放 runSkillState：`workflowStarted bool`、`workflowSkill string`。

---

### 任务 1：workflow 模型与解析（model + parse）

**文件：**
- 创建：`internal/workflow/model.go`、`internal/workflow/model_test.go`
- 创建：`internal/workflow/parse.go`、`internal/workflow/parse_test.go`

- [ ] **步骤 1：编写失败的测试**

`model_test.go`：

```go
package workflow_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/workflow"
)

func TestWorkflowValidateRejectsEmptyName(t *testing.T) {
	w := &workflow.Workflow{Steps: []workflow.Step{{ID: "a", Tool: "x"}}}
	err := w.Validate()
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err=%v", err)
	}
}

func TestWorkflowValidateRejectsDuplicateStepID(t *testing.T) {
	w := &workflow.Workflow{Name: "n", Steps: []workflow.Step{
		{ID: "a", Tool: "x"}, {ID: "a", Tool: "y"},
	}}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err=%v", err)
	}
}

func TestWorkflowValidateRejectsEmptyToolOrID(t *testing.T) {
	w := &workflow.Workflow{Name: "n", Steps: []workflow.Step{{ID: "", Tool: "x"}}}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("err=%v", err)
	}
	w2 := &workflow.Workflow{Name: "n", Steps: []workflow.Step{{ID: "a", Tool: ""}}}
	if err := w2.Validate(); err == nil || !strings.Contains(err.Error(), "tool") {
		t.Fatalf("err=%v", err)
	}
}
```

`parse_test.go`：

```go
package workflow_test

import (
	"testing"

	"github.com/rebornace/baize/internal/workflow"
)

const sample = `
name: ticket-triage
steps:
  - id: fetch
    tool: search_tickets
    args:
      query: "{{input.text}}"
  - id: reply
    tool: reply_ticket
    approve: true
    args:
      text: "{{fetch.result.summary}}"
`

func TestParseOK(t *testing.T) {
	w, err := workflow.Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if w.Name != "ticket-triage" || len(w.Steps) != 2 {
		t.Fatalf("w=%+v", w)
	}
	if !w.Steps[1].Approve {
		t.Fatalf("approve=%v", w.Steps[1].Approve)
	}
	if w.Steps[0].Args["query"] != "{{input.text}}" {
		t.Fatalf("args=%v", w.Steps[0].Args)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	bad := "name: n\nsteps:\n  - id: a\n    tool: x\n    when: \"{{a.result}}\"\n"
	if _, err := workflow.Parse([]byte(bad)); err == nil {
		t.Fatal("want unknown-field error")
	}
}

func TestParseRequiresWorkflowKey(t *testing.T) {
	if _, err := workflow.Parse([]byte("name: n\n")); err == nil {
		t.Fatal("want steps required")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

```powershell
$env:Path = "C:\Users\Administrator\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.25.0.windows-amd64\bin;D:\Git\bin;" + $env:Path
$env:GOTOOLCHAIN = "local"; $env:GOPROXY = "https://goproxy.cn,direct"
cd C:\Users\Administrator\Desktop\baize
go test ./internal/workflow/ -count=1
```
预期：编译错误 `package workflow not found` 类。

- [ ] **步骤 3：实现**

`model.go`：

```go
package workflow

// Workflow is a linear tool pipeline declared in a skill's workflow.yaml.
type Workflow struct {
	Name  string `yaml:"name"`
	Steps []Step `yaml:"steps"`
}

// Step invokes one registered tool; Approve routes it through HITL.
type Step struct {
	ID      string         `yaml:"id"`
	Tool    string         `yaml:"tool"`
	Approve bool           `yaml:"approve"`
	Args    map[string]any `yaml:"args"`
}

func (w *Workflow) Validate() error {
	if strings.TrimSpace(w.Name) == "" {
		return errors.New("workflow: name required")
	}
	if len(w.Steps) == 0 {
		return errors.New("workflow: steps required")
	}
	seen := map[string]bool{}
	for i, s := range w.Steps {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("workflow step %d: id required", i+1)
		}
		if seen[s.ID] {
			return fmt.Errorf("workflow: duplicate step id %q", s.ID)
		}
		seen[s.ID] = true
		if strings.TrimSpace(s.Tool) == "" {
			return fmt.Errorf("workflow step %q: tool required", s.ID)
		}
	}
	return nil
}
```

`parse.go`：

```go
package workflow

func Parse(raw []byte) (*Workflow, error) {
	var w Workflow
	dec := yaml.New(bytes.NewReader(raw))
	dec.KnownFields(true) // 未知字段直接报错 —— 锁死语法面
	if err := dec.Decode(&w); err != nil {
		return nil, fmt.Errorf("workflow.yaml: %w", err)
	}
	if err := (&w).Validate(); err != nil {
		return nil, err
	}
	return &w, nil
}
```

- [ ] **步骤 4：运行测试验证通过**

```powershell
go test ./internal/workflow/ -count=1
```
预期 PASS。

- [ ] **步骤 5：Commit**

```powershell
git add internal/workflow/
git commit -m "feat(workflow): 工作流模型与解析（线性流水线 v0）"
```

---

### 任务 2：模板渲染器

**文件：**
- 创建：`internal/workflow/template.go`、`internal/workflow/template_test.go`

- [ ] **步骤 1：编写失败的测试**

```go
package workflow_test

import (
	"reflect"
	"testing"

	"github.com/rebornace/baize/internal/workflow"
)

func tree() map[string]any {
	return map[string]any{
		"input": map[string]any{"text": "printer on fire"},
		"fetch": map[string]any{"result": map[string]any{
			"summary": "fire report",
			"urgent":  true,
			"count":   float64(3),
			"items":   []any{map[string]any{"id": "t1"}, map[string]any{"id": "t2"}},
			"meta":    map[string]any{"owner": map[string]any{"name": "bob"}},
		}},
	}
}

func TestRenderWholeValueKeepsType(t *testing.T) {
	tree := tree()
	got, ok := workflow.RenderArg("{{fetch.result.urgent}}", tree)
	if !ok || got != true {
		t.Fatalf("got=%v ok=%v", got, ok)
	}
	got, _ = workflow.RenderArg("{{fetch.result.count}}", tree)
	if got != float64(3) {
		t.Fatalf("got=%T %v", got, got)
	}
}

func TestRenderSubstringConcatenates(t *testing.T) {
	got, _ := workflow.RenderArg("ticket: {{fetch.result.count}}!", tree())
	if got != "ticket: 3!" {
		t.Fatalf("got=%#v", got)
	}
}

func TestRenderInputText(t *testing.T) {
	got, _ := workflow.RenderArg("{{input.text}}", tree())
	if got != "printer on fire" {
		t.Fatalf("got=%v", got)
	}
}

func TestRenderNestedAndIndex(t *testing.T) {
	got, _ := workflow.RenderArg("{{fetch.result.items.1.id}}", tree())
	if got != "t2" {
		t.Fatalf("got=%v", got)
	}
}

func TestRenderMissingPathIsError(t *testing.T) {
	_, ok := workflow.RenderArg("{{fetch.result.summary.x}}", tree())
	if ok {
		t.Fatal("want not-found")
	}
	_, ok = workflow.RenderArg("{{nope.result}}", tree())
	if ok {
		t.Fatal("want not-found")
	}
}

func TestRenderMapRecursive(t *testing.T) {
	in := map[string]any{
		"a": "{{fetch.result.summary}}",
		"b": []any{"{{input.text}}", 7},
	}
	got := workflow.RenderArgs(in, tree())
	want := map[string]any{"a": "fire report", "b": []any{"printer on fire", 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%#v", got)
	}
}

func TestPlaceholderRegexOnlyMatchesFullOrPart(t *testing.T) {
	cases := map[string]string{
		"{{fetch.result.summary}}x": "fire reportx", // 子串拼接
	}
	for in, want := range cases {
		got, _ := workflow.RenderArg(in, tree())
		if got != want {
			t.Fatalf("%s => %#v", in, got)
		}
	}
}
```

- [ ] **步骤 2：运行验证失败**

```powershell
go test ./internal/workflow/ -run TestRender -count=1
```
预期：undefined `RenderArg` 等。

- [ ] **步骤 3：实现**

```go
package workflow

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var phRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+?)\s*\}\}`)

// Resolve walks dot path over the data tree. Returns (value, found).
func Resolve(tree map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = tree
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if ok {
			cur, ok = m[p]
			if !ok {
				return nil, false
			}
			continue
		}
		s, ok := cur.([]any)
		if ok {
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(s) {
				return nil, false
			}
			cur = s[idx]
			continue
		}
		return nil, false
	}
	return cur, true
}

// RenderArg renders one scalar arg. full=true means the placeholder was the
// entire value and resolved to a non-string — caller keeps native type.
func RenderArg(v any, tree map[string]any) (any, bool) {
	s, isStr := v.(string)
	if !isStr {
		return v, true
	}
	loc := phRe.FindStringSubmatchIndex(s)
	if loc == nil {
		return v, true
	}
	expr := s[loc[2]:loc[3]]
	val, found := Resolve(tree, expr)
	if !found {
		return nil, false
	}
	full := loc[0] == 0 && loc[1] == len(s)
	if full {
		return val, true // 保类型
	}
	out := s[:loc[0]]
	switch n := val.(type) {
	case string:
		out += n
	default:
		out += fmt.Sprintf("%v", n)
	}
	return out + s[loc[1]:], true
}

// RenderArgs deep-renders maps and slices. Missing path yields error.
func RenderArgs(args map[string]any, tree map[string]any) map[string]any {
	out := renderAny(args, tree).(map[string]any)
	return out
}

func renderAny(v any, tree map[string]any) any {
	switch n := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(n))
		for k, vv := range n {
			m[k] = renderAny(vv, tree)
		}
		return m
	case []any:
		s := make([]any, len(n))
		for i, vv := range n {
			s[i] = renderAny(vv, tree)
		}
		return s
	default:
		r, ok := RenderArg(n, tree)
		if !ok {
			panic(fmt.Sprintf("path %v not found", n))
		}
		return r
	}
}

// TryRenderArgs returns rendered args or an error naming the offending value.
func TryRenderArgs(args map[string]any, tree map[string]any) (map[string]any, error) {
	var bad error
	var walk func(any) any
	walk = func(v any) any {
		switch n := v.(type) {
		case map[string]any:
			m := make(map[string]any, len(n))
			for k, vv := range n {
				m[k] = walk(vv)
			}
			return m
		case []any:
			s := make([]any, len(n))
			for i, vv := range n {
				s[i] = walk(vv)
			}
			return s
		default:
			r, ok := RenderArg(n, tree)
			if !ok {
				if bad == nil {
					bad = fmt.Errorf("template reference not found: %v", n)
				}
				return nil
			}
			return r
		}
	}
	out := walk(args).(map[string]any)
	return out, bad
}
```

（注：`RenderArgs` 可保留给测试用；执行路径只允许 `TryRenderArgs`。）

- [ ] **步骤 4：运行测试验证通过**

```powershell
go test ./internal/workflow/ -count=1
```

- [ ] **步骤 5：Commit**

```powershell
git add internal/workflow/template.go internal/workflow/template_test.go
git commit -m "feat(workflow): {{路径}} 模板渲染器（整值保类型/子串拼接/递归/fail-fast）"
```

---

### 任务 3：Catalog 加载 workflow.yaml

**文件：**
- 修改：`internal/skill/package.go`
- 修改：`internal/skill/catalog.go`（scanDir）
- 测试：`internal/skill/catalog_test.go`（追加）

- [ ] **步骤 1：编写失败的测试（追加到 catalog_test.go）**

```go
func TestLoadCatalogReadsWorkflowYAML(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "pipe")
	os.MkdirAll(pkgDir, 0o755)
	os.WriteFile(filepath.Join(pkgDir, "SKILL.md"),
		[]byte("---\nname: pipe\ntools:\n  - t\n---\nbody"), 0o644)
	os.WriteFile(filepath.Join(pkgDir, "workflow.yaml"),
		[]byte("name: pipe\nsteps:\n  - id: a\n    tool: t\n"), 0o644)

	cat, err := LoadCatalog([]string{dir}, "")
	if err != nil { t.Fatalf("LoadCatalog: %v", err) }
	p := cat.Get("pipe")
	if p == nil { t.Fatal("pkg missing") }
	if p.Workflow == nil || p.Workflow.Name != "pipe" || len(p.Workflow.Steps) != 1 {
		t.Fatalf("wf=%+v", p.Workflow)
	}
}

func TestLoadCatalogInvalidWorkflowFails(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "bad")
	os.MkdirAll(pkgDir, 0o755)
	os.WriteFile(filepath.Join(pkgDir, "SKILL.md"), []byte("---\nname: bad\n---\nb"), 0o644)
	os.WriteFile(filepath.Join(pkgDir, "workflow.yaml"), []byte("name:\nsteps: []\n"), 0o644)
	if _, err := LoadCatalog([]string{dir}, ""); err == nil {
		t.Fatal("want invalid workflow to fail load")
	}
}
```

（沿用现有 catalog_test.go 的 import 风格。）

- [ ] **步骤 2：运行验证失败**

```powershell
go test ./internal/skill/ -run TestLoadCatalogReadsWorkflow -count=1
```

- [ ] **步骤 3：实现**

`package.go`：Package 增加字段

```go
type Package struct {
	ID          string
	Name        string
	Description string
	Tools       []string
	Body        string
	Source      string
	Dir         string
	Workflow    *workflow.Workflow // optional pipeline from workflow.yaml
}
```

（新增 import `"github.com/rebornace/baize/internal/workflow"`；确认无循环依赖——workflow 不 import skill。）

`catalog.go` scanDir：填完 pkg.Dir 后追加

```go
wfRaw, wfErr := os.ReadFile(filepath.Join(pkg.Dir, "workflow.yaml"))
if wfErr == nil {
	wf, perr := workflow.Parse(wfRaw)
	if perr != nil {
		return fmt.Errorf("skill %s: %w", pkg.ID, perr)
	}
	if wf.Name != "" && wf.Name != pkg.ID && source == "builtin" {
		log.Printf("[skill] workflow name %q != package id %q", wf.Name, pkg.ID)
	}
	pkg.Workflow = wf
}
```

（对齐现有 name≠id warning 行为：仅告警不报错；但 Parse 校验错误必须让加载失败。）

- [ ] **步骤 4：运行验证通过**

```powershell
go test ./internal/skill/ ./internal/workflow/ -count=1
```

- [ ] **步骤 5：Commit**

```powershell
git add internal/skill/package.go internal/skill/catalog.go internal/skill/catalog_test.go
git commit -m "feat(skill): Catalog 加载 workflow.yaml 挂载到 Package"
```

---

### 任务 4：引擎接线与执行器（最大任务）

**文件：**
- 创建：`internal/workflow/exec.go`、`internal/workflow/exec_test.go`
- 修改：`internal/run/engine.go`（激活特判 + runWorkflow + 冷恢复）
- 修改：`internal/run/skills_overlay.go`（runSkillState 两字段）

**执行器接口（engine 只依赖它）：**

```go
package workflow

type InvokeFunc func(ctx context.Context, tool string, args map[string]any) (map[string]any, bool, error)

type EmitFunc func(typ string, data map[string]any) error // 包装 store.AppendEvent

type GateFunc func(ctx context.Context, payload store.HITLPayload) (approved bool, err error) // engine 提供 awaitHITL 闭包

type ExecHooks struct {
	Emit   EmitFunc
	Gate   GateFunc
	Invoke InvokeFunc
}

// Run executes linearly; returns final content if caller needs it.
func (w *Workflow) Run(ctx context.Context, tree map[string]any, hooks ExecHooks) error
```

- [ ] **步骤 1：编写失败的执行器测试**

`exec_test.go` 核心 4 例：

```go
func TestRunExecutesLinearlyWithEvents(t *testing.T) {
	w, _ := workflow.Parse([]byte("name: n\nsteps:\n  - id: a\n    tool: ta\n    args:\n      q: \"{{input.text}}\"\n  - id: b\n    tool: tb\n    args:\n      x: \"{{a.result.ok}}\"\n"))
	var calls []string
	hooks := workflow.ExecHooks{
		Emit: func(typ string, d map[string]any) error { calls = append(calls, typ); return nil },
		Invoke: func(ctx context.Context, tool string, args map[string]any) (map[string]any, bool, error) {
			calls = append(calls, "invoke:"+tool)
			if tool == "ta" {
				return map[string]any{"ok": true}, false, nil
			}
			return map[string]any{}, false, nil
		},
	}
	tree := map[string]any{"input": map[string]any{"text": "hi"}}
	if err := w.Run(context.Background(), tree, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"workflow.started", "workflow.step_started", "invoke:ta", "workflow.step_completed", "workflow.step_started", "invoke:tb", "workflow.step_completed"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v", calls)
	}
}

func TestRunMissingPathFails(t *testing.T) { /* b 步引 {{a.result.nope}} → Run 返回 err 含 `step "b"` 与路径 */ }

func TestRunApproveRejectStops(t *testing.T) { /* step approve:true，Gate 返回 approved=false → err 含 rejected 且后续不再 invoke */ }

func TestRunApproveApproveContinues(t *testing.T) { /* Gate 返回 true → 正常 invoke */
	/* Gate 收到的 payload 断言：Prompt 含 step id，ToolName=tool，Arguments=渲染后 */
}
```

- [ ] **步骤 2：运行验证失败**

```powershell
go test ./internal/workflow/ -run TestRun -count=1
```

- [ ] **步骤 3：实现 exec.go**

```go
package workflow

func (w *Workflow) Run(ctx context.Context, tree map[string]any, h ExecHooks) error {
	ids := make([]string, 0, len(w.Steps))
	for _, s := range w.Steps { ids = append(ids, s.ID) }
	_ = h.Emit("workflow.started", map[string]any{"skill": w.Name, "steps": ids})

	results := map[string]any{}
	for _, s := range w.Steps {
		args, rerr := TryRenderArgs(s.Args, tree)
		if rerr != nil {
			return fmt.Errorf("step %q: %w", s.ID, rerr)
		}
		if s.Approve {
			approved, err := h.Gate(ctx, store.HITLPayload{
				Prompt:    fmt.Sprintf("workflow step %q: %s", s.ID, s.Tool),
				ToolName:  s.Tool,
				Arguments: args,
			})
			if err != nil { return fmt.Errorf("step %q: %w", s.ID, err) }
			if !approved { return fmt.Errorf("workflow step %q rejected", s.ID) }
		}
		_ = h.Emit("workflow.step_started", map[string]any{"step": s.ID, "tool": s.Tool})
		content, isErr, ierr := h.Invoke(ctx, s.Tool, args)
		if ierr != nil || isErr {
			return fmt.Errorf("step %q: invoke failed: %v", s.ID, firstErr(ierr, isErr))
		}
		results[s.ID+".result"] = content
		_ = h.Emit("workflow.step_completed", map[string]any{"step": s.ID, "is_error": false})
	}
	maps.Copy(tree, results) // 引用数据树原地扩展
	return nil
}
```

（store.HITLPayload 从 `github.com/rebornace/baize/internal/store` 导入；workflow→store 方向无环。）

- [ ] **步骤 4：engine 接线（改三处）**

`skills_overlay.go` runSkillState 增加：

```go
workflowStarted bool
workflowSkill   string
workflowResults map[string]any // 步骤结果树；key: "<id>.result"；另含 input
```

`beginRunSkills` 初始化 `workflowResults: map[string]any{"input": map[string]any{"text": <用户输入>}}`——注意 beginRunSkills 当前签名不含 input；将 input 透传改为在 ExecuteWithOpts 构造 messages 之后调用的新入口或给 beginRunSkills 增加 input 参数（保持编译内聚，实现者自行选择最小改动并在报告注明）。

`engine.go` 激活特判段（L416 区域）：`handleActivateSkill` 成功且未 workflowStarted 且首个新激活 skill 带 `Workflow != nil` → 置 workflowStarted/workflowSkill，随后：

```go
werr := wf.Run(ctx, st.workflowResults, workflow.ExecHooks{
	Emit: func(typ string, data map[string]any) error {
		return e.Store.AppendEvent(runID, store.Event{Type: typ, Data: data})
	},
	Gate: func(gctx context.Context, p store.HITLPayload) (bool, error) {
		// 复用 awaitHITL 的机制但不能复用其 llm.ToolCall 参数：
		// 提取 awaitHITL 内核为 awaitHITLPayload(ctx, runID, prompt, toolName, args) error(error=nil=approved)，
		// 原 awaitHITL 改为其薄包装；拒绝时返回 errRejected sentinel。
		if err := e.awaitHITLPayload(gctx, runID, p.Prompt, p.ToolName, p.Arguments); err != nil {
			return false, err
		}
		return true, nil
	},
	Invoke: func(ictx context.Context, tool string, args map[string]any) (map[string]any, bool, error) {
		return e.invokeTool(ictx, runID, tool, args) // 抽取 runLoop L470 附近 Invoke+EventLLMToolCall+EventToolResult 段为方法供两处共用
	},
})
```

- workflow.Run 返回 nil → AppendEvent `llm.message`（`content: "workflow completed"`）后 UpdateRun(succeeded)、发既有 run 终止事件，runLoop `return nil`（跳过后续 LLM.Chat）。
- 返回 err 且是 errRejected → UpdateRun(failed) + `EventLLMError` + return。
- 其他错误同理 failed。
- **后续轮次防线**：workflowStarted 置位后，runLoop 顶部（L384 前）检查已结束时不再进 Chat（由上面直接 return 保证即可，无需额外 flag）。

冷恢复 `ContinueFromHITL`：恢复时读 events，遇 `workflow.started` 重建 st.workflowStarted/Skill，遇每个 `workflow.step_completed` 无副作用（内容缺失）；批准继续的步骤由 HITL payload 的 Arguments 直接 Invoke（走同一 invokeTool），完成后**其余剩余步骤须重跑** —— 实现：ContinueFromHITL 检测 workflowStarted 时，从命中步骤之后的列表里调一个新的 `resumeWorkflowRemaining(runID, fromStep int)`，其中 fromStep 通过匹配 hitl.waiting 事件的 step 序号得出（waiting data 补充 `"step": id` 字段）。**此子需求若一次做不稳，允许降级：冷恢复对 workflow run 统一 fail-fast 并带清晰错误信息（`workflow run interrupted by restart; please re-run`），不做续跑** —— 实现者二选一，选降级须在报告中注明并同步规格偏差。

- [ ] **步骤 5：全量相关测试**

```powershell
go test ./internal/workflow/ ./internal/run/ ./internal/skill/ ./tests/integration/ -count=1
```

- [ ] **步骤 6：Commit**

```powershell
git add internal/workflow/exec.go internal/workflow/exec_test.go internal/run/engine.go internal/run/skills_overlay.go
git commit -m "feat(run): activate_skill 切换流水线模式并顺序执行（含 HITL 复用）"
```

---

### 任务 5：前端 foldEvents 三种 block

**文件：**
- 修改：`web/chat/src/foldEvents.ts`
- 测试：`web/chat/src/foldEvents.test.ts`

- [ ] **步骤 1：编写失败的测试**

```ts
it("folds workflow events into a progress block", () => {
	let blocks = fold([ev("workflow.started", { skill: "triage", steps: ["fetch", "reply"] })]);
	expect(blocks.at(-1)).toEqual({
		kind: "workflow",
		skill: "triage",
		steps: [
			{ id: "fetch", status: "pending" },
			{ id: "reply", status: "pending" },
		],
	} satisfies ChatBlock);

	blocks = fold([
		ev("workflow.started", { skill: "triage", steps: ["fetch", "reply"] }),
		ev("workflow.step_started", { step: "fetch", tool: "search_tickets" }),
		ev("workflow.step_completed", { step: "fetch", is_error: false }),
		ev("workflow.step_started", { step: "reply", tool: "reply_ticket" }),
	]);
	expect((blocks.at(-1) as any).steps[0].status).toBe("done");
	expect((blocks.at(-1) as any).steps[1].status).toBe("running");
});
```

（fold 为该测试文件现有的聚合辅助函数名；实际名以文件为准，如不同请随现有风格。）

- [ ] **步骤 2：运行验证失败**

```powershell
cd web\chat
npm test -- foldEvents
```

- [ ] **步骤 3：实现**

ChatBlock 增加：

```ts
| {
	kind: "workflow";
	skill: string;
	steps: { id: string; status: "pending" | "running" | "done" | "failed" }[];
	runId: string;
}
```

switch 增加三分支处理 started/step_started/step_completed（同 runId 下定位/更新最近 workflow block；is_error:true → failed）。

- [ ] **步骤 4：npm test 全过 + tsc 无错**

```powershell
npm test
npx tsc --noEmit
```

- [ ] **步骤 5：Commit**

```powershell
git add web/chat/src/foldEvents.ts web/chat/src/foldEvents.test.ts
git commit -m "feat(web): 折叠条渲染工作流步骤进度"
```

---

### 任务 6：示例与文档

**文件：**
- 创建：`examples/skills/ticket-triage/workflow.yaml`
- 修改：`README.md`、`README.zh-CN.md`（各一句）
- 修改：`docs/architecture-and-plugin-protocol.md`（§5 Agent 运行补小节）

- [ ] **步骤 1：示例 workflow**

```yaml
name: ticket-triage
steps:
  - id: list
    tool: list_tickets
    args:
      limit: 10
  - id: create
    tool: create_ticket
    approve: true
    args:
      title: "{{input.text}}"
```

（mock-ticket 工具参数名以 `examples/mock-ticket/openapi.yaml` 实际为准；不符则调整字段。）文件头注释一行：`# 需 ReAct 判断分支的场景请不用本文件，让 LLM 自行调度 tools`。

- [ ] **步骤 2：文档三处一句话/小节**

README（中英）：「Skill 包可附 `workflow.yaml` 定义确定性流水线：`{{input.text}}` 取用户输入、`{{<步骤id>.result.x}}` 取上一步结果；审批标 `approve: true`。」
架构 §5：小节「线性流水线（workflow.yaml）」包含语法表 + 执行模型两句话 + 指回 ReAct 的分工说明。

- [ ] **步骤 3：全量验证**

```powershell
go build ./...
go vet ./...
go test ./... -count=1
cd web\chat; npm test; npx tsc --noEmit
```
（本地 Linux 语义等同 CI 门禁；Windows 下 gofmt 已被 .gitattributes 锁 LF。）

- [ ] **步骤 4：Commit**

```powershell
git add examples/ README.md README.zh-CN.md docs/architecture-and-plugin-protocol.md
git commit -m "docs(examples): 工作流流水线示例与说明"
```

---

## 规格覆盖自检

| 规格需求 | 任务 |
|---------|------|
| §2 五字段语法 + KnownFields 锁面 | 1 |
| §3 渲染五规则 | 2 |
| Catalog 加载/校验失败传播 | 3 |
| §4 执行模型/模式锁定/结果树 | 4 |
| §5 HITL approve/reject + 冷恢复（或降级裁定） | 4 |
| §6 三个事件 + 前端 block | 4 + 5 |
| §7 API 零端点 | 无操作 |
| §9 示例/README/架构 | 6 |

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-27-workflow-pipeline-v0.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 必需子技能：`subagent-driven-development`。
**2. 内联执行** — 必需子技能：`executing-plans`。

**选哪种方式？**
