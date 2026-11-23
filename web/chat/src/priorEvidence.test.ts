import { describe, expect, it } from 'vitest'
import type { ChatBlock } from './foldEvents'
import { collectPriorEvidence, formatEvidencePreview } from './priorEvidence'

function tool(
  name: string,
  opts: { result?: unknown; isError?: boolean; status?: 'succeeded' | 'waiting_human' } = {},
): ChatBlock {
  return {
    kind: 'tool',
    name,
    status: opts.status ?? 'succeeded',
    runId: 'r1',
    result: opts.result,
    isError: opts.isError,
  }
}

describe('formatEvidencePreview', () => {
  it('stringifies and truncates long JSON', () => {
    const long = { a: 'x'.repeat(200) }
    const preview = formatEvidencePreview(long)
    expect(preview.length).toBeLessThanOrEqual(160)
    expect(preview.endsWith('…')).toBe(true)
  })
})

describe('collectPriorEvidence', () => {
  it('returns prior tool results before the waiting block', () => {
    const blocks: ChatBlock[] = [
      tool('list_tickets', { result: { tickets: [] } }),
      tool('get_ticket', { result: { id: 'demo-1' } }),
      tool('update_ticket_status', { status: 'waiting_human' }),
    ]
    const items = collectPriorEvidence(blocks, 2)
    expect(items.map((i) => i.name)).toEqual(['list_tickets', 'get_ticket'])
    expect(items[1].preview).toContain('demo-1')
  })

  it('skips tools without results', () => {
    const blocks: ChatBlock[] = [
      tool('list_tickets'),
      tool('create_ticket', { status: 'waiting_human' }),
    ]
    expect(collectPriorEvidence(blocks, 1)).toEqual([])
  })
})
