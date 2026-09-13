# F-KV 凭据加密 DoD 核验报告

> 日期：2026-09-13  
> 范围：母规格 `2026-09-13-f-production-hardening-design.md` §2（F-KV）、§4.2 F-KV 行  
> 实现分支：`feat/f-kv-encrypt`（`bea14d0`..`14ba31f`，**待合并 `main`**）  
> 方法：规格契约 × 树内证据 × 本机测试命令（有退出码）  
> 结论：**F-KV 实现阶段可标已交付**；史诗 **F 仍欠 F-HOT**，勿标整史诗完成。

---

## 0. 环境与命令证据

| 命令 | 结果 |
|------|------|
| `go test ./internal/settingscrypto/ ./internal/store/ ./internal/runtimecfg/ ./internal/api/ ./internal/bootstrap/ -count=1` | **ok**（worktree 根目录） |

Go：`C:\Users\Administrator\go-sdk\go\bin\go.exe`。

---

## 1. 母规格 §2 / §4.2 对照

| 要求 | 规格 | 证据 | 判定 |
|------|------|------|------|
| **K1** 字段加密 | §2.1 | `model_profiles.api_key`：`model_profiles.go` / `model_profiles_sql.go` Seal/Open；`runtime_settings` 口令：`runtimecfg/persist.go`；`inbox_channels[].secret`：`api/server_inbox.go` | ✅ |
| **M1** 仅 env | §2.2 | `settingscrypto.KeyFromEnv()` → `BAIZE_SETTINGS_KEY`；`.env.example` 注释 | ✅ |
| **P1** 启动迁移 | §2.4 | `store.MigrateStore` + `bootstrap` store 就绪后调用；`secrets_migrate_test.go` 明文迁密封、已密封跳过 | ✅ |
| 无 key **拒写** | §2.2 | `Seal` → `ErrNoKey`；API `settings_errors.go` → `400` / `settings_key_required`；单测：store / runtimecfg / api inbox & models & runtime | ✅ |
| 密文无 key **拒启** | §2.2 | `MigrateStore` 无 key 扫密封 → `ErrCiphertext`；`bootstrap/migrate_test.go` | ✅ |
| GET **仍脱敏** | §2.5 | `RedactAPIKey` / `server_models.go` 响应路径未改算法；`identity/redact.go` 本分支 **无 diff** | ✅ |
| redacted PATCH 不覆盖 | §2.5 | `IsRedactedAPIKey` + upsert 逻辑（既有 + 加密前仍跳过占位） | ✅ |
| **events_webhook** | §2.1「若存在」 | 当前 JSON **无独立 secret 字段**；`MigrateStore` / 写路径 **未挂** `events_webhook`；计划显式跳过 | ✅（本刀不改） |
| 不做 KMS / 整表 | §2.6 | 仅 K1 字段；无 KMS / 无全 settings 加密 | ✅ |
| README / 文档 | §4.3 | `README.md` / `README.zh-CN.md` **Settings encryption**；热更新规格 §1.4 交叉引用 | ✅ |
| 信封 `bz1:` + SHA-256 | §2.3 | `settingscrypto/crypto.go` | ✅ |
| `reset-credentials` 正交 | §2.5 | README 写明；未改 break-glass 语义 | ✅ |

§4.2 必测行：**无 key 拒写、有 key 往返、P1 迁明文、密文无 key 拒启、GET 脱敏、redacted PATCH** — 均由上表对应单测 / 集成测覆盖（见 §2）。

---

## 2. 计划与分支边界

| 检查项 | 结果 |
|--------|------|
| 计划 `2026-09-13-f-kv-encrypt.md` 无「待定」实现步骤 | ✅ |
| 实现提交链：crypto → store → runtimecfg → api inbox → docs → migrate/bootstrap | ✅（`bea14d0`..`14ba31f`） |
| 史诗 **F** 整包完成 | ❌ **仍欠 F-HOT**（`f-store-hot-swap` 计划待写） |
| 非阻塞延后（计划/ledger 已记） | Task1：base64/ErrCorrupt 向量补强；Task4：GET 已密封 inbox 单测可选补 |

---

## 3. 结论与账本动作

**结论：F-KV 在 `feat/f-kv-encrypt` 上满足母规格 §2 与 §4.2 F-KV 行；合并 `main` 前账本可标「已交付（实现阶段）」。**

- 更新 `2026-09-13-spec-ledger.md` §5：**F-KV 已交付（实现阶段）**；仍欠 **F-HOT**  
- 更新确认清单史诗 **F** 行：同上，**不**标史诗 F 全部完成  
- 母规格 §5.1：F-KV 计划行改为已交付（实现阶段）+ 链本报告  

**不做本刀：** F-HOT、DOC、UI 改版；加密 webhook `headers`、渠道 `secret.key` 文件、整表 settings。
