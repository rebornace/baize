# DOC：架构文档与开源边界收口（P1/P2）

- 日期：2026-09-13
- 状态：已批准（2026-09-13）
- 归属：开源首版史诗 **DOC**（DOC-P1 + DOC-P2；确认清单合并策略 A1，独立于 F）
- 依据：`2026-08-28-oss-backlog-and-enterprise.md`（P1/P2 = 改文档即可）

## 1. 目标 / 非目标

### 1.1 目标

公开文档与实现/产品决策一致：

| ID | 纠正 |
|----|------|
| **DOC-P1** | 不再把官方 TS/Python SDK 列为开源交付物；表述为后续/企业或「HTTP 集成即可」 |
| **DOC-P2** | 开源工作流 DSL = 线性 `workflow.yaml`；不再把含 `branch` 的完整状态机写成开源已支持 |

范围 **B**：改 `docs/architecture-and-plugin-protocol.md`，并扫公开 `docs/*.md` + README 中英，修正同类过时句。

### 1.2 非目标

- 不实现 SDK、workflow 分支/循环/画布
- 不重写整本架构或产品定位
- 不改 `docs/superpowers` 以外的产品行为；superpowers 内仅更新 ledger/确认清单状态
- 不借机大扫除无关文档债

## 2. 必改点（架构草案）

文件：`docs/architecture-and-plugin-protocol.md`

- §1 导语「REST / SDK」→ REST / HTTP（SDK 非本版）
- §2 架构图：去掉「TS/Python SDK」；「可选状态机」→「可选线性 workflow」
- §5 表「可选 … branch」→ 线性流水线 / 见下文；branch 标非本版
- §6 开源清单去掉 TS/Python SDK；可注明官方 SDK 后续/企业

## 3. 扫一遍规则

命中则改：

- 将 **TS/Python SDK**（或「官方 SDK」）列为开源已交付 / 必含
- 将 workflow **branch / 循环 / 通用状态机** 写成开源已支持能力

不改：

- 「不用嵌 SDK」「侧车 / HTTP」正确表述
- 已写明「线性流水线」「分支用 ReAct」的段落
- Run 生命周期状态机（`queued`/`running`/…）——与 DSL 无关，保留

## 4. 验收

- 公开路径检索不再把 SDK 列为开源交付物、不再把 `branch` 列为开源 DSL
- ledger / 确认清单：**DOC 已交付**
- 无代码变更；公开仓经 export 同步架构与 README

## 5. 风险

| 风险 | 缓解 |
|------|------|
| 漏网句 | 扫完后对 `SDK`、`branch`、`状态机` 再 grep 公开 docs/README |
| 过度删减商业愿景 | §6 可保留「商业/后续」栏，只把 SDK/分支移出开源清单 |
