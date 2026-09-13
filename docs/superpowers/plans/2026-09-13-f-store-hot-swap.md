# F-HOT：Store 热切 + SIGHUP / Windows reload 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** PUT `/v0/settings/store` 默认进程内热切 Store（写 overlay → Open 新库 → 原子换引用 → Close 旧库）；保留 `restart: true` / `POST .../store/restart` 逃生舱；POSIX `SIGHUP` 与 Windows `POST /v0/settings/reload` 重读 layered YAML 并刷 `runtimecfg` 基线；SIGHUP **不**自动换 Store。

**架构：** bootstrap 持有一个 `storeRuntime`（集中持有可换引用：底层 Store、Notify 包装、Messages/Identities、Engine、Webhook Dispatcher、`llm.StoreProfileSource`、runtimecfg refresh、closer）。API 层校验+写 overlay 后调用注入的 `HotSwapStore`；失败不换引用，响应标明「已落盘未生效」。配置 reload 走 `LoadLayered` + `Holder.ReplaceBaseline`，并在 GET store 暴露 `store_config_mismatch`。

**技术栈：** Go 标准库 `os/signal`、`syscall`（SIGHUP 仅非 Windows 编译标签）、现有 `config.WriteStoreOverlay` / `LoadLayered`、`store.OpenWithOptions`、`store.MigrateStore`、`runtimecfg`、React 设置页。提交中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-13-f-production-hardening-design.md` §3（刀 **F-HOT**；决策 **H1**）

**锁定约定（实现勿偏离）：**

| 码 | 含义 |
|----|------|
| H1 | Store **API** 热切 + SIGHUP 刷 YAML；blob/S3/Redis **仍重启** |
| 默认 PUT | `restart` 省略或 `false` → 热切；`restart: true` → 今行为（写 overlay + Reexec） |
| SIGHUP | 只刷可热应用 YAML→`runtimecfg` 基线 + 更新 `Server.Config`；**不** Open 新 Store；overlay store 与运行中不一致 → 日志 + GET `store_config_mismatch: true` |
| Windows | `POST /v0/settings/reload`（admin）语义同 SIGHUP |
| 无迁移 | 与今一致：不搬数据；空新库需自行配置 |
| F-KV | 热切 Open 后必须 `MigrateStore`；有 `BAIZE_SETTINGS_KEY` 时新库明文会迁密文 |

**已知持有旧 Store 的引用（热切必须换齐）：**

| 持有者 | 字段 |
|--------|------|
| `api.Server` | `Store`、`Messages`、`Identities`、`Config.Store.*` |
| `run.Engine` | `Store`、`Messages`、`Identities`、`Meta` |
| `run.Compactor` | `Messages` |
| `webhook.Dispatcher` | `store`（需新增 `SetStore`） |
| `llm.StoreProfileSource` | `Store`（同一指针上改字段即可；Switch 不重建） |
| `runtimecfg.StartRefresh` | 捕获的 `st`（改为读 `atomic.Pointer[store.Store]` 或 cancel+重启 refresh） |
| `storeAndMCPCloser.inner` | 底层 `io.Closer`（换为新库；再 Close 旧库） |
| 工具目录 | 按新库 `ListConnectors` 重载：对旧 connector `UnregisterConnector` 后 `loadStoredConnectors` + YAML `registerConnector`；默认 agent `UpsertAgent` |
| Inbox | 从新库重新 `seedInboxChannels`（或等价 Reload）进既有 `inbox.Registry` |

**明确不做：** blob/S3/Redis 热重连；端口/TLS/`data_dir` 热改；文件 watch；多实例协调切库；SQLite↔PG 数据迁移；整配置重建 Agent 进程。

**测试环境：** 写秘密路径继续设 `BAIZE_SETTINGS_KEY`（沿用 F-KV）。热切测用 temp sqlite ↔ memory。

---

## 文件结构

创建：

- `internal/bootstrap/store_hotswap.go` — `storeRuntime` + `HotSwap(overlay)` + `ReloadLayeredConfig()`
- `internal/bootstrap/store_hotswap_test.go` — 热切成功/失败不换 / 读写落新库
- `internal/bootstrap/sighup_unix.go` — `signal.Notify(SIGHUP)` → `ReloadLayeredConfig`
- `internal/bootstrap/sighup_windows.go` — 空实现（HTTP reload 覆盖）
- `internal/api/server_settings_reload.go` — `POST /v0/settings/reload`
- `internal/api/server_settings_reload_test.go`
- `docs/superpowers/plans/2026-09-13-f-store-hot-swap.md` — 本文件

修改：

- `internal/api/server.go` — `HotSwapStore func(config.StoreOverlay) error`；`ReloadConfig func() error`；路由注册 reload
- `internal/api/server_store_settings.go` — PUT：`restart` 假/省略 → HotSwap；失败码；GET 加 `store_config_mismatch` / `effective_driver`
- `internal/api/server_store_settings_test.go` — 热切成功/失败/逃生舱
- `internal/webhook/dispatcher.go` — `SetStore(store.Store)`
- `internal/runtimecfg/runtimecfg.go` — `ReplaceBaseline(Snapshot)`（保留 KV 覆盖，重 merge）
- `internal/runtimecfg/persist.go` — `StartRefresh` 改为每次 tick 读 `*atomic.Pointer[store.Store]`，或新增 `StartRefreshPtr`
- `internal/runtimecfg/runtimecfg_test.go` — ReplaceBaseline 测例
- `internal/bootstrap/bootstrap.go` — 构造 `storeRuntime`、注入回调、挂 SIGHUP、Agent Upsert 抽可复用
- `internal/controlplane/acl.go` + `acl_test.go` — `POST /v0/settings/reload` → Admin
- `web/chat/src/pages/StorageSettings.tsx` + `StorageSettings.test.tsx` — 默认「保存并热切换」；次要「保存并重启」
- `web/chat/src/strings.ts` — 文案
- `web/chat/src/api.ts` — 类型字段（可选）
- `README.md` / `README.zh-CN.md` — 热切 vs 重启、SIGHUP / Windows reload；改「驱动切换须重启」过时句
- `docs/superpowers/specs/2026-09-05-runtime-settings-hot-reload-design.md` §1.4 — 交叉引用 F-HOT 已承接驱动热切/SIGHUP
- `docs/superpowers/notes/2026-09-13-spec-ledger.md` / 确认清单 / 母规格 §5.1 — 挂本计划

不改：blob 装配、middleware Redis、监听端口、F-KV 信封、渠道适配器进程。

---

## 任务 1：`runtimecfg.ReplaceBaseline` + refresh 可换 Store

**文件：**
- 修改：`internal/runtimecfg/runtimecfg.go`、`persist.go`
- 测试：`internal/runtimecfg/runtimecfg_test.go`

- [ ] **步骤 1：写失败测试**

```go
func TestReplaceBaselineKeepsOverrides(t *testing.T) {
	h := New(Snapshot{Knobs: Knobs{MaxSteps: 10}})
	steps := 20
	h.mu.Lock()
	h.ko = knobsOverride{MaxSteps: &steps}
	h.swapLocked()
	h.mu.Unlock()

	h.ReplaceBaseline(Snapshot{Knobs: Knobs{MaxSteps: 50, MaxMessages: 99}})
	k := h.Knobs()
	if k.MaxSteps != 20 {
		t.Fatalf("override lost: got %d", k.MaxSteps)
	}
	if k.MaxMessages != 99 {
		t.Fatalf("baseline MaxMessages not applied: %d", k.MaxMessages)
	}
}
```

（若 `ReplaceBaseline` 为导出且测试在 `package runtimecfg`，可直接调；对外测用公开 API。）

- [ ] **步骤 2：** `go test ./internal/runtimecfg/ -run ReplaceBaseline -count=1` → FAIL

- [ ] **步骤 3：实现**

```go
// ReplaceBaseline swaps the YAML/config baseline and re-merges current KV overrides.
func (h *Holder) ReplaceBaseline(base Snapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.base = base
	h.swapLocked()
}
```

`StartRefresh`：改为接受 `func() store.Store` 或 `*atomic.Pointer[store.Store]`，每次 `Load` 用当前指针。bootstrap 热切时 `storePtr.Store(&newWrapped)`。

- [ ] **步骤 4：** `go test ./internal/runtimecfg/ -count=1` PASS

- [ ] **步骤 5：Commit** `feat(runtimecfg): 支持 ReplaceBaseline 与可换 Store 刷新`

---

## 任务 2：`webhook.Dispatcher.SetStore`

**文件：**
- 修改：`internal/webhook/dispatcher.go`
- 测试：现有或补 `dispatcher_test.go` 一行：SetStore 后 outbox 读写走新 store

- [ ] **步骤 1–4：** TDD 最小实现：

```go
func (d *Dispatcher) SetStore(st store.Store) {
	d.mu.Lock()
	d.store = st
	d.mu.Unlock()
}
```

出站路径读 `d.store` 已在锁外用局部变量处：核对所有 `d.store` 访问在 `RLock` 下拷贝，避免与 SetStore 竞态。

- [ ] **步骤 5：Commit** `feat(webhook): Dispatcher 支持热切 Store`

---

## 任务 3：`storeRuntime.HotSwap`（bootstrap 核心）

**文件：**
- 创建：`internal/bootstrap/store_hotswap.go`、`store_hotswap_test.go`
- 修改：`bootstrap.go` — Start 末尾把可变引用塞进 `storeRuntime`，`srv.HotSwapStore = rt.HotSwap`

**`storeRuntime` 建议字段：** `mu sync.Mutex`；`hub *eventbus.Hub`；`storePtr *atomic.Pointer[store.Store]`（Notify 包装后的）；`rawCloser *storeAndMCPCloser`；`srv *api.Server`；`engine *run.Engine`；`compactor *run.Compactor`；`dispatcher *webhook.Dispatcher`；`profiles *llm.StoreProfileSource`（可为 nil）；`reg *tool.Registry`；`inboxReg *inbox.Registry`；`cfg *config.Config`；`configPath string`；`callbackCfg connector.CallbackConfig`；以及 seed inbox / registerConnector 所需只读依赖。

- [ ] **步骤 1：写失败集成测（包内）**

```go
func TestHotSwapMemoryToSQLite(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	dir := t.TempDir()
	// StartForTest memory → HotSwap sqlite path under dir
	// UpsertAgent / CreateRun on new store succeeds
	// old memory CreateRun id 在新库 GetRun 失败
}
```

- [ ] **步骤 2：** FAIL（无 HotSwap）

- [ ] **步骤 3：实现 `HotSwap(o config.StoreOverlay) error` 顺序**

1. `OpenWithOptions` 新库；失败 → return（overlay 已由 API 写好）
2. `MigrateStore(new)`
3. `openConversationAndIdentities(new, *cfgWithOverlay)`
4. `UpsertAgent` 默认 agent（与启动相同）
5. `wrapped := eventbus.Notify(new, hub)`
6. 锁内：更新 `srv.Store/Messages/Identities`、`engine.*`、`compactor.Messages`、`dispatcher.SetStore(wrapped)`、`profiles.Store = wrapped`（若非 nil）、`cfg.Store = ...`、`storePtr`、`rawCloser.inner = storeCloser(new)`
7. 锁外：重载 connectors（Unregister 旧 ID + `loadStoredConnectors` + `registerConnector`）；reload inbox from store
8. `Close()` 旧 raw store（保存换之前的 closer）
9. Open 失败路径：**绝不**改引用

失败测：对非法 DSN/`Open` 失败，断言 `srv.Store` 指针不变。

- [ ] **步骤 4：** `go test ./internal/bootstrap/ -run HotSwap -count=1` PASS

- [ ] **步骤 5：Commit** `feat(bootstrap): Store 进程内热切`

---

## 任务 4：API PUT 热切 + GET mismatch + restart 逃生舱

**文件：**
- 修改：`internal/api/server.go`、`server_store_settings.go`、`server_store_settings_test.go`

- [ ] **步骤 1：失败测**

- PUT `restart:false`（或省略）且 `HotSwapStore` 注入成功回调 → 200 + `status=hot_swapped`，**不**调 `RestartProcess`
- `HotSwapStore` 返回 error → 500/`hot_swap_failed`，body 含 `overlay_saved: true`（或等价字段）
- `restart:true` → 仍 202 restarting（可用假 RestartProcess 计数）

- [ ] **步骤 2–3：实现**

```go
// handlePutStoreSettings 在 WriteStoreOverlay 成功后：
if body.Restart {
  go s.scheduleRestart()
  // 202 同今
  return
}
if s.HotSwapStore == nil {
  writeError(..., "hot_swap_unavailable", "...")
  return
}
if err := s.HotSwapStore(overlay); err != nil {
  writeJSON/writeError(..., "hot_swap_failed", ..., overlay_saved)
  return
}
writeJSON(200, {"status":"hot_swapped","message":"..."})
```

GET：比较 `cfg.Store.Driver`（文件已写进 `Server.Config` 时）与「启动/热切后的 effective」——热切成功后应更新 `Server.Config.Store`；SIGHUP 后若 overlay 改了 store 而尚未 API 热切，设 `store_config_mismatch: true`，并返回 `effective_driver`（运行中）vs `driver`（配置文件）。

实现建议：`storeRuntime` 另存 `effective config.StoreOverlay`；`ReloadLayeredConfig` 只更新 `Server.Config` 全量 YAML，**不**改 `effective`；GET 用两者比较。

- [ ] **步骤 4：** `go test ./internal/api/ -run StoreSettings -count=1` PASS

- [ ] **步骤 5：Commit** `feat(api): Store PUT 默认热切并暴露 mismatch`

---

## 任务 5：SIGHUP + `POST /v0/settings/reload`

**文件：**
- 创建：`sighup_unix.go`、`sighup_windows.go`、`server_settings_reload.go` + test
- 修改：`acl.go`、`bootstrap.go`（`ReloadLayeredConfig`）

- [ ] **步骤 1：API 测** — admin POST reload → 调 `ReloadConfig`；无权限 403

- [ ] **步骤 2–3：`ReloadLayeredConfig`**

1. `cfg, _, err := config.LoadLayered(configPath, LocalOverlayPath(...))`
2. 解析 control_plane tokens（同启动）
3. `runtimeHolder.ReplaceBaseline(snapshotFrom(cfg, tokens...))`
4. `*srv.Config = cfg`（保留指针同一）
5. 若 `cfg.Store` ≠ effective store → `log.Printf("store config mismatch: ...; use PUT /v0/settings/store to hot-swap")`
6. **不**改 blob/middleware/端口

Unix：`go signal.Notify` 在 Start 成功后起 goroutine；ctx cancel 时 stop。

- [ ] **步骤 4：** `go test ./internal/api/ ./internal/bootstrap/ -count=1` 相关 PASS

- [ ] **步骤 5：Commit** `feat(ops): SIGHUP 与 POST settings/reload 刷 YAML 基线`

---

## 任务 6：存储设置 UI 默认热切

**文件：**
- `web/chat/src/pages/StorageSettings.tsx`、`StorageSettings.test.tsx`、`strings.ts`

- [ ] **步骤 1：失败测** — 主按钮确认后 PUT body **不含** `restart: true`（或 `restart: false`）；存在「保存并重启」次要操作仍带 `restart: true`

- [ ] **步骤 2–3：实现** — 主 CTA「保存并热切换」；ConfirmDialog 文案说明不迁移、进行中任务可能失败；`<details>` 或次要 Button「保存并重启」走旧路径

- [ ] **步骤 4：** `npm test -- StorageSettings` PASS；必要时 `npm run build`

- [ ] **步骤 5：Commit** `feat(ui): 存储设置默认热切换`

---

## 任务 7：文档与账本收口

**文件：** README、README.zh-CN、热更新规格 §1.4 交叉引用、ledger、确认清单、母规格 §5.1

- [ ] **步骤 1：** 文档写明：`BAIZE_SETTINGS_KEY` 与热切正交；热切 vs 重启；`kill -HUP <pid>` / `POST /v0/settings/reload`；blob 仍须重启

- [ ] **步骤 2：** ledger §5 F 行 → F-HOT 计划已挂；母规格 §5.1 第三计划改为本路径

- [ ] **步骤 3：** `go test ./internal/api/ ./internal/bootstrap/ ./internal/runtimecfg/ ./internal/webhook/ -count=1`

- [ ] **步骤 4：Commit** `docs: F-HOT 热切与 SIGHUP 说明`

---

## 任务 8：DoD 自检（实现后）

对照母规格 §3.5 / §4.2 F-HOT 行：

| 项 | 验证 |
|----|------|
| memory↔sqlite 热切读写新库 | `TestHotSwap*` |
| 热切失败旧库仍服务 | Open 失败测 |
| overlay 已写 | 失败响应 + 文件存在 |
| SIGHUP / POST reload | API 测 +（可选）手动 |
| restart 逃生舱 | PUT `restart:true` |
| F-KV | 热切后带 key 读 K1 不炸 |

写短笔记 `docs/superpowers/notes/2026-09-13-f-hot-dod-audit.md`（私有仓），ledger 标 F-HOT 已交付（实现阶段）。

- [ ] **步骤：Commit** `docs: F-HOT DoD 核验`

---

## 风险备忘（实现时）

- **双 Store：** 所有引用必须经 `storeRuntime` 一处替换；禁止 API handler 直接 `Open` 后只改 `srv.Store`。
- **进行中 run：** 旧 run 事件在旧库；热切后 Resume 可能失败——UI/文档提示；逃生舱重启。
- **Notify 包装：** closer 始终对 **未包装** raw store；`srv.Store` 用 Notify 包装。
- **Windows：** 无 SIGHUP 编译；依赖 HTTP reload。
