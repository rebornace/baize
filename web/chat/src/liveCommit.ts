import type { ChatMessage, Event } from './api'
import { foldEvents } from './foldEvents'

export function lastLiveAssistantText(runId: string, events: Event[]): string {
  const blocks = foldEvents(runId, events)
  let text = ''
  for (const b of blocks) {
    if (b.kind === 'assistant' && b.text.trim()) text = b.text
  }
  return text
}

/** Keep the live answer on screen when the SSE fold is torn down. */
export function commitLiveAssistant(
  prev: ChatMessage[],
  args: { conversationId: string; runId: string | null; events: Event[] },
): ChatMessage[] {
  const { conversationId, runId, events } = args
  if (!runId) return prev
  if (prev.some((m) => m.role === 'assistant' && m.run_id === runId && m.content.trim())) {
    return prev
  }
  const text = lastLiveAssistantText(runId, events)
  if (!text.trim()) return prev
  return [
    ...prev,
    {
      id: `local_asst_${runId}`,
      conversation_id: conversationId,
      role: 'assistant',
      content: text,
      run_id: runId,
      created_at: new Date().toISOString(),
    },
  ]
}

/** Prefer server rows, but do not drop a live snapshot if persist is still behind. */
export function adoptServerMessages(
  prev: ChatMessage[],
  server: ChatMessage[],
  runId: string | null,
): ChatMessage[] {
  if (!runId) return server
  const serverHas = server.some(
    (m) => m.role === 'assistant' && m.run_id === runId && m.content.trim(),
  )
  if (serverHas) return server
  const local = [...prev]
    .reverse()
    .find((m) => m.role === 'assistant' && m.run_id === runId && m.content.trim())
  if (!local) return server
  return [...server, local]
}

/** Server rows plus a live snapshot if persist is still behind. */
export function handoffTranscript(
  prev: ChatMessage[],
  server: ChatMessage[],
  args: { conversationId: string; runId: string | null; events: Event[] },
): ChatMessage[] {
  return adoptServerMessages(commitLiveAssistant(prev, args), server, args.runId)
}

export function messageRowKey(
  m: ChatMessage,
  index: number,
  all: ReadonlyArray<ChatMessage>,
): string {
  if (m.role === 'user' && m.run_id) return `user-${m.run_id}`
  if (m.role === 'assistant' && m.run_id) {
    let n = 0
    for (let i = 0; i <= index; i++) {
      const row = all[i]
      if (row.role === 'assistant' && row.run_id === m.run_id) n += 1
    }
    return `asst-${m.run_id}-${n}`
  }
  return m.id
}

/** Same identity as persisted `asst-${runId}-n` so finish does not remount the answer. */
export function liveAssistantKey(
  runId: string,
  blocks: ReadonlyArray<{ kind: string }>,
  index: number,
): string {
  let n = 0
  for (let i = 0; i <= index; i++) {
    if (blocks[i]?.kind === 'assistant') n += 1
  }
  return `asst-${runId}-${n}`
}
