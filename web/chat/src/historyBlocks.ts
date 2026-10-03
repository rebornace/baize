import type { Event } from './api'
import { foldEvents, type ChatBlock, type UsageMeta } from './foldEvents'

export type HistoryBlock = Extract<ChatBlock, { kind: 'tool' | 'workflow' | 'thinking' }>

/** @deprecated Prefer HistoryBlock; kept for call-site compatibility. */
export type ToolOrWorkflowBlock = HistoryBlock

// emptyUsage is the "nothing to report" marker.
const emptyUsage: UsageMeta = {
  promptTokens: 0,
  completionTokens: 0,
  totalTokens: 0,
  savedTokens: 0,
  cachedTokens: 0,
}

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

/**
 * 从持久化 run 事件中聚合 run 级 token 用量与记忆抽取节省量，供历史回看
 * 附加到该 run 的助手消息。无任何可报告数据时返回 undefined。
 */
export function runUsageFromEvents(runId: string, events: Event[]): UsageMeta | undefined {
  const folded = foldEvents(runId, events)
  for (let i = folded.length - 1; i >= 0; i--) {
    const b = folded[i]
    if (b.kind === 'assistant' && b.usage) return b.usage
  }
  // No assistant block carried usage; surface savings recorded on their own.
  let saved = 0
  for (const ev of events) {
    if (ev.type === 'memory.extract_skipped') {
      const n = Number(ev.data?.saved_tokens)
      if (Number.isFinite(n)) saved += n
    }
  }
  if (saved <= 0) return undefined
  return { ...emptyUsage, savedTokens: saved }
}
