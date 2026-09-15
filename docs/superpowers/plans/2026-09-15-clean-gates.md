# CLEAN-GATES 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 用 CI **全量** golangci（去掉 `only-new-issues`）证明存量债清零；对齐开发者文档与 `.golangci.yml` 叙述；保持 gofmt/vet/test 与前端门禁。

**架构：** 先清零本地 `golangci-lint run ./...` 的全部 issue，再改 CI；最后更新文档。不新增功能、不上覆盖率门槛、不上 Playwright。

**技术栈：** golangci-lint **v2.4.0**（与 CI 一致）、Go 1.25、eslint。分支：`feat/clean-gates`。Windows：`git commit -m "..."`。

规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md) §4.4  
基线：AUDIT §6（约 **46** issues：errcheck 20 / staticcheck 22 / ineffassign 2 / unused 2）  
前置：**STRUCT-P1 已交付**（或产品在账本明示「P1 本版保留」后）。计划：[`2026-09-15-clean-struct-p1.md`](./2026-09-15-clean-struct-p1.md)  
后续：PERF-HOT 另开。

**硬约束：**

- 修复以「真问题」为准：errcheck 补错误处理或显式 `_ =`/`Close` 检查；禁止大面积 `//nolint` 糊弄  
- 允许少量 `//nolint:errcheck` **仅**在 defer Close 等惯用且注释说明原因处  
- 不改对外 API 语义；行为变更须有测试  
- 排除 `.superpowers` 噪声（产品源码门禁）

---

## 文件结构

| 文件 | 职责 |
|------|------|
| 各 `internal/**`、`cmd/**` 被 lint 命中处 | 逐条修复 |
| `.github/workflows/ci.yml` | 去掉 `only-new-issues: true` |
| `.golangci.yml` | 更新头部注释（去掉「全仓清债不在本刀」） |
| `docs/developers/getting-started.md` | CI/本地命令与「全量 golangci」一致 |
| 账本 / 规格 | GATES 已交付 |

---

### 任务 1：复现基线并建修复清单

**文件：** 创建 `docs/superpowers/notes/2026-09-15-clean-gates-baseline.md`（私有，不进 public）

- [ ] **步骤 1：安装并跑全量**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$(go env GOPATH)\bin;$env:PATH"
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0
golangci-lint run ./... --timeout=5m 2>&1 | Tee-Object -FilePath $env:TEMP\golangci-gates.txt
```

- [ ] **步骤 2：汇总**

统计总 issue 数、按 linter 分类、按文件 Top 10。写入 `2026-09-15-clean-gates-baseline.md`（**不要**提交巨型原始日志；可附 Top 列表）。

- [ ] **步骤 3：Commit**

```powershell
git checkout -b feat/clean-gates
git add docs/superpowers/notes/2026-09-15-clean-gates-baseline.md
git commit -m "docs: CLEAN-GATES golangci 全量基线清单"
```

---

### 任务 2：清零 golangci issues

**文件：** 基线清单中的 Go 源文件；测试：`go test` 相关包 + 最终全量 lint

按 linter 分批（建议顺序）：

1. **unused / ineffassign**（通常少）  
2. **staticcheck**（S/QF/SA/ST…）  
3. **errcheck**（最多）

- [ ] **步骤 1：unused + ineffassign**

修完 → 

```powershell
go test ./... -count=1
# 若过慢：至少测改动包 + ./internal/api/ ./internal/store/ ./internal/run/
golangci-lint run ./... --timeout=5m
```

Commit：`fix: 清理 unused/ineffassign 存量 lint`

- [ ] **步骤 2：staticcheck**

同样：改 → 测 → 全量 lint 计数下降 → Commit：`fix: 清理 staticcheck 存量告警`

- [ ] **步骤 3：errcheck**

对 `Close`/`Remove`/`Encode` 等：优先处理错误；defer 场景用：

```go
defer func() { _ = f.Close() }()
// 或
defer func() {
  if err := f.Close(); err != nil {
    // 仅当有合理日志/返回路径时记录
  }
}()
```

禁止无注释的大范围 nolint。

Commit：`fix: 清理 errcheck 存量告警`

- [ ] **步骤 4：验收 0 issues**

```powershell
golangci-lint run ./... --timeout=5m
echo $LASTEXITCODE   # 预期 0
```

若仍有：继续修，不得进入任务 3。

---

### 任务 3：拧紧 CI + 配置注释

**文件：** `.github/workflows/ci.yml`、`.golangci.yml`

- [ ] **步骤 1：改 CI**

删除：

```yaml
only-new-issues: true
```

保留 `golangci-lint-action` 与 `version: v2.4.0`、`args: --timeout=5m`。

- [ ] **步骤 2：改 `.golangci.yml` 头注释**

改为说明：规则集为稳妥默认；**全仓必须通过**；与 F-C1 历史说明可保留一句但去掉「清债不在本刀 / only-new」。

- [ ] **步骤 3：Commit**

```powershell
git add .github/workflows/ci.yml .golangci.yml
git commit -m "ci: golangci 改为全量检查并更新配置说明"
```

---

### 任务 4：前端豁免清扫 + 开发者文档

**文件：** `web/chat` 中 `eslint-disable` / `as any`；`docs/developers/getting-started.md`

- [ ] **步骤 1：盘点**

```powershell
Select-String -Path web\chat\src -Pattern 'eslint-disable|as any|@ts-expect-error|@ts-ignore' -Recurse |
  Select-Object Path,LineNumber,Line
```

- [ ] **步骤 2：能删则删**

测试里不可避免的 `as any`：改为更窄类型或保留并注释原因。目标：生产 `src`（非 test）尽量 0；test 可少量。

```powershell
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
```

- [ ] **步骤 3：文档**

`getting-started.md` 写明：

```markdown
### Lint（Go）

本地须与 CI 一致跑全量：

go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0
golangci-lint run ./... --timeout=5m
```

- [ ] **步骤 4：Commit**

```powershell
git commit -m "chore: 收紧前端类型豁免并文档化全量 golangci"
```

---

### 任务 5：账本收口与停住

- [ ] **步骤 1：全量回归**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$(go env GOPATH)\bin;$env:PATH"
golangci-lint run ./... --timeout=5m
go test ./internal/api/ ./internal/store/ ./internal/run/ ./cmd/baize/ -count=1
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
```

- [ ] **步骤 2：账本** — GATES **已交付**；下一动作 **PERF-HOT**  
- [ ] **步骤 3：规格头** 更新 GATES 状态  
- [ ] **步骤 4：Commit** `docs: CLEAN-GATES 交付与账本收口`  
- [ ] **步骤 5：停住** — 提示合入后看 CI 是否全绿；再开 PERF-HOT

---

## 自检（对照规格 §4.4）

| 需求 | 任务 |
|------|------|
| 去掉 only-new；全量过 | 2–3 |
| gofmt/vet/test/前端门禁 | 2、4、5 |
| 清理不合理豁免 | 4 |
| 更新 golangci 叙述 | 3 |
| 开发者文档命令一致 | 4 |
| 不上覆盖率/Playwright | 头部 |
| STRUCT 后再拧 | 前置说明 |

无占位步骤。
