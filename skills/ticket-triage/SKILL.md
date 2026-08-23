---
name: ticket-triage
description: 工单分诊与建单流程（mock-ticket）
tools:
  - list_tickets
  - get_ticket
  - create_ticket
  - update_ticket_status
---

# 工单分诊

1. 先用 list_tickets / get_ticket 了解现状
2. 需要新建时再 create_ticket（可能触发人工审批）
3. 改状态用 update_ticket_status（可能触发人工审批）
