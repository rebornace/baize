---
name: ticket-triage
description: 工单分诊与建单流程（mock-ticket）
domain: ticket
tools:
  - list_tickets
  - get_ticket
  - create_ticket
  - update_ticket_status
---

# 工单分诊

1. 先用 list_tickets / get_ticket 了解现状。
2. **只有用户明确要求新建工单时**才调用 create_ticket；只读请求不要建单。
3. **只有用户明确要求改状态时**才调用 update_ticket_status。
4. 写操作可能触发人工审批；审批等待不算完成，不要重复提交同一写操作。
