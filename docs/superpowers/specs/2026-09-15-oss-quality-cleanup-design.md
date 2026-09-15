# OSS 质量收口：规范、契约、结构、门禁与可证性能

> 状态：**进行中**（AUDIT 已确认；**CONTRACT 已交付**；STRUCT / GATES / PERF-HOT 待实现）  
> 日期：2026-09-15  
> 史诗：CLEAN（分阶段：AUDIT → CONTRACT → STRUCT → GATES → PERF-HOT）  
> 前置：开源首版 12 史诗均已交付；产品确认零外部用户，允许破坏性契约清理  
> 备注：非新功能；目标为干净可维护的开源首版表面  
> 实现计划：AUDIT · CONTRACT（已交付）· STRUCT P0 [`../plans/2026-09-15-clean-struct.md`](../plans/2026-09-15-clean-struct.md)（已交付）· STRUCT-P1 [`../plans/2026-09-15-clean-struct-p1.md`](../plans/2026-09-15-clean-struct-p1.md) · GATES [`../plans/2026-09-15-clean-gates.md`](../plans/2026-09-15-clean-gates.md) · PERF 另开

---

## 1. 背景与动机

功能史诗已齐，但工程上仍有：

- CI golangci 使用 `only-new-issues`，存量债被刻意搁置  
- 超大包/文件（如 `internal/api`、`ChatPage.tsx`、`api.ts`）提高贡献门槛  
- 公开文档（`docs/architecture-and-plugin-protocol.md`、`docs/deployment.md`）偏技术堆砌，不适合产品/运营读者，也未与「开发者专档」分层  
- 零用户窗口是一次把公开契约做干净、再以门禁锁住的机会  

本史诗按阶段横切推进，避免单 PR 大搬家。

## 2. 已确认产品决策

| 议题 | 决策 |
|------|------|
| 推进方式 | **方案 3：分阶段横切**（非单史诗一把梭、非按层竖切） |
| 风险 | **零用户** → 允许破坏公开契约；删除优先于兼容别名 |
| 成功标准 | **贡献者门槛 + 债务清零证明**；性能仅修有证据热点 |
| 协议 | **本轮包含** HTTP / 配置 / CLI / 前端客户端对齐清理 |
| README | **产品/运营向**：亮点、核心能力、场景案例；性能图/表仅用实测 |
| 技术文档 | **另开开发者文档**；不与 README 混写 |
| 旧公开 docs | **删除** `architecture-and-plugin-protocol.md`、`deployment.md`；若仍需则按真实现状重写进开发者文档 |
| 覆盖率门槛 | **不上**强制覆盖率挡合并 |
| E2E | Playwright **不做**（既有产品确认） |

## 3. 成功标准

1. **贡献者门槛：** 新人按 README 能理解产品并试用；按开发者文档能构建、测、贡献；关键大文件/包已拆到可审阅或有「本版保留」理由。  
2. **债务清零证明：** CI 全量 golangci（非 only-new）绿；前端 lint / tsc / vitest 无不合理豁免；废弃协议与兼容别名按白名单清零。  
3. **契约一致：** 服务端、`web/chat` 客户端、README/开发者文档、配置示例一致；破坏性变更有 changelog/Breaking 说明。  
4. **性能：** 仅 profiling/可复现基准证明的热点有改动；README 图表只引用这些数据，无数据则不编对比。  

## 4. 阶段设计

### 4.1 CLEAN-AUDIT（只读盘点）

**目标：** 产出可执行清单，不改行为。

**盘点面：**

- 公开面：HTTP `/v0/*`、配置键、CLI/env、webhook 渠道协议、WebUI 路由与 `api.ts`  
- 债信号：`@deprecated`、双写/别名、兼容分支、lint 全量基线、超大文件/包、死代码、过期公开 docs  
- 性能候选：标注「值得测」路径（聊天流式、会话读写、blob、渠道出站）；本阶段不测不改  

**产出：**

1. 契约白名单表：`保留` / `重命名` / `删除` + 理由  
2. 结构热点表：路径、体量、建议拆法  
3. 门禁基线：全量 golangci / eslint 问题计数  
4. 明确不做列表（本轮）  

**门禁：** 清单经产品确认后，才进入 CONTRACT。

### 4.2 CLEAN-CONTRACT（公开契约 + 文档分层骨架）

**目标：** 公开形状一次做干净。

**原则：** 以 AUDIT 白名单为准；删除优先于别名；配置 / HTTP / CLI / `api.ts` 同刀；清单外不顺手改。

