---
name: busabase-readonly
description: 只读查询 Busabase 类团队沉淀（表/记录）；禁止写入
tools:
  - list_tables
  - list_records
  - get_record
---

# Busabase 只读查询

**禁止**创建、更新、删除 Busabase 中的任何记录。写入请在 Busabase UI 由人完成。

1. 先用 `list_tables` 确认表名
2. 用 `list_records`（可带 `q`）检索
3. 需要详情时用 `get_record`
