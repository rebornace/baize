import type { Event } from './api'
import { foldEvents, type ChatBlock } from './foldEvents'

export type HistoryBlock = Extract<ChatBlock, { kind: 'tool' | 'workflow' | 'thinking' }>

/** @deprecated Prefer HistoryBlock; kept for call-site compatibility. */
export type ToolOrWorkflowBlock = HistoryBlock

/** 该 index 是否为同一 run_id 的第一条 assistant 消息（历史工具块只在此锚定一次）。 */
export function isFirstAssistantMessageOfRun(
  index: number,
  msgs: ReadonlyArray<{ role: string; run_id?: string | null }>,
): boolean {
  const runId = msgs[index]?.run_id
  if (!runId || msgs[index]?.role !== 'assistant') return false
  return msgs.findIndex((mm) => mm.role === 'assistant' && mm.run_id === runId) === index
}

/**
 * 从持久化 run 事件中折叠工具/流程/思考块（剔除 llm.message 等文本块），
 * 供历史回看在对应助手消息处展示，避免与已持久化的助手文本重复。
 */
export function foldToolBlocks(runId: string, events: Event[]): HistoryBlock[] {
  return foldEvents(runId, events).filter(
    (b): b is HistoryBlock =>
      b.kind === 'tool' || b.kind === 'workflow' || b.kind === 'thinking',
  )
}
