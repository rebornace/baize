# CLEAN-CONTRACT 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 按已批准的 AUDIT 白名单清理对外死表面，并把公开文档拆成「产品向 README」与「开发者文档」；删除过时公开 docs。

**架构：** HTTP `/v0` 全部保留（零路由删除）。仅删除前端无引用客户端导出；公开仓删除 `architecture-and-plugin-protocol.md` / `deployment.md`，内容按真实现状重写进 `docs/developers/`；README 中/英改为产品/运营可读，性能数字区留 PERF-HOT 占位。

**技术栈：** TypeScript（`web/chat`）、Markdown 公开文档、Go 测试回归（本阶段原则上不改 Go）。分支：`feat/clean-contract`（自 `main`）。Windows：`git commit -m "..."`。**禁止** `move_agent_to_root`。双仓：实现合入后由用户要求再推。

规格：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md) §4.2  
清单（已批准）：[`../notes/2026-09-15-clean-audit.md`](../notes/2026-09-15-clean-audit.md)

**本阶段明确不做：** STRUCT 拆大文件；去掉 golangci `only-new`；PERF 测速与 README 数字；删 HTTP 路由；删 AUDIT 标「保留」的 `@deprecated` helper（`MAIN_KNOB_FIELDS` 等留给 STRUCT/后续）；改 `disable_thinking` 字段语义。

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `web/chat/src/api.ts` | 删除 `patchToolRequireLogin`、`clearMessages` |
| `web/chat/src/**/*.test.ts(x)` | 若有引用上述导出则改测或删断言 |
| `CHANGELOG.md`（无则创建） | Breaking：前端客户端删除两项导出 |
| `docs/developers/README.md` | 开发者入口（目录） |
| `docs/developers/getting-started.md` | 构建、跑测、贡献、CI 命令 |
| `docs/developers/configuration.md` | 配置键 / 重要 env / CLI（来自 AUDIT §2 + 真码） |
| `docs/developers/http-api.md` | `/v0` 控制面鸟瞰（链到 routes 列表思路，不抄私有 notes） |
| `docs/developers/architecture.md` | 五抽象 + 运行时边界（重写自旧 architecture，去掉草案/未实现 SDK/OTel 承诺） |
| `docs/developers/deployment.md` | 部署：单进程、微信适配器两种模式（重写自旧 deployment） |
| `README.md` / `README.zh-CN.md` | 产品向重写；试用入口；链到 `docs/developers/` |
| `docs/architecture-and-plugin-protocol.md` | **删除** |
| `docs/deployment.md` | **删除** |
| 公开仓内其它 md 链到旧 docs 的链接 | 改为新路径 |
| 账本 / 规格头 | CONTRACT 进行中 → 交付后已交付 |

---

### 任务 1：删除死客户端导出

**文件：**
- 修改：`web/chat/src/api.ts`（约 L928 `patchToolRequireLogin`、约 L1248 `clearMessages`）
- 修改：任何仍 import 二者的测试（预期无）
- 创建或修改：`CHANGELOG.md`

- [ ] **步骤 1：确认无引用**

```powershell
Select-String -Path web\chat\src -Pattern 'patchToolRequireLogin|clearMessages' -Recurse |
  Select-Object Path,LineNumber,Line
```

预期：仅 `api.ts` 定义行（或测试若误引用则记下待改）。

- [ ] **步骤 2：删除两个函数**

从 `api.ts` 删除：

```typescript
// 删除整个函数 patchToolRequireLogin(...)
// 删除整个函数 clearMessages(...)
```

保留后端仍存在的 `DELETE /v0/conversations/{id}/messages`（无 Web 封装即可）。保留 `patchTool`。

- [ ] **步骤 3：前端回归**

```powershell
Push-Location web\chat
npm ci
npm run lint
npm test
npx tsc --noEmit
Pop-Location
```

预期：全绿。

- [ ] **步骤 4：CHANGELOG Breaking**

若无 `CHANGELOG.md`，在仓库根创建；写入：

```markdown
# Changelog

## Unreleased

### Breaking

- Web 客户端（`web/chat/src/api.ts`）移除未使用导出 `patchToolRequireLogin`、`clearMessages`。请改用 `patchTool`；清空消息若需可直接调用 `DELETE /v0/conversations/{id}/messages`（当前 UI 未提供封装）。
```

- [ ] **步骤 5：Commit**

