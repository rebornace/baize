# GitHub Actions CI v0 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 建立 `.github/workflows/ci.yml`：push main / PR main 触发，`go` 与 `web` 两个并行 job 跑 vet/gofmt/build/test 与 vitest/tsc，双仓共用。

**架构：** 单文件 workflow；Go job 用 setup-go@v5 缓存模块；web job 在 `web/chat` 用 setup-node@v4 + npm ci 缓存；`.github` 不进 export 排除列表，随干净树同步到 public 仓；README 中英加 public Actions 徽章。

**技术栈：** Go 1.25.0（纯 Go sqlite，免 CGO）、Node 22 LTS、npm ci、vitest run、tsc --noEmit、ubuntu-latest。

**规格：** `docs/superpowers/specs/2026-08-27-github-actions-ci-v0-design.md`

**Git：** 建议分支 `feat/github-actions-ci-v0`（本计划改动小，可经用户同意后直接在 main 上做——由执行者按 finishing-a-development-branch 流程与用户确认）。

---

## 文件结构

| 路径 | 职责 |
|------|------|
| `.github/workflows/ci.yml` | 唯一 workflow：触发规则 + go/web 两 job |
| `README.md` | 顶部加 CI 徽章一行 |
| `README.zh-CN.md` | 同上 |

不修改：`scripts/export-public.ps1`（`.github` 本就不在排除列表，导出自动带上）。

已核实事实（编写时确认）：

- go.mod：`module github.com/rebornace/baize`，`go 1.25.0`
- `web/chat/package.json` scripts：`build: "tsc && vite build"`、`test: "vitest run"`；npm 有 package-lock.json
- 测试无外部依赖：sqlite 纯 Go（modernc.org/sqlite），全部 `t.TempDir()`；configs/*.yaml 已提交
- `internal/ui/dist` 已提交入库，Go 编译/测试无需先构建前端
- vitest 测试文件 19 个（`src/**/*.test.ts(x)`），非 watch 模式（`vitest run`）

---

### 任务 1：创建 workflow

**文件：**
- 创建：`.github/workflows/ci.yml`

- [ ] **步骤 1：写入完整 workflow 内容**

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  go:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25.0"
          cache: true
          cache-dependency-path: go.sum
      - name: Vet
        run: go vet ./...
      - name: Format check
        run: test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)
      - name: Build
        run: go build ./...
      - name: Test
        run: go test -count=1 ./...

  web:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web/chat
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: "22"
          cache: npm
          cache-dependency-path: web/chat/package-lock.json
      - name: Install
        run: npm ci
      - name: Test
        run: npm test
      - name: Typecheck
        run: npx tsc --noEmit
```

要点：
- go/web 两 job 并行（无 needs 依赖）
- gofmt 步骤先打印不合格清单再失败，便于定位
- 不设 GOPROXY / timeout-minutes（规格 §2/§3）

- [ ] **步骤 2：本地静态校验**

PowerShell 校验 YAML 可解析（有 python 时）：

```powershell
python -c "import yaml,io;yaml.safe_load(io.open('.github/workflows/ci.yml',encoding='utf-8'));print('yaml ok')"
```

预期输出 `yaml ok`。（无 python 则人工核对缩进两遍。）

- [ ] **步骤 3：本地预演 go 侧门禁（确保首跑不会红在存量问题）**

```powershell
$env:Path = "C:\Users\Administrator\go\pkg\mod\golang.org\toolchain@v0.0.1-go1.25.0.windows-amd64\bin;D:\Git\bin;" + $env:Path
$env:GOTOOLCHAIN = "local"
$env:GOPROXY = "https://goproxy.cn,direct"
cd C:\Users\Administrator\Desktop\baize
go vet ./...
gofmt -l .
go build ./...
```

预期：vet 零输出、`gofmt -l .` 无清单、build 成功。
若 vet/gofmt 报存量问题：就地修复并纳入同一 commit（修复内容随步骤 4 提交）。

- [ ] **步骤 4：本地预演 web 侧门禁**

```powershell
cd C:\Users\Administrator\Desktop\baize\web\chat
npm ci
npm test
npx tsc --noEmit
```

预期：19+ 个测试文件全绿、tsc 无错。
若有存量测试/tsc 失败：停下上报，单独裁定修法后再继续。

- [ ] **步骤 5：Commit**

```powershell
cd C:\Users\Administrator\Desktop\baize
git add .github/workflows/ci.yml
git add -u
git commit -m "ci: 新增 GitHub Actions workflow（go vet/fmt/build/test + web vitest/tsc）"
```

（若步骤 3 有存量修复，commit message 追加一句「含存量 vet/gofmt 修复」。）

---

### 任务 2：README 徽章

**文件：**
- 修改：`README.md`（顶部第一行前插入）
- 修改：`README.zh-CN.md`（顶部第一行前插入）

- [ ] **步骤 1：插入徽章**

两个文件的标题（首个 `# ...` 行）之前加入：

```markdown
[![CI](https://github.com/rebornace/baize/actions/workflows/ci.yml/badge.svg)](https://github.com/rebornace/baize/actions/workflows/ci.yml)
```

注意：URL 指向 **public 仓** rebornace/baize（real 私有仓不加徽章）。

- [ ] **步骤 2：验证**

人工检查渲染：徽章 markdown 与现有 README 结构无冲突（首行即为徽章）。

- [ ] **步骤 3：Commit**

```powershell
git add README.md README.zh-CN.md
git commit -m "docs: README 增加 CI 徽章"
```

---

## 规格覆盖自检

| 规格需求 | 任务 |
|---------|------|
| 触发 push/PR main | 1 |
| go job: vet/gofmt/build/test（缓存、锁版本） | 1 |
| web job: npm ci/vitest/tsc（缓存） | 1 |
| 双仓共用 workflow（不改排除列表） | 无操作（已核实默认满足） |
| README 中英徽章指向 public | 2 |
| 存量 vet/gofmt/测试预案 | 1 步骤 3/4 |

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-27-github-actions-ci-v0.md`。两种执行方式：

**1. 子代理驱动（推荐）** — 每个任务一个新子代理 + 任务间审查。必需子技能：`subagent-driven-development`。

**2. 内联执行** — 当前会话用 `executing-plans` 批量执行。必需子技能：`executing-plans`。

**选哪种方式？**