**允许：** 路径与字段重命名、删半成品 API、删 `@deprecated` 垫片、删无引用直达、更新示例与 Breaking 说明。

**禁止：** 借机加功能；「说不定以后要」的兼容层；未说明的语义偷换。

**文档（公开仓）：**

| 文档 | 读者 | 内容 |
|------|------|------|
| README（中/英） | 产品、运营、决策者 | 是什么、亮点、核心能力（人话）、场景/案例、试用入口；性能区先留位或「待 PERF-HOT 回填」 |
| 开发者文档（新建，如 `docs/developers/` 或单文件 guide） | 贡献者/集成方 | 构建、配置、契约/API、目录地图、CI、贡献流程 |
| 删除 | — | `docs/architecture-and-plugin-protocol.md`、`docs/deployment.md`；需要则重写进开发者文档 |

**DoD：** 废弃别名清零（或白名单注明保留为产品能力）；测试绿；文档与代码一致。

### 4.3 CLEAN-STRUCT（内部瘦身）

**目标：** 降贡献成本；行为以测试为准。

**优先热点（以 AUDIT 复核为准）：** `internal/api`、`internal/store`、`internal/run`、`internal/bootstrap`；`ChatPage.tsx`、`api.ts`、偏大 Settings 页。

**做法：** 按职责拆文件/子包；去重复；删仅服务旧契约的死代码。`internal/...` 导入在更清晰时可改（零用户）。

**禁止：** 无测大搬家；顺便换框架；夹带无关行为微调。

**DoD：** 热点表勾选完成或标明保留理由；测试绿；开发者文档有目录地图。

### 4.4 CLEAN-GATES（门禁收口）

**在 CONTRACT/STRUCT 清完后拧紧**，避免边拆边红。

- 去掉 golangci `only-new-issues`；全量必须过  
- 保持 gofmt / vet / test / build；web lint / tsc / test  
- 清理不合理 `eslint-disable` / `as any`  
- 更新 `.golangci.yml` 过期「全仓清债不在本刀」叙述  
- 贡献命令与 CI 在开发者文档中一致  

**不做：** 强制覆盖率挡合并；Playwright。

**DoD：** `main` CI 全量绿 = 可重复的债清零证明。

### 4.5 PERF-HOT（可证性能 + README 回填）

1. 按 AUDIT 候选固定环境与脚本（开发者文档可复现）  
2. 基线 → 只改差异显著且不伤可读性的点 → 再测  
3. **有前后数据才写入 README**；无显著收益则记录「已测、保持现状」  

**禁止：** 无证据优化；伪造竞品碾压。

## 5. 阶段依赖

```
AUDIT → CONTRACT（文档分层骨架）→ STRUCT → GATES → PERF-HOT（回填 README 图表）
```

前一阶段 DoD 未过，不开下一阶段实现计划的大规模改动（AUDIT 清单确认除外）。

## 6. 总非目标

- 新功能史诗；插件公共 Go SDK；OTel；Playwright；多 Agent 等已确认不做项  
- 为 i18n/皮肤再开大改（漏译当 bug）  
- 无法复现的性能叙事  
- 单 PR 合并全部五阶段  

## 7. 测试与验收（规格级）

- 每阶段：`go test ./...`、`web/chat` lint + test + tsc（或该阶段约定子集）绿  
- GATES：CI 配置变更后在 PR 上验证全量 golangci  
- CONTRACT：抽查白名单「删除」项在代码与 OpenAPI/客户端中不可达  
- PERF：基准脚本可重复；README 数字可追溯到脚本输出  

## 8. 风险与回滚

- 破坏性契约：零用户可接受；若中途有早期试用者，以 changelog 为唯一迁移说明，不做长期双写  
- 结构拆分：小步 PR、保持测试；单阶段可回滚该阶段提交  
- 门禁过早拧紧：必须放在 STRUCT 之后，避免噪音阻塞有效重构  

## 9. 账本与公开仓

- 本规格仅私有仓 `docs/superpowers`；不导出公开仓  
- 公开仓文档变更落在 CONTRACT/PERF 的实现提交中（README、开发者文档、删除旧 docs）  
---

## 10. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-09-15 | 初稿：方案 3 + 文档分层 + 零用户破契约；设计评审通过 |
| 2026-09-15 | CLEAN-CONTRACT 交付：客户端死导出删除、公开 docs 分层、产品向 README；测试绿 |
