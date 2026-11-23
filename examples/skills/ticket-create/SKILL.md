---
name: ticket-create
description: 显式建单技能（mock-ticket）；只在用户明确要求新建工单时激活
domain: ticket
tools:
  - list_tickets
  - create_ticket
---

# 显式建单

- **只在用户明确要求“新建/创建工单”时激活**。
- 只读、查询、统计、改状态请求不要激活本技能。
- 建单前必须先 `list_tickets`，再调用 `create_ticket`。
- 写操作可能触发人工审批；审批等待不算完成。
- 需要创建后再改状态的复杂链路，不要用本技能；让模型自行调度工具。
