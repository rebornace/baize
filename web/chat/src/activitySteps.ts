import type { ChatBlock } from './foldEvents'
import type { HistoryBlock } from './historyBlocks'

export type ActivityStepBlock = Extract<ChatBlock, { kind: 'tool' | 'workflow' }>
export type ActivityThinkingBlock = Extract<ChatBlock, { kind: 'thinking' }>
export type LiveChromeBlock = Exclude<ChatBlock, ActivityStepBlock | ActivityThinkingBlock>

export type LiveActivityItem =
  | { kind: 'step'; block: ActivityStepBlock; index: number }
  | { kind: 'thinking'; block: ActivityThinkingBlock; index: number }

export type LiveActivitySegment =
  | { type: 'chrome'; block: LiveChromeBlock; index: number }
  | { type: 'thinking'; block: ActivityThinkingBlock; index: number }
  | { type: 'steps'; items: LiveActivityItem[]; stepBlocks: ActivityStepBlock[] }

export type HistoryActivityItem =
  | { kind: 'step'; block: ActivityStepBlock }
  | { kind: 'thinking'; block: ActivityThinkingBlock }

export type HistoryActivitySegment =
  | { type: 'thinking'; block: ActivityThinkingBlock }
  | { type: 'steps'; items: HistoryActivityItem[]; stepBlocks: ActivityStepBlock[] }

function isActivityKind(kind: string): boolean {
  return kind === 'tool' || kind === 'workflow' || kind === 'thinking'
}

/**
 * Consecutive tool / workflow / thinking blocks share one collapsible strip so
 * interleaved thinking does not shatter the run into many visible cards.
 */
export function partitionLiveActivity(blocks: readonly ChatBlock[]): LiveActivitySegment[] {
  const out: LiveActivitySegment[] = []
  let i = 0
  while (i < blocks.length) {
    const b = blocks[i]
    if (!isActivityKind(b.kind)) {
      out.push({ type: 'chrome', block: b as LiveChromeBlock, index: i })
      i++
      continue
    }

    const items: LiveActivityItem[] = []
    const stepBlocks: ActivityStepBlock[] = []
    while (i < blocks.length && isActivityKind(blocks[i].kind)) {
      const cur = blocks[i]
      if (cur.kind === 'thinking') {
        items.push({ kind: 'thinking', block: cur, index: i })
      } else {
        const step = cur as ActivityStepBlock
        items.push({ kind: 'step', block: step, index: i })
        stepBlocks.push(step)
      }
      i++
    }

    if (stepBlocks.length === 0) {
      for (const it of items) {
        if (it.kind === 'thinking') {
          out.push({ type: 'thinking', block: it.block, index: it.index })
        }
      }
      continue
    }
    out.push({ type: 'steps', items, stepBlocks })
  }
  return out
}

/** History: fold thinking into the same strip as neighboring tools/workflows. */
export function partitionHistoryActivity(blocks: readonly HistoryBlock[]): HistoryActivitySegment[] {
  const out: HistoryActivitySegment[] = []
  let i = 0
  while (i < blocks.length) {
    if (!isActivityKind(blocks[i].kind)) {
      i++
      continue
    }
    const items: HistoryActivityItem[] = []
    const stepBlocks: ActivityStepBlock[] = []
    while (i < blocks.length && isActivityKind(blocks[i].kind)) {
      const cur = blocks[i]
      if (cur.kind === 'thinking') {
        items.push({ kind: 'thinking', block: cur })
      } else {
        const step = cur as ActivityStepBlock
        items.push({ kind: 'step', block: step })
        stepBlocks.push(step)
      }
      i++
    }
    if (stepBlocks.length === 0) {
      for (const it of items) {
        if (it.kind === 'thinking') out.push({ type: 'thinking', block: it.block })
      }
      continue
    }
    out.push({ type: 'steps', items, stepBlocks })
  }
  return out
}

/** Prefer waiting / running, else the latest step (for the collapsed ticker). */
export function focusActivityStepIndex(blocks: readonly ActivityStepBlock[]): number {
  if (blocks.length === 0) return -1
  const waiting = blocks.findIndex((b) => b.kind === 'tool' && b.status === 'waiting_human')
  if (waiting >= 0) return waiting
  const running = blocks.findIndex(
    (b) =>
      (b.kind === 'tool' && (b.status === 'running' || b.status === 'approved')) ||
      (b.kind === 'workflow' && b.steps.some((s) => s.status === 'running')),
  )
  if (running >= 0) return running
  return blocks.length - 1
}

export function activityStepsNeedAttention(blocks: readonly ActivityStepBlock[]): boolean {
  return blocks.some((b) => b.kind === 'tool' && b.status === 'waiting_human')
}

/** Compact strip whenever there are 2+ tool/workflow steps. */
export function shouldCollapseActivitySteps(stepBlocks: readonly ActivityStepBlock[]): boolean {
  return stepBlocks.length >= 2
}
