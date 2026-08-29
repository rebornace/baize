# SQL 存储驱动（P4）设计规格

> 状态：已批准（2026-08-29）  
> 日期：2026-08-29  
> 前置：I1 Inbox HITL resume 已交付  
> 依据：开源首版边界清单 P4；头脑风暴选定方案 1（驱动注册表 + 本版交付 postgres 范例）  
> 路线：P4 → P1/P2 文档 + F 生产硬化

---

## 1. 目标与成功标准

**目标：** 主存储支持 **SQL 多源适配**（编译期驱动注册表）；本版交付 `memory` / `sqlite` / `postgres`。同一 DSN 承载 Runtime `store.Store`、会话消息、身份。设置页可选已注册驱动并保存配置，确认后 **落盘 + 优雅退出 + 同参自拉起**。

**成功标准：**

1. `store.RegisterDriver` / `store.Open` / `store.ListDrivers`；新方言通过独立包 `init` 注册，**不**为每个库改核心 switch 业务逻辑。
2. `driver: postgres` + 有效 `dsn` 可启动；Run / Inbox / outbox / 会话 / 身份落同一 PG；`PersistIdentities` 语义不变。
3. 设置页（admin）：列出已注册驱动；编辑 `sqlite_path` 或 `dsn`；保存须勾选「数据不会自动迁移」；确认后可靠自重启。
4. CI 带 Postgres service，相关测试默认必绿；sqlite/memory 不回退。
5. README：如何换库、单实例、无迁移、Docker 需 `restart` 策略。

**明确不做：**

| 项 | 原因 |
|----|------|
| MySQL / SQL Server 本版实现 | 仅 PG 作范例；文档说明如何加驱动包 |
| ORM | 与现有手写 SQL / 语义对齐成本 |
| SQLite↔PG 迁移工具 | 部署时选库；换源自负 |
| 多实例 outbox 抢锁 | 本版单实例 |
| Redis / Mongo 主存储 | 非 SQL 主库模型 |
| 插件市场下载驱动 / 热切换 | 后续里程碑 |
| 不重启切换主库 | 必须重启 |

---

## 2. 架构

```
cmd/baize ── blank import ──► internal/store/driver/postgres (RegisterDriver)
bootstrap ──► store.Open(driver, opts)
              ├── memory
              ├── sqlite  ──► SQLStore + DialectSQLite
              └── postgres ──► SQLStore + DialectPostgres
                    │
                    ├── conversation.OpenSQL(db, dialect)
                    └── identity.OpenSQL(db, dialect)
```

- 持久化驱动实现 `SQLDB() *sql.DB`（经 `SQLStore` 或等价接口）。
- Agent 仍进程内内存（与 SQLite 一致）。

---

## 3. 配置

```yaml
store:
  driver: postgres          # memory | sqlite | postgres
  sqlite_path: ./data/baize.db
  dsn: "postgres://user:pass@localhost:5432/baize?sslmode=disable"
```

- `sqlite`：使用 `sqlite_path`。
- `postgres`：必填 `dsn`。
- 设置页保存写入 **local 覆盖 YAML**（与 `configs/*.local.yaml` 优先级一致）。

---

## 4. API / 设置 UI

| 方法 | 路径 | Gate | 说明 |
|------|------|------|------|
| GET | `/v0/settings/store` | admin | 当前驱动 + 已注册列表；DSN 脱敏 |
| PUT | `/v0/settings/store` | admin | 须 `acknowledge_no_migrate: true`；原子写 local yaml |
| POST | `/v0/settings/store/restart` | admin | 确认后触发落盘后的自重启 |

---

## 5. 重启语义

1. PUT 成功落盘 local 配置。
2. POST restart：`Shutdown` HTTP → Unix `syscall.Exec` / Windows 起新进程再退旧进程。
3. 新进程读 local 配置并 `Open` 新驱动。
4. Docker：文档要求 `restart: unless-stopped`；应用不保证无编排时容器自启。
5. 无写权限 → 拒绝保存。

---

## 6. 表结构（Postgres）

镜像 SQLite：`runs`、`events`、`connectors`、`tools`、`settings`、`inbox_deliveries`、`inbox_threads`、`webhook_outbox`、`messages`、`identities`、`artifacts`。

- 占位符 `$n`；布尔 `BOOLEAN`；时间 `TIMESTAMPTZ` 或 TEXT（实现选一种并保持一致）。
- 空库 `CREATE IF NOT EXISTS`；不做从 SQLite 迁数据。

---

## 7. 测试 / CI

- GitHub Actions `services.postgres` + `BAIZE_TEST_PG_DSN`。
- `internal/store` postgres 驱动测试；conversation/identity 同库测试。
- 重启：`Reexec` 可注入/mock 的单测。

---

## 8. 文档

- README.zh-CN / EN：换库步骤、设置页、不迁移警告。
- `docs/architecture-and-plugin-protocol.md`：PG 已实现。
- 「如何新增 SQL 驱动包」清单（RegisterDriver + DDL + blank import + CI optional job）。

---

## 9. 后续（非本版）

- MySQL 驱动包、多实例 `SKIP LOCKED`、插件市场安装驱动、可选迁移工具。
