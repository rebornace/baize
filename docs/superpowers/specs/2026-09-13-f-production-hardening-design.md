# F 生产硬化（母篇）

- 日期：2026-09-13
- 状态：已批准（2026-09-13）
- 归属：开源首版史诗 **F**（含 C1 + OPS-HOT）；确认清单合并策略 A / F×待办 **A1**
- 交付形态：**S1** — 本稿为母规格；实现计划按 **B1** 拆三份独立计划
- 产品决策来源：`docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md`

## 1. 目标 / 非目标 / 三切片边界

### 1.1 目标

把 F 收成可交付的生产硬化三刀：**质量门禁**、**秘密落库加密**、**Store 可运维热切**（含 SIGHUP / Windows 等价 reload），且不吞并 DOC / 渠道 / Memory / OAuth 等史诗。

### 1.2 非目标（本母篇明确不做）

- DOC-P1/P2、完整 i18n、export_db_ro、PORT/OUTBOX、@登录、思考级别、BLOB-CS、MCP OAuth、Memory
- blob / S3 / Redis 热重连；监听端口 / TLS / `data_dir` 热改
- 外部 KMS / Vault 真实现；整表 settings 加密；全仓 lint 清债
- 未产品确认的额外「硬化」点子
- 文件 watch 守护进程；多实例协调切库

### 1.3 三切片边界

| 刀 | ID | 做什么 | 不做什么 |
|----|-----|--------|----------|
| 1 | **F-C1** | `golangci-lint` + ESLint 配置；CI fail 门禁；只修本刀改动路径上的必破规则 | 不清历史债；不改业务行为 |
| 2 | **F-KV** | K1 秘密字段加密落库；`BAIZE_SETTINGS_KEY`；无 key fail-closed；P1 启动自动迁明文；API GET 仍脱敏 | 不加密非秘密 settings；不做 KMS；不改 UI 交互语义 |
| 3 | **F-HOT** | Store 驱动 API 热切（进程内换连接）；SIGHUP / Windows reload 重读 YAML/overlay 并刷入可热应用快照 | blob/S3 仍重启；不做整配置 watch |

### 1.4 依赖顺序（B1）

`F-C1`（无功能依赖）→ `F-KV`（读写秘密路径）→ `F-HOT`（热切时须能解密读库；SIGHUP 与现有 `runtimecfg` 对齐）。

### 1.5 已锁定产品决策

| 码 | 决策 |
|----|------|
| A1 | DOC 独立；F 不吸收其它史诗 |
| B1 | 切片顺序 C1 → KV 加密 → 热切换/SIGHUP |
| C1-a | 配置 + CI；只修改动路径必破规则，不清全仓债 |
| K1 | 加密控制面口令 + 模型 `api_key` + webhook/inbox `secret` |
| M1 | 主密钥仅环境变量；未设置则 fail-closed |
| H1 | Store API 热切 + SIGHUP 刷 YAML；blob/S3 仍重启 |
| P1 | 有 key 启动时自动就地加密 K1 范围明文 |
| S1 | 一篇母规格 + 三份独立实现计划 |

---

## 2. F-KV：加密契约 / fail-closed / 迁移

### 2.1 密文字段（K1）

| 落库位置 | 字段 |
|----------|------|
| `runtime_settings` JSON（settings KV） | `operator_token`、`admin_token`、`operators[].token` |
| `model_profiles` 表 | `api_key`（`api_key_env` **不**加密） |
| `events_webhook` JSON | webhook `secret`（若存在） |
| `inbox_channels` JSON | 各 channel `secret` |

进程内鉴权、调 LLM、验签：**始终使用明文**；仅持久化层加密。

### 2.2 主密钥（M1）

- 环境变量名：`BAIZE_SETTINGS_KEY`（实现计划约定最小熵与编码：原始或 base64）。
- **有 key：** 写路径加密；读路径：密文解密 / 明文兼容（见 §2.4）。
- **无 key：**
  - **拒写**任何 K1 秘密（API 返回明确错误，提示需配置主密钥）；
  - **可读**存量明文（兼容旧部署）；
  - 若库中已是密文而启动无 key → **启动失败**（无法解密，fail-closed）。
- 不写本地密钥文件；不做 KMS。

### 2.3 信封格式

- AES-256-GCM；nonce 随机。
- 落库值加可识别前缀：`bz1:` + base64(nonce ‖ ciphertext ‖ tag)，避免与明文混淆。
- 派生：`SHA-256(BAIZE_SETTINGS_KEY)` → 32 字节（或实现计划若改 HKDF，须带测试向量；母篇默认 SHA-256）。

### 2.4 迁移（P1）

启动装配（store 可用后、对外服务前）：

1. 扫描 K1 字段；
2. 明文 → 加密写回；已是 `bz1:` → 跳过；
3. 迁移写失败 → 启动失败并打日志；
4. 读路径长期兼容明文，直到被迁完或改写。

### 2.5 对外语义不变

- GET 凭据 / profile：**仍脱敏**，响应形状不因加密改变。
- `baize settings reset-credentials`：仍删除 KV 中凭据覆盖段（break-glass）；与加密正交。
- 测例底线：无 key 拒写；有 key 往返；启动迁明文；密文无 key 拒启；redacted PATCH 不覆盖原文。

### 2.6 本刀不做

整表加密、主密钥轮换 UI、多密钥版本、加密非秘密字段。

---

## 3. F-HOT：Store 热切 + SIGHUP

### 3.1 现状 → 目标