```powershell
git checkout -b feat/clean-contract   # 若已在该分支则跳过
git add web/chat/src/api.ts CHANGELOG.md
git commit -m "refactor(ui): 删除无引用 api 导出 patchToolRequireLogin/clearMessages"
```

---

### 任务 2：新建开发者文档（重写架构与部署）

**文件：**
- 创建：`docs/developers/README.md`
- 创建：`docs/developers/getting-started.md`
- 创建：`docs/developers/configuration.md`
- 创建：`docs/developers/http-api.md`
- 创建：`docs/developers/architecture.md`
- 创建：`docs/developers/deployment.md`

- [ ] **步骤 1：入口 README**

`docs/developers/README.md`：

```markdown
# 开发者文档

面向贡献者与集成方。产品介绍与试用见仓库根目录 [README](../../README.zh-CN.md)（英文 [README.md](../../README.md)）。

| 文档 | 内容 |
|------|------|
| [getting-started](./getting-started.md) | 构建、测试、CI、贡献 |
| [configuration](./configuration.md) | YAML / 环境变量 / CLI |
| [http-api](./http-api.md) | 控制面 HTTP 鸟瞰 |
| [architecture](./architecture.md) | 运行时边界与核心抽象 |
| [deployment](./deployment.md) | 单机与微信适配器部署 |
```

- [ ] **步骤 2：getting-started**

写明（与 CI 一致）：

- Go **1.25.0**（以 `.github/workflows/ci.yml` 为准；勿再写过时的 1.22 若 CI 已是 1.25）
- `go test ./...` / `go vet` / `gofmt`
- `web/chat`：`npm ci`、`npm run lint`、`npm test`、`npx tsc --noEmit`
- 试用：`.\demo.cmd` / `./scripts/demo.sh`
- 贡献：PR 对 `main`；私有过程文档在 `docs/superpowers`（不进公开仓）——**公开仓版本文档删除「superpowers」句**，改为「设计讨论通过 Issue/PR」

- [ ] **步骤 3：configuration**

从 `internal/config/config.go` 与 AUDIT §2 整理**现行**键：`listen`、`store`、`llm`、`skills`、`agent`、`connector`、`run`、`conversation`、`channels`（声明式；省略时 legacy 仅 auto-wire 非 `DeclarativeOnly`）、重要 env（`BAIZE_API_KEY`、`BAIZE_LISTEN`、S3 等）。**不要**复制私有 AUDIT 全文。

- [ ] **步骤 4：http-api**

说明控制面前缀 `/v0`、鉴权（控制面口令）、主要资源组：agents / connectors / tools / skills / runs（含 stream）/ conversations / settings / mcp-export / channels inbound。指向「以 `internal/api/server.go` 的 `HandleFunc` 注册为准」。不要声称有独立 OpenAPI 规范文件 unless 仓库真有。

- [ ] **步骤 5：architecture（重写）**

基于旧 `docs/architecture-and-plugin-protocol.md` **删减**后重写：

- 保留：侧车定位、五抽象、OpenAPI→Tool、HITL、HTTP 插件/MCP 为连接器形态  
- **删除或改写为「不做」：** 官方 SDK、OTel「可选承诺」、过时草案口吻、与代码不符的未来清单  
- 篇幅控制在可读短文（建议 ≤200 行）

- [ ] **步骤 6：deployment（重写）**

基于旧 `docs/deployment.md` 按**现行**代码更新：

- baize 单二进制；`weixin-adapter` 旁路进程  
- Autostart vs 独立 systemd/compose（对照 `deploy/`、`docker-compose.weixin.yml` 真实路径）  
- 入站 `POST /v0/channels/{name}/inbound`；适配器 `/outbound`、`/admin/*`  
- 去掉已不存在的路径或命令

- [ ] **步骤 7：Commit**

```powershell
git add docs/developers
git commit -m "docs: 新增开发者文档（架构与部署按现状重写）"
```

---

### 任务 3：产品向 README 重写 + 删除旧公开 docs

**文件：**
- 修改：`README.zh-CN.md`、`README.md`
- 删除：`docs/architecture-and-plugin-protocol.md`、`docs/deployment.md`
- 修改：所有仍链到上述两文件的**公开**路径（根 README、`docs/developers` 以外若有）

- [ ] **步骤 1：定位旧链接**

