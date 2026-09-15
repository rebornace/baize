# CLEAN-AUDIT 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 只读盘点公开契约、结构热点、lint 基线与性能候选，产出经产品确认的白名单清单；**不改业务代码行为**。

**架构：** 全部产出写入私有 notes；用仓库内可复现命令生成路由表/体量/lint 计数；人工标注 `保留|重命名|删除`。本计划结束后须停住，等产品确认清单，再另开 CONTRACT 计划。

**技术栈：** PowerShell + ripgrep/`Select-String`、Go 1.25、可选 golangci-lint v2.4（与 CI 一致）、Node 22（eslint）。提交中文 Conventional Commits。Windows：`git commit -m "..."`。**禁止** `move_agent_to_root` / 抢 `main` 的 worktree。分支：`chore/clean-audit`（自 `main` 拉出）。

规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md) §4.1

**后续计划（本文件不做）：** CLEAN-CONTRACT / STRUCT / GATES / PERF-HOT 各自独立计划。

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `docs/superpowers/notes/2026-09-15-clean-audit.md` | 总清单：契约表、热点表、门禁基线、性能候选、明确不做、待产品确认栏 |
| `docs/superpowers/notes/2026-09-15-clean-audit-routes.txt` | 从 `server.go` 抽出的 `METHOD path` 原始列表（可机器再生成） |
| `docs/superpowers/specs/2026-09-15-oss-quality-cleanup-design.md` | 回链 AUDIT 计划；状态仍「待实现」至 AUDIT 确认 |
| `docs/superpowers/notes/2026-09-13-spec-ledger.md` | CLEAN 下一动作改为「清单待产品确认」 |

**硬约束：** 本计划 **禁止** 修改 `internal/**`、`web/**`、`.github/**`、公开 `docs/*.md`、`README*`（除私有 superpowers 与分支上的 notes）。

---

### 任务 1：开分支 + 清单骨架

**文件：**
- 创建：`docs/superpowers/notes/2026-09-15-clean-audit.md`
- 创建：`docs/superpowers/notes/2026-09-15-clean-audit-routes.txt`（可先空文件占位）

- [ ] **步骤 1：从 main 拉分支**

```powershell
cd C:\Users\Administrator\Desktop\baize
git fetch real
git checkout main
git pull real main
git checkout -b chore/clean-audit
```

预期：当前分支 `chore/clean-audit`。

- [ ] **步骤 2：写入清单骨架**

创建 `docs/superpowers/notes/2026-09-15-clean-audit.md`，内容必须含以下标题（正文可先写「（待填）」）：

```markdown
# CLEAN-AUDIT 盘点清单

> 日期：2026-09-15  
> 状态：**待产品确认**  
> 规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md)  
> 约束：本文件只读盘点结果；确认前不得开 CONTRACT 实现。

## 0. 产品确认栏

- [ ] 契约白名单已审阅
- [ ] 结构热点优先级已审阅
- [ ] 明确不做无异议
- [ ] 批准进入 CLEAN-CONTRACT

确认人 / 日期：

## 1. 契约白名单（HTTP）

| METHOD path | 处置 | 理由 |
|-------------|------|------|
| （待填） | 保留 | |

处置枚举：**保留** / **重命名** / **删除**。

## 2. 契约白名单（配置 / env / CLI）

| 键或命令 | 处置 | 理由 |
|----------|------|------|
| （待填） | 保留 | |

## 3. 契约白名单（Web 客户端 / 路由）

| 表面（api.ts 函数或 UI 路由） | 处置 | 理由 |
|------------------------------|------|------|
| （待填） | 保留 | |

## 4. 废弃 / 兼容 / 双写信号

| 位置 | 信号摘要 | 建议处置 | 理由 |
|------|----------|----------|------|
| （待填） | | 删除或保留 | |

## 5. 结构热点

| 路径 | 行数（约） | 建议拆法（一句话） | 本版优先级 |
|------|------------|--------------------|------------|
| （待填） | | | P0/P1/P2/保留 |

## 6. 门禁基线

| 工具 | 命令 | 问题数或退出码 | 备注 |
|------|------|----------------|------|
| golangci 全量 | | | |
| eslint | | | |
| gofmt -l | | | |

## 7. 性能候选（只标注，不测）

| 路径 | 为何值得测 | PERF-HOT 建议探针 |
|------|------------|-------------------|
| 聊天流式 `GET /v0/runs/{id}/stream` | | |
| 会话消息读写 | | |
| blob / artifacts | | |
| 渠道出站 | | |

## 8. 公开文档处置（CONTRACT 执行）

| 路径 | 建议 | 理由 |
|------|------|------|
| `README.md` / `README.zh-CN.md` | 重写为产品向 | 规格已定 |
| 新建开发者文档 | 新建 | 规格已定 |
| `docs/architecture-and-plugin-protocol.md` | 删除 | 规格已定 |
| `docs/deployment.md` | 删除 | 规格已定 |

## 9. 明确不做（本轮 CLEAN）

- 插件公共 Go SDK、OTel、Playwright、新功能史诗
- 强制覆盖率挡合并
- 无证据的性能改动、伪造竞品对比
- （AUDIT 中发现但决定不做的项追加于此）

## 10. 复现命令备忘

（任务 2–5 填入实际用过的命令）
```

