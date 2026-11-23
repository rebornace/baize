---
name: busabase-readonly
description: 只读查询 Busabase 类团队沉淀（节点/记录检索）；禁止写入
tools:
  - workbench.search
  - workbench.nodes.list
  - workbench.nodes.searchByName
  - workbench.nodes.get
  - workbench.records.search
  - workbench.records.listPage
  - workbench.records.get
  - workbench.auth.verify
  - system.health
---

# Busabase 只读查询

**禁止**创建、更新、删除 Busabase 中的任何记录。写入请在 Busabase UI 由人完成。

1. 用 `workbench.search` 或 `workbench.nodes.searchByName` / `workbench.nodes.list` 定位节点与上下文
2. 用 `workbench.records.search` 或 `workbench.records.listPage` 检索记录
3. 需要单条详情时用 `workbench.nodes.get` 或 `workbench.records.get`
4. 鉴权或连通性异常时可调用 `workbench.auth.verify`、`system.health` 辅助排查（仍只读）
