import type { ChatBlock } from './foldEvents'

export type PriorEvidenceItem = {
  name: string
  /** Compact one-line preview of the tool result (already truncated). */
  preview: string
  isError?: boolean
}

const PREVIEW_MAX = 160

/** Summarize a tool result for the HITL evidence strip. */
export function formatEvidencePreview(result: unknown): string {
  if (result === undefined || result === null) return ''
  let text: string
  if (typeof result === 'string') {
    text = result
  } else {
    try {
      text = JSON.stringify(result)
    } catch {
      text = String(result)
    }
  }
  text = text.replace(/\s+/g, ' ').trim()
  if (text.length <= PREVIEW_MAX) return text
  return text.slice(0, PREVIEW_MAX - 1) + '…'
}

/**
 * Collect successful (and error) tool results that appear before the tool block
 * at `blockIndex` in the same live/history block list. Used so HITL can show
 * what was already established before the pending write.
 */
export function collectPriorEvidence(
  blocks: ChatBlock[],
  blockIndex: number,
): PriorEvidenceItem[] {
  const out: PriorEvidenceItem[] = []
  const end = Math.max(0, Math.min(blockIndex, blocks.length))
  for (let i = 0; i < end; i++) {
    const b = blocks[i]
    if (b.kind !== 'tool') continue
    if (b.result === undefined) continue
    out.push({
      name: b.name,
      preview: formatEvidencePreview(b.result),
      isError: b.isError,
    })
  }
  return out
}