- [ ] **步骤 3：Commit 骨架**

```powershell
git add docs/superpowers/notes/2026-09-15-clean-audit.md
git commit -m "docs: CLEAN-AUDIT 清单骨架"
```

---

### 任务 2：HTTP 路由全表

**文件：**
- 修改：`docs/superpowers/notes/2026-09-15-clean-audit-routes.txt`
- 修改：`docs/superpowers/notes/2026-09-15-clean-audit.md` §1

- [ ] **步骤 1：从 `internal/api/server.go` 抽出路由**

在仓库根执行（PowerShell）：

```powershell
Select-String -Path internal\api\server.go -Pattern 'HandleFunc\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"' |
  ForEach-Object { if ($_.Line -match 'HandleFunc\("((?:GET|POST|PUT|PATCH|DELETE) [^"]+)"') { $Matches[1] } } |
  Sort-Object -Unique |
  Set-Content -Encoding utf8 docs\superpowers\notes\2026-09-15-clean-audit-routes.txt

# 补上非 HandleFunc 的 MCP export 前缀（手写两行追加）
Add-Content docs\superpowers\notes\2026-09-15-clean-audit-routes.txt "HANDLE /v0/mcp/export"
Add-Content docs\superpowers\notes\2026-09-15-clean-audit-routes.txt "HANDLE /v0/mcp/export/"
Add-Content docs\superpowers\notes\2026-09-15-clean-audit-routes.txt "HANDLE /ui/"
```

预期：`clean-audit-routes.txt` 行数 roughly ≥ 80；含 `GET /healthz`、`POST /v0/runs`、`GET /v0/runs/{id}/stream`。

- [ ] **步骤 2：核对是否还有其它包注册 `/v0`**

```powershell
Select-String -Path internal\**\*.go,cmd\**\*.go -Pattern '"/v0/' -SimpleMatch:$false |
  Where-Object { $_.Path -notmatch '_test\.go$' -and $_.Line -match 'HandleFunc|Handle\(' } |
  Select-Object -First 30 Path,LineNumber,Line
```

若发现 `server.go` 以外的注册，追加进 `routes.txt` 并在清单 §10 注明来源文件。

- [ ] **步骤 3：填 §1 契约表**

将 `routes.txt` 每一行落入 `clean-audit.md` §1 表格。默认处置填 **保留**；仅当满足以下之一时标 **删除** 或 **重命名**（必须写理由）：

- 注释/文档标明 deprecated、legacy-only、或「勿用」  
- Web/测试已无任何调用，且与现行产品确认不做的能力相关（如 MCP capture UI）  
- 命名明显错误且零用户窗口值得改（标 **重命名**，写出建议新名）  

**不要**在本任务删除代码。

- [ ] **步骤 4：Commit**

```powershell
git add docs/superpowers/notes/2026-09-15-clean-audit-routes.txt docs/superpowers/notes/2026-09-15-clean-audit.md
git commit -m "docs: CLEAN-AUDIT HTTP 路由白名单初稿"
```

---

### 任务 3：配置 / env / CLI + 废弃信号

**文件：**
- 修改：`docs/superpowers/notes/2026-09-15-clean-audit.md` §2、§4

