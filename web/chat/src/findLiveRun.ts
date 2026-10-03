import type { ChatMessage, RunStatus } from './api'
import { isTerminal } from './api'

/**
 * Pick the newest run_id that appears on messages but has no non-user
 * (assistant / system_note) message yet — typical of an in-flight or
 * waiting_human run whose terminal assistant row is not written.
 */
export function findLiveRunCandidate(messages: ChatMessage[]): string | null {
  const settled = new Set<string>()
  for (const m of messages) {
    if (m.run_id && m.role !== 'user') {
      settled.add(m.run_id)
    }
  }
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]
    if (m.run_id && !settled.has(m.run_id)) {
      return m.run_id
    }
  }
  return null
}

export function isActiveRunStatus(status: RunStatus): boolean {
  return !isTerminal(status)
}
