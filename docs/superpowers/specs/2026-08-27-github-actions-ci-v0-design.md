# GitHub Actions CI v0 设计规格

> 状态：已批准（2026-08-27）
> 范围：为 baize（Go 后端 + web/chat React 前端）建立首个自动化 CI 门禁，双仓（real 私有仓 / public 开源仓）共用同一份 workflow。

## 1. 目标与非目标

**目标**

1. `push` 到 `main` 与指向 `main` 的 `pull_request` 自动触发检查。
2. Go 侧：`go vet`、`gofmt` 格式守门、全量编译、全量测试。
3. 前端侧：vitest 单测 + TypeScript 类型检查。
4. 双仓均启用 CI；README（中英）加 public 徽章。

**非目标（v0 明确不做）**

- 多 OS 矩阵（Windows/macOS runner）——sqlite 为纯 Go，无平台差异需求。
- Docker 镜像构建/发布、release 自动化。
- 前端 `npm run build`（产物写入 `internal/ui/dist` 会污染工作树，CI 无消费方）。
- lint 工具引入（golangci-lint、eslint 等，留待后续版本）。

## 2. 触发与运行环境

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
```

- Runner：`ubuntu-latest`，单一矩阵。
- 不设 `timeout-minutes` 特殊预算（首跑观察后再调）。

## 3. Go job

| 步骤 | 命令 | 说明 |
|------|------|------|
| checkout | `actions/checkout@v4` | — |
| setup-go | `actions/setup-go@v5`, `go-version: "1.25.0"`, `cache: true`, `cache-dependency-path: go.sum` | 与 go.mod 锁同版本；官方缓存 GOMODCACHE/GOCACHE |
| vet | `go vet ./...` | 失败即红 |
| fmt | `test -z "$(gofmt -l .)"` | 存量未格式化文件需在本提交内一次性格式化修复 |
| build | `go build ./...` | 覆盖 cmd、examples 全量编译 |
| test | `go test -count=1 ./...` | `-count=1` 绕过测试缓存防假绿 |

事实依据：

- `modernc.org/sqlite` 纯 Go → Linux 免 CGO/gcc。
- 测试全部使用 `t.TempDir()` + 自设环境变量，无外部服务/DB 依赖；仓库内 `configs/*.yaml` 已提交可相对路径读取。
- 不设置 `GOPROXY`（CI 在海外走默认 proxy.golang.org；Dockerfile 的 goproxy.cn 仅为本机构建镜像用）。

## 4. 前端 job（web/chat）

```yaml
web:
  runs-on: ubuntu-latest
  defaults: { run: { working-directory: web/chat } }
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-node@v4
      with:
        node-version: "22"
        cache: npm
        cache-dependency-path: web/chat/package-lock.json
    - run: npm ci
    - run: npm test            # vitest run
    - run: npx tsc --noEmit
```

- Node 22 LTS 满足 vite engines `^20.19 || ^22.12 || >=24`；包管理器为 npm（存在 package-lock.json）。
- Go job 与 web job **并行执行**。

## 5. 双仓策略

- `.github/workflows/ci.yml` **不**加入 `scripts/export-public.ps1` 排除列表 → 每次导出随干净树进入 public 仓 → real 与 public 各自运行同一份 workflow。
- README.md / README.zh-CN.md 顶部添加 Actions 徽章，URL 指向 **public 仓**（rebornace/baize）；real 私有仓不加徽章但同样跑 CI。

## 6. 验收标准

1. `go vet ./...` 零输出；`gofmt -l .` 清单为空。
2. `go test -count=1 ./...` 全绿。
3. web/chat vitest 全绿且 `tsc --noEmit` 通过。
4. PR 页面 go/web 两个 job 均 ✅，总时长 ≤ 5 分钟。
5. README 中英徽章渲染正常并链接到 public 仓 Actions。
6. real 与 public push 后各自 Actions 页有运行记录。

## 7. 风险与对策

| 风险 | 对策 |
|------|------|
| `go vet` 报存量问题 | 首跑逐条修掉；不允许全局跳过或移除 vet 步骤 |
| gofmt 存量未格式化 | 本提交一次性格式化纳入 |
| vitest 慢/超时 | Node+npm 缓存先行；观察后必要时调 timeout 或 sharding |

## 8. 文件清单

| 文件 | 动作 |
|------|------|
| `.github/workflows/ci.yml` | 新建 |
| `README.md` / `README.zh-CN.md` | 加徽章一行 |
| （如首跑红）存量 vet/gofmt 修复 | 就地修 |
