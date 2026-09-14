# 规格账本（交付状态目录）

> 日期：2026-09-13  
> 状态：**现行**（对齐方案：总表 + 规格头批量改状态）  
> 目的：回答「已交付 / 仍欠 / 确认不做」，避免只看旧 OSS backlog 或「设计稿」字样时漏项。  
> 范围：仅私有仓 `docs/superpowers`；不导出公开仓。  
> 对照原则：以 `main` 上明显实现 + 既有交付叙事为准；未逐项对照 DoD 的标 **待核验**。

**相关：** [`2026-08-28-oss-backlog-and-enterprise.md`](2026-08-28-oss-backlog-and-enterprise.md)（开源首版边界；本表覆盖其后新增线）

---

## 状态枚举

| 状态 | 含义 |
|------|------|
| 已交付 | 主路径在 `main`，可当完成 |
| 已作废 | 被后续规格取代 |
| 仍欠 | 明确未做或未收口，可进下一里程碑 |
| 待核验 | 代码疑似有，未对照完整 DoD |
| 确认不做（本版） | OSS backlog 已否决 |

---

## 1. 开源首版能力（OSS 顺序 1–3）

| 规格 | 状态 | 对照 | 下一动作 |
|------|------|------|----------|
| `2026-08-28-inbox-hitl-resume-v0-design.md` | 已交付 | I1 | — |
| `2026-08-29-sql-store-drivers-v0-design.md` | 已交付 | P4 SQL/PG | —（本地 worktree `sql-store-drivers-v0` 已于 2026-09-13 清理） |
| `2026-08-29-channel-weixin-v0-design.md` | 已交付（进程内 v0） | 后续形态见 phase2b | — |
| `2026-08-29-mcp-export-v0-design.md` | 已交付 | X1 | — |
| `2026-08-30-multi-model-profiles-design.md` | 已交付 | X2 | — |
| `2026-09-01-context-compaction-design.md` | 已交付 | X4 | — |
| `2026-09-03-middleware-multi-source-design.md` | 已交付 | X3 | — |
| `2026-09-04-blob-object-storage-design.md` | 已交付 | Blob 多源 | connector/skill blob 化仍欠（规格非目标） |
| `2026-09-04-agent-workspace-files-design.md` | 已交付 | 会话工作区 | — |
| `2026-08-28-webhook-inbox-v1` / outbound-retry 等 | 已批准→视为已交付 | 集成闭环 | 头状态可按需再刷；功能不欠 |

---

## 2. 渠道解耦与适配器

| 规格 | 状态 | 对照 | 下一动作 |
|------|------|------|----------|
| `2026-09-06-plugin-decoupling-architecture.md` | 阶段一/二已交付；阶段三仍欠 | 注册表 + webhook；公共 Go SDK 未做 | 阶段三按需另开 |
| 计划 `2026-09-06-phase1-channel-registry.md` | 已交付（无独立规格头） | bootstrap `wireChannels` | — |
| `2026-09-06-phase2-webhook-channel-design.md` | 已交付 | `internal/channel/webhook`、`examples/im-adapter` | — |
| `2026-09-07-phase2b-weixin-adapter.md` | 已交付 | `cmd/weixin-adapter`；进程内 weixin 已移除 | — |
| `2026-09-08-adapter-supervisor-hardening-design.md` | 已交付 | watchdog / 优雅关停 / `docker-compose.weixin.yml` / `deploy/systemd` | DoD 核验见 [`2026-09-13-channel-supervisor-dod-audit.md`](2026-09-13-channel-supervisor-dod-audit.md)（通过；群聊为产品明确不做） |

---

## 3. 运行参数热更新

| 规格 | 状态 | 对照 | 下一动作 |
|------|------|------|----------|
| `2026-09-05-runtime-settings-hot-reload-design.md` | 已交付 | `PATCH /v0/settings/runtime` + 设置页 | — |

---

## 4. WebUI 体验改版

| 规格 | 状态 | 对照 | 下一动作 |
|------|------|------|----------|
| `2026-09-09-webui-experience-refresh-design.md` | 已交付（母规格） | P0–P4 + 消息组 | 见下「仍欠体验债」 |
| P0 地基 | 已交付 | 仅有计划 `…-p0-foundation.md` | — |
| `2026-09-09-webui-refresh-p1-chat.md` | 已交付 | 聊天人话 | — |
| `2026-09-10-webui-refresh-p2-settings-ia.md` | 已交付 | 设置 IA / 运营 gate | — |
| `2026-09-11-webui-refresh-p3-forms-accounts-storage-design.md` | 已交付 | P3-A | — |
| `2026-09-11-webui-refresh-p3b-connector-pages-design.md` | 已交付 | P3-B | — |
| `2026-09-12-webui-refresh-p3c-mcp-pages-design.md` | 已交付 | P3-C | MCP OAuth 等仍欠（原非目标） |
| `2026-09-12-webui-refresh-p3d-assistant-pages-design.md` | 已交付 | P3-D | 运行参数页原 defer |
| `2026-09-13-webui-refresh-p4-wrapup-design.md` | 已交付 | P4 收尾 | — |
| `2026-09-13-assistant-tools-ia-cleanup-design.md` | 已交付 | 助手功能 IA | — |
| `2026-09-13-messaging-settings-humanize-design.md` | 已交付 | 消息组原语/人话壳 | — |
| `2026-09-13-messaging-copy-outcome-design.md` | 已交付 | 结果导向文案；已去掉技术折叠 | — |
| `2026-09-13-runtime-settings-humanize-design.md` | 已交付 | UI-RUNTIME 运行参数人话 | — |

