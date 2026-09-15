# 开源第一版：产品需求确认清单（全量盘点）

> 日期：2026-09-13  
> 用途：产品确认「第一版还要做什么 / 明确不做」  
> 依据：`2026-08-28-oss-backlog`、`2026-09-13-spec-ledger`、各规格非目标/defer、近期 DoD 核验  
> 状态：**产品勾选已收口**；合并策略 **A（最小合并）** 已确认；史诗表见 §1

---

## 0. 已确认决策（2026-09-13）

### 首版不做

| ID | 项 |
|----|-----|
| B1 | SDK 实现（TS/Python） |
| P3 | OTel |
| P5 | 多 Agent |
| A1 | Webhook 多订阅 |
| I2–I5 | Inbox 增强 / 多 Channel 广播 |
| W1 / W2 | Workflow 分支·画布 |
| UI-SKILL-EDITOR | 技能可视化编辑器 |
| U2 | Playwright E2E |
| P7 | 飞书 / 钉钉真实适配器 |
| CH-SDK | 公共 Go SDK |
| CH-GROUP / CH-AV | 微信群聊；语音/视频 |
| U1 | 对话自动生成标题 |
| AGENT-TOOL-WL | Agent↔Tool 白名单 |
| UI-MCP-CAPTURE | MCP capture UI |
| E1–E5 | 商业多租户/SSO/厂商连接器/画布/托管记忆等 |
| CH-WL-FORCE | 白名单入站强制（**已有 / 不新立项**） |

### 产品回复原文（节选）

完整原文见 §3。合并策略回复：**A（最小合并）**。

---

## 1. 合并策略 A（已确认）→ 史诗表

**原则：** 能挂进已有史诗的挂进去；OAuth / Memory 不硬塞；实现计划仍「一目标一计划」。

| 史诗 ID | 覆盖原勾选项 | 就绪 | 备注 |
|---------|--------------|------|------|
| **DOC** | DOC-P1 + DOC-P2 | **已交付** | [`specs/2026-09-13-doc-p1-p2-design.md`](../specs/2026-09-13-doc-p1-p2-design.md) |
| **F** | F + **C1** + **OPS-HOT**（驱动热切换 / SIGHUP / 凭据 KV 加密） | **已交付** | F-HOT 计划 [`plans/2026-09-13-f-store-hot-swap.md`](../plans/2026-09-13-f-store-hot-swap.md)；DoD [`2026-09-13-f-hot-dod-audit.md`](2026-09-13-f-hot-dod-audit.md) |
| **UI-RUNTIME** | UI-RUNTIME + **i18n 文案抽离（附录）** | 已交付 | 已交付（2026-09-13）；`specs/2026-09-13-runtime-settings-humanize-design.md` |
| **UI-I18N** | i18n 框架（语言切换 / 英等） | 先定范围 | 可紧接 RUNTIME 后；独立规格 |
| **UI-EXPORT-DB-RO** | export_db_readonly UI | **已交付** | [`specs/2026-09-13-ui-export-db-readonly-design.md`](../specs/2026-09-13-ui-export-db-readonly-design.md) |
| **CH-PORT** | 动态端口 | **已交付** | 规格 [`specs/2026-09-13-ch-port-design.md`](../specs/2026-09-13-ch-port-design.md)；计划 [`plans/2026-09-13-ch-port.md`](../plans/2026-09-13-ch-port.md) |
| **CH-OUTBOX** | 渠道 outbox | **已交付** | [`specs/2026-09-13-ch-outbox-design.md`](../specs/2026-09-13-ch-outbox-design.md)；计划 [`plans/2026-09-13-ch-outbox.md`](../plans/2026-09-13-ch-outbox.md) |
| **LOGIN-SKILL**（原 **UI-LOGIN-AT**） | 连接器自动登录 Skill / login_required「去登录」 | **已交付（待合入）** | 规格 [`specs/2026-09-14-login-skill-design.md`](../specs/2026-09-14-login-skill-design.md)；计划 [`plans/2026-09-14-login-skill.md`](../plans/2026-09-14-login-skill.md)；分支 `feat/login-skill`；UI-LOGIN-AT 直达（login-entries / login-invoke / 登录专用区）已拆除 |
| **LLM-THINK** | 思考级别开关 | **已交付** | 规格 [`specs/2026-09-14-llm-think-design.md`](../specs/2026-09-14-llm-think-design.md)；计划 [`plans/2026-09-14-llm-think.md`](../plans/2026-09-14-llm-think.md)；**不合并** |
| **BLOB-CS** | Connector / Skill blob 化 | **已交付** | 规格 [`specs/2026-09-15-blob-cs-design.md`](../specs/2026-09-15-blob-cs-design.md)；计划 [`plans/2026-09-15-blob-cs.md`](../plans/2026-09-15-blob-cs.md)；**不合并** |
| **UI-MCP-OAUTH** | MCP OAuth 2.1 | **已交付** | 规格 [`specs/2026-09-14-mcp-oauth-design.md`](../specs/2026-09-14-mcp-oauth-design.md)；计划 [`plans/2026-09-14-mcp-oauth.md`](../plans/2026-09-14-mcp-oauth.md) |
| **P6** | Memory 产品化 | **已交付** | 规格 [`specs/2026-09-14-p6-memory-design.md`](../specs/2026-09-14-p6-memory-design.md)；计划 [`plans/2026-09-14-p6-memory.md`](../plans/2026-09-14-p6-memory.md)；**不合并** |

