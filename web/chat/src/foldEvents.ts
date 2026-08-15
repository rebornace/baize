import type { Event } from './api'

export type ChatBlock =
  | { kind: 'user'; text: string }
  | { kind: 'assistant'; text: string }
  | { kind: 'system'; text: string }
  | {
      kind: 'tool'
      name: string
      status: 'running' | 'waiting_human' | 'succeeded' | 'failed' | 'approved' | 'rejected'
      arguments?: unknown
      result?: unknown
      isError?: boolean
      runId: string
    }

type ToolBlock = Extract<ChatBlock, { kind: 'tool' }>

function isUnfinished(status: ToolBlock['status']): boolean {
  switch (status) {
    case 'running':
    case 'waiting_human':
    case 'approved':
      return true
    case 'succeeded':
    case 'failed':
    case 'rejected':
      return false
    default: {
      const _exhaustive: never = status
      return _exhaustive
    }
  }
}

function findTopUnfinished(
  blocks: ChatBlock[],
  name: string,
): ToolBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind === 'tool' && b.name === name && isUnfinished(b.status)) {
      return b
    }
  }
  return undefined
}

function findTopWaiting(blocks: ChatBlock[], name?: string): ToolBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind !== 'tool' || b.status !== 'waiting_human') continue
    if (name !== undefined && b.name !== name) continue
    return b
  }
  return undefined
}

function hitlToolName(data: Record<string, unknown> | undefined): string | undefined {
  if (data?.tool_name != null) return String(data.tool_name)
  if (data?.name != null) return String(data.name)
  return undefined
}

export function foldEvents(runId: string, events: Event[]): ChatBlock[] {
  const blocks: ChatBlock[] = []

  for (const ev of events) {
    const data = ev.data
    switch (ev.type) {
      case 'llm.tool_call': {
        const name = String(data?.name ?? 'tool')
        blocks.push({
          kind: 'tool',
          name,
          status: 'running',
          arguments: data?.arguments,
          runId,
        })
        break
      }
      case 'tool.result': {
        const name = String(data?.name ?? 'tool')
        const card = findTopUnfinished(blocks, name)
        const isError = Boolean(data?.is_error)
        if (card) {
          card.status = isError ? 'failed' : 'succeeded'
          card.result = data?.content
          card.isError = isError
        } else {
          blocks.push({
            kind: 'tool',
            name,
            status: isError ? 'failed' : 'succeeded',
            result: data?.content,
            isError,
            runId,
          })
        }
        break
      }
      case 'hitl.waiting': {
        const name = hitlToolName(data) ?? 'tool'
        const card = findTopUnfinished(blocks, name)
        if (card) {
          card.status = 'waiting_human'
          if (data?.arguments !== undefined) card.arguments = data.arguments
        } else {
          blocks.push({
            kind: 'tool',
            name,
            status: 'waiting_human',
            arguments: data?.arguments,
            runId,
          })
        }
        break
      }
      case 'hitl.resumed': {
        const name = hitlToolName(data)
        // Named: only same-name waiting card (no cross-tool fallback).
        // Unnamed: fall back to topmost waiting card.
        const card =
          name !== undefined
            ? findTopWaiting(blocks, name)
            : findTopWaiting(blocks)
        if (card) card.status = 'approved'
        break
      }
      case 'hitl.rejected': {
        const name = hitlToolName(data)
        const card =
          name !== undefined
            ? findTopWaiting(blocks, name)
            : findTopWaiting(blocks)
        if (card) card.status = 'rejected'
        break
      }
      case 'llm.message': {
        const content = String(data?.content ?? '')
        if (content) blocks.push({ kind: 'assistant', text: content })
        break
      }
      case 'llm.error': {
        blocks.push({ kind: 'system', text: String(data?.error ?? '未知错误') })
        break
      }
      default:
        break
    }
  }

  return blocks
}
