# BLOB-CS：Connector 规格与 Skill 包进 blob.Store

> 状态：**已交付**（2026-09-15；计划 [`../plans/2026-09-15-blob-cs.md`](../plans/2026-09-15-blob-cs.md)）  
> 日期：2026-09-15  
> 史诗：BLOB-CS  
> 前置：`blob.Store` 与驱动已交付（[`2026-09-04-blob-object-storage-design.md`](2026-09-04-blob-object-storage-design.md)）；artifacts / workspaces / channel-media 已用前缀约定  
> 备注：**不**与 UI-I18N / P6 / LLM-THINK / MCP OAuth 合并；承接 2026-09-04 规格非目标「connector/skill blob 化」

---

## 1. 背景与动机

通用 `blob.Store`（file / memory / s3）已服务产物、工作区与渠道媒体。Connector OpenAPI 仍经 `specstore` 落本地 `data/connectors/`，Skill（用户上传与 managed）仍落 `data/skills/`。多机与换盘时这两类用户内容无法与对象存储对齐，且存储路径分裂。

开发阶段数据不重要：优先**干净单路径**，不要旧盘双读/双写兼容层；迁移不方便则丢弃重导即可。

## 2. 已确认产品决策

| 议题 | 决策 |
|------|------|
| 范围 | **一次做完**：Connector 规格 + 用户 Skill + managed Skill |
| builtin Skill | **不进 blob**，继续读仓库/配置的 builtin 目录 |
| 旧本地数据 | **不强制迁移器**；旧绝对路径视为无效，可打日志并要求重导 |
| 无 SQL / demo | **永远装配** `blob.Store`（至少 memory 或 file 驱动）；业务代码不处理 `Store == nil` 回退写盘 |
| API / UI | **不变**（仍 `spec_content` / FormData 上传） |
| 方案 | 改存储后端（方案 1）；不做异步同步、不做分期只迁 Connector |
| 合并 | **独立史诗** |

## 3. 成功标准

1. 导入 Connector 后，规范化规格在 blob 中可读，并能装出工具。  
2. 用户上传 Skill（md/zip）与 managed skill 写入 blob；`Catalog.Reload` 后可见可调用。  
3. builtin skill 仍从本地目录加载。  
4. 删除 Connector / 用户或 managed Skill 时删除对应 blob 前缀对象。  
5. Bootstrap 在 memory / sqlite / postgres 配置下均提供非 nil `blob.Store`。  
6. 代码路径**不再**把新写入落到 `data/connectors` / `data/skills`（除 builtin 源目录外）。

## 4. 范围

### 做

- Blob key 约定与 Put/Get/Delete/List 接线。  
- `specstore`（或等价）改为 blob；`store.Connector.Spec` 存 normalized 的 **blob key**。  
- `skill.Catalog` 用户/managed 读写改 blob；Reload = builtin 扫盘 + blob 前缀 List。  
- OpenAPI 加载：从 blob 取字节（`io.Reader`/字节 API，或短生命周期临时文件）。  
- Bootstrap：任意主 Store 驱动都挂 blob。  
- 删除路径清理 blob 对象。

### 不做

- builtin 灌入 blob。  
- 旧盘双读、异步同步、完整迁移 CLI（非必须）。  
- 聊天附件再迁、blob GC 策略、团队共享包仓库。  
- UI-I18N、改设置页交互形态。

## 5. 架构草图

```
设置页 / API（契约不变）
        │
        ▼
specstore.Write / Catalog.Install*
        │
        ▼
blob.Store
  connectors/<id>/imported.bin
  connectors/<id>/openapi.normalized.json
  skills/user/<id>/…
  skills/managed/<id>/…
        │
builtin ──仍──► ./skills（或配置 builtin 目录）
```

与既有前缀并存：`artifacts/`、`workspaces/`、`channel-media/`。

## 6. 键与数据模型

| 用途 | Key |
|------|-----|
| Connector 原始导入 | `connectors/<id>/imported.bin` |
| Connector 规范化 OpenAPI | `connectors/<id>/openapi.normalized.json` |
| 用户 Skill | `skills/user/<id>/<relpath>`（至少 `SKILL.md`） |
| Managed Skill | `skills/managed/<id>/<relpath>` |

- `Connector.Spec` = normalized blob key 字符串。  
- Skill **不**新增 SQL 表；内存 Catalog + blob List/Get。  
- 默认不加 per-skill `meta.json`（YAGNI）。

## 7. 读写流

1. **写 Connector**：Normalize（内存）→ Put imported + normalized → DB 写 Spec key。  
2. **读/装工具**：Get(Spec) → 解析；若现有加载器死要路径，仅用临时文件且用完删除。  
3. **写 Skill**：Install 内容 Put 到对应前缀 → Reload。  
4. **读 Skill**：builtin 本地；user/managed 从 blob Get。  
5. **删**：先尽量删 blob 前缀，再清 DB/索引；失败显式报错，**不**静默回退写本地盘。

## 8. 错误与依赖

- blob 失败 → API 明确错误。  
- DB 中旧绝对路径 → 加载失败 + 日志；不写复杂迁移。  
- 依赖：现有 `blob.Open` / 驱动注册；LOGIN-SKILL managed 目录改为 blob 前缀后须同步改写入点。

## 9. 测试与验收（规格级）

- Connector：Put 后装工具成功；删除后对象不存在。  
- Skill：用户 zip 安装后 Reload 可见；managed 同理；builtin 仍本地。  
- 驱动：`memory` + `file` 必测；S3 无凭证可 Skip。  
- Bootstrap：memory 主存储配置下 `blob.Store != nil`。

## 10. 后继（非本批）

- 可选一次性迁移 CLI。  
- blob 侧 GC / 引用计数。  
- 只读共享技能包仓库。

## 11. 文档与账本

- 账本 / 确认清单：BLOB-CS → **已交付**（计划 `plans/2026-09-15-blob-cs.md`）。  
- README：Connector 规格与用户/managed Skill 存于配置的对象存储（blob）；builtin Skill 仍本地。
