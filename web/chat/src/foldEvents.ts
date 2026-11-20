import type { Event } from './api'

export type WorkflowStepStatus = 'pending' | 'waiting' | 'running' | 'done' | 'failed'

export type UsageMeta = {
  promptTokens: number
  completionTokens: number
  totalTokens: number
  savedTokens: number
  cachedTokens: number
}

export type ChatBlock =
  | { kind: 'user'; text: string }
  | {
      kind: 'assistant'
      text: string
      usage?: UsageMeta
      /** True while llm.content.delta is still arriving; typewriter only in this window. */
      streaming?: boolean
    }
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
      /** Timestamp of the last hitl.waiting event, for "waited N" display. */
      waitingSince?: string
      /** Workflow step ID when this card belongs to a workflow wait. */
      workflowStep?: string
      /** Human gate kind for a waiting card; login keeps the login actions. */
      hitlKind?: 'approval' | 'login'
      contractHash?: string
      catalogRevision?: number
    }
  | {
      kind: 'workflow'
      skill: string
      steps: {
        id: string
        status: WorkflowStepStatus
        waitingKind?: 'approval' | 'login'
      }[]
      runId: string
    }
  | {
      kind: 'subagent'
      childRunId: string
      goal: string
      groupId?: string
      mode?: string
      async?: boolean
      status: 'running' | 'waiting' | 'succeeded' | 'failed' | 'cancelled' | 'rejected'
      waitingKind?: 'approval' | 'login'
      summary?: string
      facts?: Record<string, unknown>
      error?: string
      runId: string
    }

type ToolBlock = Extract<ChatBlock, { kind: 'tool' }>
type WorkflowBlock = Extract<ChatBlock, { kind: 'workflow' }>
type ThinkingBlock = Extract<ChatBlock, { kind: 'thinking' }>
type AssistantBlock = Extract<ChatBlock, { kind: 'assistant' }>

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

function findLastAssistant(blocks: ChatBlock[]): AssistantBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i].kind === 'assistant') return blocks[i] as AssistantBlock
  }
  return undefined
}

function lastToolOrWorkflowIndex(blocks: ChatBlock[]): number {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const k = blocks[i].kind
    if (k === 'tool' || k === 'workflow') return i
  }
  return -1
}

function upsertAssistant(blocks: ChatBlock[], text: string, streaming?: boolean): void {
  // After tools, a later LLM turn must become a new bubble. Overwriting the
  // last assistant (often sitting *above* the tool cards) makes live order
  // look reversed until history re-anchors tools above persisted text.
  const after = lastToolOrWorkflowIndex(blocks)
  for (let i = blocks.length - 1; i > after; i--) {
    const b = blocks[i]
    if (b.kind === 'assistant') {
      b.text = text
      if (streaming !== undefined) b.streaming = streaming
      return
    }
  }
  const next: AssistantBlock = { kind: 'assistant', text }
  if (streaming !== undefined) next.streaming = streaming
  blocks.push(next)
}

// applyUsageMeta writes the run-level usage summary onto the terminal
// assistant block when there is anything to report.
function applyUsageMeta(blocks: ChatBlock[], meta: UsageMeta): void {
  if (meta.totalTokens <= 0 && meta.savedTokens <= 0) return
  const asst = findLastAssistant(blocks)
  if (!asst) return
  asst.usage = { ...meta }
}

function numField(data: Record<string, unknown> | undefined, key: string): number {
  const n = Number(data?.[key])
  return Number.isFinite(n) ? n : 0
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

function findTopByName(blocks: ChatBlock[], name: string): ToolBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind === 'tool' && b.name === name) return b
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

function findSubagent(
  blocks: ChatBlock[],
  childRunId: string,
): Extract<ChatBlock, { kind: 'subagent' }> | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind === 'subagent' && b.childRunId === childRunId) return b
  }
  return undefined
}

function removeLastSpawnToolCard(blocks: ChatBlock[]): void {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.kind === 'tool' && b.name === 'spawn_group_run' && b.status === 'running') {
      blocks.splice(i, 1)
      return
    }
  }
}

function subagentFacts(data: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  const raw = data?.facts
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return undefined
  return raw as Record<string, unknown>
}

function workflowStepFromData(data: Record<string, unknown> | undefined): string | undefined {
  const raw = data?.workflow_step
  return typeof raw === 'string' && raw.trim() ? raw.trim() : undefined
}

