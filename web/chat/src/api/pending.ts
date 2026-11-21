import { authInit, parseJSON } from './http'

export interface PendingApproval {
  run_id: string
  conversation_id: string
  conversation_title?: string
  kind: 'approval' | 'login' | 'waiting_child' | string
  tool_name?: string
  prompt?: string
  arguments?: Record<string, unknown>
  waiting_since?: string
  workflow_skill?: string
  workflow_step?: string
  origin?: string
}

export interface PendingApprovalsResponse {
  pending: PendingApproval[]
  count: number
}

export async function listPendingApprovals(): Promise<PendingApprovalsResponse> {
  const res = await fetch('/v0/hitl/pending', { headers: authInit() })
  return parseJSON<PendingApprovalsResponse>(res)
}