- [ ] **步骤 1：配置结构盘点**

阅读 `internal/config/config.go` 中 `type Config struct` 及其嵌套（含 `yaml:` 标签）。将顶层与重要嵌套键列入 §2，默认 **保留**。

同时扫环境变量约定：

```powershell
Select-String -Path internal\**\*.go,cmd\**\*.go,configs\**\* -Pattern 'BAIZE_|os\.Getenv|APIKeyEnv' |
  Where-Object { $_.Path -notmatch '_test\.go$' } |
  Select-Object -First 60 Path,LineNumber,Line
```

将稳定 env（如 `BAIZE_API_KEY`、`BAIZE_LISTEN`、`BAIZE_TEST_PG_DSN`、S3 相关）写入 §2。

- [ ] **步骤 2：CLI 盘点**

阅读 `cmd/baize/main.go`、`cmd/baize/settings_reset.go`、`cmd/weixin-adapter/main.go` 的 flag/子命令；逐条进 §2。

- [ ] **步骤 3：废弃 / 兼容信号**

```powershell
Select-String -Path internal\**\*.go,web\chat\src\**\*.{ts,tsx} -Pattern 'deprecated|Deprecated|legacy|back-compat|backward compatible|兼容|废弃|@deprecated' |
  Select-Object Path,LineNumber,Line |
  Out-File -Encoding utf8 docs\superpowers\notes\_tmp-debt-grep.txt
```

打开 `_tmp-debt-grep.txt`，将**产品契约相关**行转入 §4（跳过纯粹内部注释如「legacy single-instance behavior」若确认非对外契约——仍可记一条「内部保留」）。删临时文件：

```powershell
Remove-Item docs\superpowers\notes\_tmp-debt-grep.txt -ErrorAction SilentlyContinue
```

已知须点名写入 §4 的至少包括：

- `web/chat/src/pages/runtimeSettingsHelpers.ts` 中 `@deprecated` helper  
- `internal/store/sqlite.go` 的 `SQLite` 别名（标：内部 API，建议 STRUCT 时保留或改名——**不是** HTTP 契约）  
- `internal/store` / models 的 `disable_thinking` legacy 同步（标：配置/API 字段是否仍对外）  

- [ ] **步骤 4：Commit**

```powershell
git add docs/superpowers/notes/2026-09-15-clean-audit.md
git commit -m "docs: CLEAN-AUDIT 配置与废弃信号"
```

---

### 任务 4：Web 客户端表面 + 结构热点

**文件：**
- 修改：`docs/superpowers/notes/2026-09-15-clean-audit.md` §3、§5

- [ ] **步骤 1：api.ts 导出面**

```powershell
Select-String -Path web\chat\src\api.ts -Pattern '^export (async )?function |^export const ' |
  ForEach-Object { $_.Line.Trim() }
```

将每个导出函数/常量列入 §3；对照 §1 路由，标出「无后端路由」或「无 UI 调用」的候选 **删除**（须再 `Select-String` 全 `web/chat/src` 确认无引用后再标删除）。

- [ ] **步骤 2：UI 路由**

阅读 `web/chat/src` 中 `createBrowserRouter` / `Routes` / `path:` 定义（常见于 `main.tsx` 或 `App.tsx`），列入 §3。

- [ ] **步骤 3：体量热点**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"

