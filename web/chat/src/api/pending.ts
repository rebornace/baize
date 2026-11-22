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

/**
 * Consume the SSE snapshot stream. The caller handles reconnects; this function
 * resolves when the stream ends and rejects on network/HTTP errors.
 */
export async function streamPendingApprovals(
  signal: AbortSignal,
  onSnapshot: (res: PendingApprovalsResponse) => void,
): Promise<void> {
  const res = await fetch('/v0/hitl/pending/stream', { headers: authInit(), signal })
  if (!res.ok || !res.body) {
    throw new Error(`pending stream: HTTP ${res.status}`)
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) return
    buf += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n')
    let idx = buf.indexOf('\n\n')
    while (idx >= 0) {
      const block = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      const data = block
        .split('\n')
        .filter((line) => line.startsWith('data:'))
        .map((line) => line.slice(5).trim())
        .join('\n')
      if (data) {
        try {
          const parsed = JSON.parse(data) as Partial<PendingApprovalsResponse>
          const pending = Array.isArray(parsed.pending) ? parsed.pending : []
          onSnapshot({ pending, count: pending.length })
        } catch {
          // Ignore malformed events; a later snapshot will replace the list.
        }
      }
      idx = buf.indexOf('\n\n')
    }
  }
}