**计数：** 勾选能力仍是那些要做的点；**立项史诗 = 12**（上表 12 行）。相对「15 行清单」少掉的是：DOC 合一、C1/OPS 并入 F、i18n **抽离**挂 RUNTIME（完整 i18n 仍单独一行）。

**明确不合并：** OAuth ↛ export_db_ro；Memory ↛ 任一现有项；F/OPS ↛ UI-RUNTIME；完整 i18n ↛ @登录 / OAuth。

### 建议下一刀顺序

1. **DOC**（独立；**不**并入 F — 合并姿态 A1）  
2. **F 母篇**（仅 C1 + KV 加密 + 驱动热切换/SIGHUP；**B1** 顺序已锁）→ 可先落地 **C1**  
3. **UI-EXPORT-DB-RO**（小刀）  
4. **CH-PORT** → **CH-OUTBOX**  
5. ~~**LOGIN-SKILL**~~（已交付，待合入） · ~~**LLM-THINK**~~（已交付） · ~~**BLOB-CS**~~（已交付；见 [`plans/2026-09-15-blob-cs.md`](../plans/2026-09-15-blob-cs.md)） · **UI-I18N**  
6. ~~**UI-MCP-OAUTH**~~（已交付） · ~~**P6**~~（已交付；见 [`plans/2026-09-14-p6-memory.md`](../plans/2026-09-14-p6-memory.md)）  

**F 与现有待办：** 不吸收 OUTBOX/BLOB/PORT/OAuth/Memory/@登录/i18n/export_db_ro；DOC 保持独立（A1）。

---

## 2. 历史盘点摘要

见 `2026-09-13-spec-ledger.md` §1–§4。

---

## 3. 产品回复原文留档

```
SDK 实现：不做
OTel：不做
多 Agent：不做
Memory 产品化：要做
Webhook 多订阅：不做
Inbox 附件 / 多模态入站：不做
Inbox JSONPath / 模板映射：不做
Inbox 限速多副本一致：不做
多 Channel 广播 / 路由引擎：不做
Workflow 分支 / 循环 / 表达式：不做
Workflow 画布：不做
MCP OAuth：要做
技能可视化编辑器：不做
Playwright E2E：不做
飞书 / 钉钉真实适配器：不做
公共 Go SDK：不做
渠道 outbox：要做
动态端口：要做
```

```
DOC-P1/P2：要做
F 生产硬化：要做
UI-RUNTIME：要做
商业 E1–E5：不做
CH-WL-FORCE：已有 / 不做新立项
```

```
U1 对话自动生成标题：不做
C1 golangci / eslint：要做
登录入口 @ 直达 / login_required 卡片：要做
Connector / Skill blob 化：要做
驱动热切换 · SIGHUP · 凭据 KV 加密：要做
微信 语音 / 视频消息：不做
i18n：要做
模型「思考级别」开关：要做
export_db_readonly UI：要做
Agent↔Tool 白名单：不做
MCP capture：不做
```

```
合并策略：A（最小合并）
F×待办合并：A1（DOC 独立；F 不吸收其它史诗）
F 切片顺序：B1（C1 → KV 加密 → 热切换/SIGHUP）
C1 完成标准：C1-a（配置+CI；只修改动路径必破规则，不清全仓债）
KV 加密范围：K1（控制面口令 + 模型 api_key + webhook/inbox secret）
主密钥：M1（仅 env；未设置则 fail-closed / 拒写秘密）
热切换：H1（Store API 热切 + SIGHUP 刷 YAML；blob/S3 仍重启）
明文迁移：P1（有 key 启动时自动就地加密 K1 范围明文）
交付形态：S1（一篇母规格 + 三份独立实现计划）
```