# Go 包行数 Top 15
Get-ChildItem -Recurse -Filter '*.go' internal,cmd |
  Where-Object { $_.FullName -notmatch '\\vendor\\' } |
  Group-Object { $_.DirectoryName } |
  ForEach-Object {
    $lines = 0
    $_.Group | ForEach-Object { $lines += @(Get-Content $_.FullName).Count }
    [PSCustomObject]@{ Lines=$lines; Dir=$_.Name.Replace((Get-Location).Path + '\','') }
  } |
  Sort-Object Lines -Descending |
  Select-Object -First 15

# TS/TSX 单文件 Top 15
Get-ChildItem -Recurse -Include '*.ts','*.tsx' web\chat\src |
  Sort-Object { @(Get-Content $_.FullName).Count } -Descending |
  Select-Object -First 15 |
  ForEach-Object { "{0,5}  {1}" -f @(Get-Content $_.FullName).Count, $_.FullName.Replace((Get-Location).Path+'\','') }
```

将 Top 结果填入 §5；对每个 P0/P1 写一句话拆法（例：`ChatPage.tsx` → 消息列表 / 输入区 / 顶栏；`api.ts` → 按资源分模块 `api/runs.ts` 等）。`locales/zh.ts`/`en.ts` 体量大但属文案包，标 **保留（文案包）**，不拆除非 AUDIT 另有理由。

- [ ] **步骤 4：Commit**

```powershell
git add docs/superpowers/notes/2026-09-15-clean-audit.md
git commit -m "docs: CLEAN-AUDIT Web 表面与结构热点"
```

---

### 任务 5：门禁基线 + 性能候选 + 收口

**文件：**
- 修改：`docs/superpowers/notes/2026-09-15-clean-audit.md` §6、§7、§9、§10  
- 修改：规格与账本回链

- [ ] **步骤 1：gofmt / eslint 基线**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
$gofmt = gofmt -l .
"gofmt dirty files: $(($gofmt | Measure-Object -Line).Lines)"
$gofmt | Select-Object -First 20

Push-Location web\chat
npm ci
npm run lint 2>&1 | Tee-Object -Variable eslintOut
Pop-Location
# 将 eslint 退出码与告警摘要写入 §6
```

- [ ] **步骤 2：golangci 全量基线**

若本机无二进制，用与 CI 同版本的官方 action 等价方式：

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
# 安装到用户目录（若尚未安装）
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.4.0
$env:PATH = "$(go env GOPATH)\bin;$env:PATH"
golangci-lint run ./... --timeout=5m 2>&1 | Tee-Object docs\superpowers\notes\_tmp-golangci.txt
```

从输出统计 `issues` 数量（或「0 issues」）；写入 §6。若命令失败，§6 记失败原因与「CI 上用 only-new 故本地全量首次」。保留摘要后删临时文件或把**非密钥**摘要附录到清单末尾（勿提交巨型原始日志；可只保留计数 + Top 10 规则名）。

```powershell
# 示例：统计
Select-String -Path docs\superpowers\notes\_tmp-golangci.txt -Pattern '^\S+:\d+:\d+:' |
  Measure-Object |
  Select-Object -ExpandProperty Count
Remove-Item docs\superpowers\notes\_tmp-golangci.txt -ErrorAction SilentlyContinue
```

- [ ] **步骤 3：填 §7 性能候选**

对规格已列四条路径各写「为何值得测」与「建议探针」（例：本地 `go test -bench`、或脚本打 `POST /v0/runs` + 读 stream 耗时）。**不运行**重型压测。

- [ ] **步骤 4：§9 / §10 收口**

§9 追加本轮发现但不做的项。§10 粘贴本计划实际使用过的关键命令。

§0 保持「待产品确认」；**不要**自己勾选批准。

- [ ] **步骤 5：回链规格与账本**

在规格头「实现计划」行改为指向本计划文件；账本 CLEAN 行「下一动作」改为：清单见 `2026-09-15-clean-audit.md`，**待产品确认**后再开 CONTRACT 计划。

- [ ] **步骤 6：最终 Commit**

```powershell
git add docs/superpowers/notes/2026-09-15-clean-audit.md docs/superpowers/specs/2026-09-15-oss-quality-cleanup-design.md docs/superpowers/notes/2026-09-13-spec-ledger.md
git commit -m "docs: CLEAN-AUDIT 盘点收口待产品确认"
```

- [ ] **步骤 7：停住并请求确认**

向用户展示清单路径，明确：**在 §0 勾选批准前，不得开始 CONTRACT 编码。** 可选：`git push -u real HEAD`（仅当用户要求推送时）。

---

## 自检（对照规格 §4.1）

| 规格产出 | 本计划任务 |
|----------|------------|
| 契约白名单表 | 任务 2–4 → §1–3 |
| 结构热点表 | 任务 4 → §5 |
| 门禁基线 | 任务 5 → §6 |
| 性能候选只标注 | 任务 5 → §7 |
| 明确不做 | 任务 5 → §9 |
| 不改行为 | 硬约束 + 分支仅 docs |
| 确认后才 CONTRACT | 任务 5 步骤 7 |

无占位「待定」步骤；临时 grep 文件不提交。