```powershell
Select-String -Path README.md,README.zh-CN.md,docs -Pattern 'architecture-and-plugin-protocol|docs/deployment\.md' -Recurse |
  Where-Object { $_.Path -notmatch 'superpowers' } |
  Select-Object Path,LineNumber,Line
```

- [ ] **步骤 2：重写中文 README 结构（必须按此大纲，内容用人话）**

`README.zh-CN.md` 目标读者：产品 / 运营 / 决策者。建议章节顺序：

1. 标题 + CI 徽章 + 一句话价值（保留侧车定位，减少术语堆砌）  
2. **亮点**（3–5 条短句：不改业务代码、OpenAPI 变工具、审批闸、可卸载、中英 UI 等）  
3. **核心能力**（人话列表，少表格术语）  
4. **适用场景 / 案例**（至少 2 个短场景：遗留 HTTP 系统旁挂；运营审批写操作）  
5. **30 秒试用**（保留 `demo.cmd` / `demo.sh` 最短路径）  
6. **性能说明**：明确写「基准数据将在后续 PERF 回填；此处不写竞品对比数字」  
7. **下一步**：链到 `docs/developers/getting-started.md`（技术读者）  

将现有超长「设置页说明书」式段落**迁出或大幅压缩**——细节进开发者文档或设置页 UI，README 不堆操作员手册。

- [ ] **步骤 3：英文 README 平行**

`README.md` 与中文**同结构**；勿只改标题。Go 版本徽章与正文要求对齐 CI（1.25）。

- [ ] **步骤 4：删除旧文件并改链**

```powershell
git rm docs/architecture-and-plugin-protocol.md docs/deployment.md
```

所有公开引用改为 `docs/developers/architecture.md` / `docs/developers/deployment.md`。

- [ ] **步骤 5：抽查**

```powershell
Select-String -Path README.md,README.zh-CN.md,docs -Pattern 'architecture-and-plugin-protocol|docs/deployment\.md' -Recurse |
  Where-Object { $_.Path -notmatch 'superpowers' }
```

预期：无命中（或仅历史无关）。

- [ ] **步骤 6：Commit**

```powershell
git add README.md README.zh-CN.md docs
git commit -m "docs: 产品向 README 分层并移除过时公开架构/部署文"
```

---

### 任务 4：回归与账本收口

**文件：**
- 修改：`docs/superpowers/notes/2026-09-13-spec-ledger.md`
- 修改：`docs/superpowers/specs/2026-09-15-oss-quality-cleanup-design.md`（实现计划回链 CONTRACT；CONTRACT 状态）
- 修改：`docs/superpowers/notes/2026-09-15-clean-audit.md`（若需注明 CONTRACT 已执行删除项）

- [ ] **步骤 1：全量前端 + 关键 Go**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
go test -count=1 ./internal/api/ ./cmd/baize/
```

预期：绿。（全仓 `go test ./...` 可选；若环境 MCP 文件锁失败则单包重试，与本阶段无关。）

- [ ] **步骤 2：核对删除项不可达**

```powershell
Select-String -Path web\chat\src -Pattern 'patchToolRequireLogin|clearMessages' -Recurse
Test-Path docs\architecture-and-plugin-protocol.md   # 预期 False
Test-Path docs\deployment.md                         # 预期 False
Test-Path docs\developers\deployment.md              # 预期 True
```

- [ ] **步骤 3：账本**

CLEAN 行：CONTRACT **已交付**；下一动作 STRUCT 计划待开。

- [ ] **步骤 4：最终 Commit**

```powershell
git add docs/superpowers
git commit -m "docs: CLEAN-CONTRACT 交付与账本收口"
```

- [ ] **步骤 5：停住**

向用户报告：CONTRACT 完成；下一步写 **CLEAN-STRUCT** 计划。合入/双仓推送等用户指示。

---

## 自检（对照规格 §4.2）

| 需求 | 任务 |
|------|------|
| 白名单删除执行（客户端死导出） | 任务 1 |
| 删除优先于别名；HTTP 未在白名单删除则不动 | 任务 1；无路由改动 |
| README 产品向 | 任务 3 |
| 开发者文档另开 | 任务 2 |
| 删 architecture / deployment 并按现状重写 | 任务 2+3 |
| 性能数字不编造 | 任务 3 占位句 |
| 测试绿 | 任务 1、4 |
| 不做 STRUCT/GATES/PERF | 头部非目标 |

无「待定」步骤；旧公开 docs 删除后链接必须改完。