**WebUI / 体验仍欠（产品已勾）：**

| 项 | 来源 | 备注 |
|----|------|------|
| MCP OAuth 交互登录 | P3-C | **要做** |
| 登录入口 `@` 直达 / `login_required` | P3-B/D → **UI-LOGIN-AT** | **实现完成（待合入）**；[`2026-09-14-ui-login-at-design.md`](../specs/2026-09-14-ui-login-at-design.md) |
| i18n | WebUI 非目标 → 上调 | **要做**；须切片 |
| 模型思考级别开关 | P1 非目标 → 上调 | **要做** |
| `export_db_readonly` UI | P3-C → **已交付** | [`2026-09-13-ui-export-db-readonly-design.md`](../specs/2026-09-13-ui-export-db-readonly-design.md) |
| 技能可视化编辑器 | P3-D | 确认不做 |
| Playwright E2E | U2 | 确认不做 |
| 对话自动标题 U1 | OSS defer | 确认不做 |
| MCP capture UI | P3-C | **确认不做** |

---

## 5. 文档、工程与运维（仍欠 · 产品确认要做）

| 项 | 状态 | 下一动作 |
|----|------|----------|
| **DOC**（P1+P2） | **已交付** | [`2026-09-13-doc-p1-p2-design.md`](../specs/2026-09-13-doc-p1-p2-design.md)；架构草案已收口 |
| **F**（含 C1 + OPS-HOT） | **已交付** | F-C1 / F-KV / F-HOT；DoD：[`2026-09-13-f-kv-dod-audit.md`](2026-09-13-f-kv-dod-audit.md)、[`2026-09-13-f-hot-dod-audit.md`](2026-09-13-f-hot-dod-audit.md) |
| **UI-RUNTIME**（+ i18n 文案抽离附录） | 已交付 | 人话化；完整多语言见 UI-I18N |
| **UI-I18N** | 要做 · 先定范围 | 独立规格 |
| **CH-OUTBOX** | **已交付** | [`2026-09-13-ch-outbox-design.md`](../specs/2026-09-13-ch-outbox-design.md)；计划 [`2026-09-13-ch-outbox.md`](../plans/2026-09-13-ch-outbox.md)；`channel_outbox` + worker；设置 → 微信可重投 |
| **UI-LOGIN-AT** | **实现完成（待合入）** | 规格 [`2026-09-14-ui-login-at-design.md`](../specs/2026-09-14-ui-login-at-design.md)；计划 [`2026-09-14-ui-login-at.md`](../plans/2026-09-14-ui-login-at.md)；`loginentry` + `login-invoke` + WebUI `@`/`/` 与「去登录」 |
| **BLOB-CS** | 要做 | 各开规格 |
| **CH-PORT** | **已交付** | 规格 [`2026-09-13-ch-port-design.md`](../specs/2026-09-13-ch-port-design.md)；计划 [`2026-09-13-ch-port.md`](../plans/2026-09-13-ch-port.md)；`BAIZE_LISTEN` + autostart 端口文件 |
| **P6** Memory / **MCP OAuth** | 要做 · 先脑暴 | **不合并**进其他史诗 |

---

## 6. 确认不做（本版）— 2026-09-13 产品确认

**不做 / 不新立项：** A1 · B1 · P3 OTel · P5 · I2–I5 · W1/W2 · P7 · CH-SDK · 技能可视化 · Playwright · 微信群聊/语音视频 · U1 · Agent↔Tool 白名单 · MCP capture · 商业 E1–E5 · CH-WL-FORCE（已有）。

**要做史诗（合并策略 A，共 12）：** DOC · F(C1+OPS) · UI-RUNTIME(+文案抽离) · UI-I18N · UI-EXPORT-DB-RO · CH-PORT · CH-OUTBOX · UI-LOGIN-AT · LLM-THINK · BLOB-CS · MCP OAuth · P6 Memory。详见确认清单 §1。

---

## 7. 建议的下一刀候选（选题用）

1. ~~**DOC**~~（已交付）  
2. ~~**CH-PORT**~~ / ~~**CH-OUTBOX**~~（已交付）  
3. ~~**F-HOT**~~ / ~~**UI-EXPORT-DB-RO**~~ / ~~**UI-RUNTIME**~~（已交付）  
4. ~~**UI-LOGIN-AT**~~（实现完成，待合入） · **LLM-THINK** · **BLOB-CS** · **UI-I18N**  
5. **MCP OAuth** · **P6 Memory**  
6. ~~渠道 DoD / sql-store worktree~~（已完成）

---

## 8. 本账本维护

- 新规格合并/交付后：改规格头状态 + 更新本表对应行。  
- 不要只改本表不改规格头（或相反）。  
- 公开仓不同步本文件。