| 现状 | 目标（H1） |
|------|------------|
| PUT store → 写 local overlay → 可选整进程 `Reexec` | PUT 仍写 overlay；**默认进程内热切**（`Open` 新库 → 原子换 `Store` 及会话/身份句柄 → 关旧库） |
| 无配置热重载信号 | **SIGHUP**（POSIX）重读 base+overlay，刷入可热应用快照 |

仍保留 `restart: true` / `POST .../restart` 作逃生舱（热切失败或运维强制整进程拉起）。

### 3.2 Store API 热切

1. 校验同今：`acknowledge_no_migrate`、driver 已注册、postgres 需 DSN、sqlite 路径默认。
2. 原子写 local overlay（跨重启真相仍在文件）。
3. `store.Open` 新驱动；失败 → **不换**当前 Store，返回错误；若 overlay 已写，响应标明「已落盘未生效」，可建议 restart。
4. 成功：原子替换 Server 上 Store / conversation / identity（及任何直接持有旧 Store 的引用）；`Close` 旧连接。
5. **不做** SQLite ↔ PG 数据迁移（与今一致）。
6. UI：默认「保存并热切换」；高级/失败路径保留「保存并重启」。

### 3.3 SIGHUP / 跨平台

- **Linux / macOS：** `SIGHUP` → `LoadLayered` → 将可热应用段刷入 `runtimecfg`（及同类快照）。
- **SIGHUP 不自动换 Store：** 若 overlay 中 store 段与当前运行驱动不一致，记日志（及可观察状态位「需热切或重启」），避免文件手改与 API 竞态时静默切库。Store 切换以 API 热切路径为准。
- **Windows（无 SIGHUP）：** 等价 **`POST /v0/settings/reload`（admin）**，语义同 SIGHUP。
- 不做文件 watch 守护进程。

### 3.4 明确不做

blob / S3 / Redis 热重连；端口 / TLS / `data_dir` 热改；整配置无差别全量重建 Agent。

### 3.5 验收要点

- memory ↔ sqlite ↔ postgres 热切后读写落在新库；旧连接关闭。
- 热切失败时进程仍用旧库且可服务。
- SIGHUP / Windows `POST reload` 后引擎参数等与 YAML 对齐。
- 在 F-KV 之后：热切后仍能解密读 K1 字段。

---

## 4. F-C1 门禁 + 测试 / 文档 / 风险

### 4.1 F-C1（C1-a）

| 项 | 约定 |
|----|------|
| Go | 根目录 `.golangci.yml`；CI 跑 `golangci-lint`；规则集偏稳（如 errcheck / staticcheck / govet），避免一上来开满噪声规则 |
| Web | 现有 ESLint；CI 中 lint **fail**（非 warn-only） |
| 清债 | **只修本刀为过门禁而改的路径**；历史债不进本刀 |
| 本地 | README / 贡献说明：如何安装与提交前运行 |
| 不做 | 全仓 format 大扫除、强制复杂度清零、借机改业务逻辑 |

### 4.2 三刀测试（母篇级）

| 刀 | 必测 |
|----|------|
| F-C1 | CI job 红/绿契约（配置存在且门禁启用） |
| F-KV | 无 key 拒写；有 key 往返；P1 启动迁明文；密文无 key 拒启；GET 仍脱敏；redacted PATCH 不覆盖 |
| F-HOT | 热切成功/失败回滚；overlay 落盘；SIGHUP 与 Windows `POST reload`；`restart` 逃生舱仍可用 |

### 4.3 文档

- README / README.zh-CN：`BAIZE_SETTINGS_KEY`、热切 vs 重启、SIGHUP / Windows reload。
- 本规格批准后：ledger / 确认清单挂路径；按 B1 写三份计划（见 §5）。
- 热更新规格（`2026-09-05-runtime-settings-hot-reload-design.md`）中已 defer 且由本母篇承接的项，交叉引用至本稿。

### 4.4 风险与缓解

| 风险 | 缓解 |
|------|------|
| 热切漏换引用 → 双 Store | 集中持有点 + 集成测读写身份 |
| 主密钥丢失 | 文档强调备份；`reset-credentials` 仍可回 YAML break-glass（仅口令覆盖） |
| lint 噪声拖垮 PR | C1-a；规则渐进，不本刀开满 |
| Windows 无 SIGHUP | 强制提供等价 HTTP reload |

---

## 5. 交付物与后续

### 5.1 交付物（S1）

1. 本稿：`docs/superpowers/specs/2026-09-13-f-production-hardening-design.md`
2. 实现计划（按 writing-plans 另开，顺序 B1）：
   - `docs/superpowers/plans/2026-09-13-f-c1-lint.md`（**实现中**；C1 配置+CI 已落地）
   - `docs/superpowers/plans/2026-09-13-f-kv-encrypt.md`（待写）
   - `docs/superpowers/plans/2026-09-13-f-store-hot-swap.md`（待写）

### 5.2 开刀建议

产品建议顺序仍是 **DOC 独立史诗可先于或并行于 F-C1**；F 功能刀按 B1。本母篇不规定 DOC 的实现细节。

### 5.3 与既有规格关系

- 承接：`2026-09-05-runtime-settings-hot-reload-design.md` §1.4 中「驱动热切换 / SIGHUP / 凭据 KV 加密」。
- 扩展：`2026-08-29-sql-store-drivers-v0-design.md` 的「必须重启换库」→ 本篇 F-HOT 改为默认热切，重启降为逃生舱。
- 不替代：UI-RUNTIME 人话化（已交付）；完整 i18n（UI-I18N）。