function markWorkflowStepForCard(
  blocks: ChatBlock[],
  card: ToolBlock | undefined,
  status: WorkflowStepStatus,
): void {
  if (!card?.workflowStep) return
  const wf = findLastWorkflow(blocks)
  if (wf) setWorkflowStepStatus(wf, card.workflowStep, status)
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
  if (!step) return
  step.status = status
  if (status !== 'waiting') delete step.waitingKind
}

function setWorkflowStepWaiting(
  block: WorkflowBlock,
  stepId: string,
  kind: 'approval' | 'login',
): void {
  const step = block.steps.find((s) => s.id === stepId)
  if (!step) return
  step.status = 'waiting'
  step.waitingKind = kind
}

export function foldEvents(runId: string, events: Event[]): ChatBlock[] {
  const blocks: ChatBlock[] = []
  // Run-level totals accumulated across all turns. llm.usage (tool-call turns)
  // arrives before the terminal llm.message; memory.extract_skipped arrives
  // after it. Both accumulate here and are flushed onto the assistant block.
  const meta: UsageMeta = { promptTokens: 0, completionTokens: 0, totalTokens: 0, savedTokens: 0, cachedTokens: 0 }

  for (const ev of events) {
    const data = ev.data
    switch (ev.type) {
      case 'llm.usage': {
        meta.promptTokens += numField(data, 'prompt_tokens')
        meta.completionTokens += numField(data, 'completion_tokens')
        meta.totalTokens += numField(data, 'total_tokens')
        meta.cachedTokens += numField(data, 'cached_tokens')
        applyUsageMeta(blocks, meta)
        break
      }
      case 'memory.extract_skipped': {
        meta.savedTokens += numField(data, 'saved_tokens')
        applyUsageMeta(blocks, meta)
        break
      }
      case 'llm.tool_call': {
        // Model left generation for tools — snap any in-flight typewriter.
        for (const b of blocks) {
          if (b.kind === 'assistant' && b.streaming) b.streaming = false
        }
        const name = String(data?.name ?? 'tool')
        blocks.push({
          kind: 'tool',
          name,
          status: 'running',
          arguments: data?.arguments,
          runId,
          contractHash: typeof data?.contract_hash === 'string' ? data.contract_hash : undefined,
          catalogRevision: typeof data?.catalog_revision === 'number' ? data.catalog_revision : undefined,
        })
        break
      }
      case 'tool.result': {
        const name = String(data?.name ?? 'tool')
        if (name === 'spawn_group_run') {
          const content = data?.content as { child_run_id?: unknown } | undefined
          const childRunId =
            typeof content?.child_run_id === 'string' ? content.child_run_id : ''
          if (childRunId && findSubagent(blocks, childRunId)) break
        }
        const card = findTopUnfinished(blocks, name)
        const isError = Boolean(data?.is_error)
        const hash = typeof data?.contract_hash === 'string' ? data.contract_hash : undefined
        const rev = typeof data?.catalog_revision === 'number' ? data.catalog_revision : undefined
        if (card) {
          card.status = isError ? 'failed' : 'succeeded'
          card.result = data?.content
          card.isError = isError
          if (hash) card.contractHash = hash
          if (rev !== undefined) card.catalogRevision = rev
        } else {
          blocks.push({
            kind: 'tool',
            name,
            status: isError ? 'failed' : 'succeeded',
            result: data?.content,
            isError,
            runId,
            contractHash: hash,
            catalogRevision: rev,
          })
        }
        break
      }
      case 'hitl.waiting': {
        // The parent waits on a child receipt; the subagent card carries it.
        if (data?.kind === 'waiting_child') break
        const kind = data?.kind === 'login' ? 'login' : 'approval'
        const name = hitlToolName(data) ?? 'tool'
        const workflowStep = workflowStepFromData(data)
        if (kind === 'login') {
          // Login waits keep the existing login_required tool card; do not
          // swap it for an approve/reject HITL card. The card can already be
          // marked failed by the login_required tool.result, so match by name.
          const card = findTopByName(blocks, name)
          if (card) {
            card.status = 'waiting_human'
            card.waitingSince = ev.timestamp
            card.hitlKind = 'login'
            if (workflowStep) card.workflowStep = workflowStep
          }
        } else {
          const card = findTopUnfinished(blocks, name)
          if (card) {
            card.status = 'waiting_human'
            card.waitingSince = ev.timestamp
            if (workflowStep) card.workflowStep = workflowStep
            if (data?.arguments !== undefined) card.arguments = data.arguments
          } else {
            blocks.push({
              kind: 'tool',
              name,
              status: 'waiting_human',
              arguments: data?.arguments,
              runId,
              waitingSince: ev.timestamp,
              workflowStep,
            })
          }
        }
        if (workflowStep) {
          const wf = findLastWorkflow(blocks)
          if (wf) setWorkflowStepWaiting(wf, workflowStep, kind)
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
        if (card) {
          card.status = 'approved'
          if (card.hitlKind === 'login' || data?.kind === 'login') card.result = undefined
          markWorkflowStepForCard(blocks, card, 'running')
        }
        break
      }
      case 'hitl.rejected': {
        const name = hitlToolName(data)
        const card =
          name !== undefined
            ? findTopWaiting(blocks, name)
            : findTopWaiting(blocks)
        if (card) {
          card.status = 'rejected'
          if (card.hitlKind === 'login' || data?.kind === 'login') card.result = undefined
          markWorkflowStepForCard(blocks, card, 'failed')
        }
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
        upsertAssistant(blocks, String(data?.text ?? ''), true)
        break
      }
      case 'llm.message': {
        const content = String(data?.content ?? '')
        // Final message: mark streaming done so the UI snaps (no fake typewriter
        // over tool results / already-complete text).
        if (content) upsertAssistant(blocks, content, false)
        meta.promptTokens += numField(data, 'prompt_tokens')
        meta.completionTokens += numField(data, 'completion_tokens')
        meta.totalTokens += numField(data, 'total_tokens')
        meta.cachedTokens += numField(data, 'cached_tokens')
        applyUsageMeta(blocks, meta)
        break
      }
      case 'llm.error': {
        for (const b of blocks) {
          if (b.kind === 'subagent' && (b.status === 'running' || b.status === 'waiting')) {
            b.status = 'failed'
          }
        }
        blocks.push({ kind: 'system', text: String(data?.error ?? '未知错误') })
        break
      }
      case 'run.spawned': {
        const childRunId = String(data?.child_run_id ?? '')
        if (!childRunId) break
        removeLastSpawnToolCard(blocks)
        const existing = findSubagent(blocks, childRunId)
        if (existing) {
          existing.status = 'running'
          existing.waitingKind = undefined
          if (data?.async) existing.async = true
          break
        }
        blocks.push({
          kind: 'subagent',
          childRunId,
          goal: String(data?.goal ?? ''),
          groupId: typeof data?.group_id === 'string' ? data.group_id : undefined,
          mode: typeof data?.mode === 'string' ? data.mode : undefined,
          async: Boolean(data?.async),
          status: 'running',
          runId,
        })
        break
      }
      case 'run.child_waiting': {
        const childRunId = String(data?.child_run_id ?? '')
        const block = childRunId ? findSubagent(blocks, childRunId) : undefined
        if (!block) break
        block.status = 'waiting'
        block.waitingKind = data?.kind === 'login' ? 'login' : 'approval'
        break
      }
      case 'run.child_resumed': {
        const childRunId = String(data?.child_run_id ?? '')
        const block = childRunId ? findSubagent(blocks, childRunId) : undefined
        if (!block) break
        block.status = 'running'
        block.waitingKind = undefined
        break
      }
      case 'run.receipt': {
        const childRunId = String(data?.child_run_id ?? '')
        if (!childRunId) break
        const existing = findSubagent(blocks, childRunId)
        const status = String(data?.status ?? '')
        const block =
          existing ??
          ({
            kind: 'subagent',
            childRunId,
            goal: '',
            status: 'running',
            runId,
          } as Extract<ChatBlock, { kind: 'subagent' }>)
        if (!existing) blocks.push(block)
        block.status =
          status === 'succeeded' ||
          status === 'failed' ||
          status === 'cancelled' ||
          status === 'rejected'
            ? status
            : 'running'
        block.waitingKind = undefined
        block.summary = typeof data?.summary === 'string' ? data.summary : undefined
        block.facts = subagentFacts(data)
        block.error = typeof data?.error === 'string' && data.error ? data.error : undefined
        if (!block.groupId && typeof data?.group_id === 'string') block.groupId = data.group_id
        if (!block.mode && typeof data?.mode === 'string') block.mode = data.mode
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
      case 'hitl.expired':
      case 'run.cancelled': {
        if (ev.type === 'run.cancelled') {
          for (const b of blocks) {
            if (b.kind === 'subagent' && (b.status === 'running' || b.status === 'waiting')) {
              b.status = 'cancelled'
            }
          }
        }
        const card = findTopWaiting(blocks)
        if (card) {
          if (card.hitlKind === 'login' || data?.kind === 'login') card.result = undefined
          markWorkflowStepForCard(blocks, card, 'failed')
        }
        break
      }
      default:
        break
    }
  }

  return blocks
}
