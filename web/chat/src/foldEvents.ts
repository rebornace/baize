import type { Event } from './api'

export type WorkflowStepStatus = 'pending' | 'running' | 'done' | 'failed'

export type ChatBlock =
  | { kind: 'user'; text: string }
  | { kind: 'assistant'; text: string }
  | { kind: 'system'; text: string }
  | {
      kind: 'thinking'
      turn: number
      text: string
      status: 'streaming' | 'done' | 'redacted'
      collapsed: boolean
    }
  | {
      kind: 'tool'
      name: string
      status: 'running' | 'waiting_human' | 'succeeded' | 'failed' | 'approved' | 'rejected'
      arguments?: unknown
      result?: unknown
      isError?: boolean
      runId: string
    }
  | {
      kind: 'workflow'
      skill: string
      steps: { id: string; status: WorkflowStepStatus }[]
      runId: string
    }

type ToolBlock = Extract<ChatBlock, { kind: 'tool' }>
type WorkflowBlock = Extract<ChatBlock, { kind: 'workflow' }>
type ThinkingBlock = Extract<ChatBlock, { kind: 'thinking' }>

function findThinkingByTurn(blocks: ChatBlock[], turn: number): ThinkingBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind === 'thinking' && b.turn === turn) return b
  }
  return undefined
}

function upsertThinking(
  blocks: ChatBlock[],
  turn: number,
  patch: Partial<Pick<ThinkingBlock, 'text' | 'status' | 'collapsed'>>,
): ThinkingBlock {
  const existing = findThinkingByTurn(blocks, turn)
  if (existing) {
    if (patch.text !== undefined) existing.text = patch.text
    if (patch.status !== undefined) existing.status = patch.status
    if (patch.collapsed !== undefined) existing.collapsed = patch.collapsed
    return existing
  }
  const created: ThinkingBlock = {
    kind: 'thinking',
    turn,
    text: patch.text ?? '',
    status: patch.status ?? 'streaming',
    collapsed: patch.collapsed ?? false,
  }
  blocks.push(created)
  return created
}

function upsertAssistant(blocks: ChatBlock[], text: string): void {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind === 'assistant') {
      b.text = text
      return
    }
  }
  blocks.push({ kind: 'assistant', text })
}

function eventTurn(data: Record<string, unknown> | undefined): number {
  const raw = data?.turn
  const n = typeof raw === 'number' ? raw : Number(raw)
  return Number.isFinite(n) ? n : 0
}

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

function findLastWorkflow(blocks: ChatBlock[]): WorkflowBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind === 'workflow') return b
  }
  return undefined
}

function workflowStepIds(data: Record<string, unknown> | undefined): string[] {
  const raw = data?.steps
  if (!Array.isArray(raw)) return []
  return raw.map((id) => String(id))
}

function setWorkflowStepStatus(
  block: WorkflowBlock,
  stepId: string,
  status: WorkflowStepStatus,
): void {
  const step = block.steps.find((s) => s.id === stepId)
  if (step) step.status = status
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
      case 'llm.thinking.delta': {
        const turn = eventTurn(data)
        upsertThinking(blocks, turn, {
          text: String(data?.text ?? ''),
          status: 'streaming',
          collapsed: false,
        })
        break
      }
      case 'llm.thinking': {
        const turn = eventTurn(data)
        const redacted = Boolean(data?.thinking_redacted)
        upsertThinking(blocks, turn, {
          text: String(data?.text ?? ''),
          status: redacted ? 'redacted' : 'done',
          collapsed: redacted ? true : undefined,
        })
        break
      }
      case 'llm.content.delta': {
        const turn = eventTurn(data)
        const thinking = findThinkingByTurn(blocks, turn)
        if (thinking) {
          thinking.status = thinking.status === 'redacted' ? 'redacted' : 'done'
          thinking.collapsed = true
        }
        upsertAssistant(blocks, String(data?.text ?? ''))
        break
      }
      case 'llm.message': {
        const content = String(data?.content ?? '')
        if (content) upsertAssistant(blocks, content)
        break
      }
      case 'llm.error': {
        blocks.push({ kind: 'system', text: String(data?.error ?? '未知错误') })
        break
      }
      case 'workflow.started': {
        blocks.push({
          kind: 'workflow',
          skill: String(data?.skill ?? 'workflow'),
          steps: workflowStepIds(data).map((id) => ({ id, status: 'pending' as const })),
          runId,
        })
        break
      }
      case 'workflow.step_started': {
        const block = findLastWorkflow(blocks)
        const stepId = String(data?.step ?? '')
        if (block && stepId) setWorkflowStepStatus(block, stepId, 'running')
        break
      }
      case 'workflow.step_completed': {
        const block = findLastWorkflow(blocks)
        const stepId = String(data?.step ?? '')
        if (block && stepId) {
          setWorkflowStepStatus(block, stepId, data?.is_error ? 'failed' : 'done')
        }
        break
      }
      default:
        break
    }
  }

  return blocks
}
