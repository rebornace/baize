import { describe, expect, it } from 'vitest'
import type { PendingApproval } from './api'
import { freshPendingApprovals, pendingDocumentTitle } from './pendingNotice'

const item = (id: string): PendingApproval => ({
  run_id: id,
  conversation_id: `c-${id}`,
  kind: 'approval',
})

describe('freshPendingApprovals', () => {
  it('returns only run ids that were not known', () => {
    const current = [item('r1'), item('r2'), item('r3')]
    const fresh = freshPendingApprovals(current, new Set(['r1', 'r3']))
    expect(fresh.map((x) => x.run_id)).toEqual(['r2'])
  })

  it('returns nothing when known is empty and the list is empty', () => {
    expect(freshPendingApprovals([], new Set())).toEqual([])
  })

  it('returns everything for an empty known set', () => {
    expect(freshPendingApprovals([item('a'), item('b')], new Set()).map((x) => x.run_id)).toEqual([
      'a',
      'b',
    ])
  })
})

describe('pendingDocumentTitle', () => {
  it('adds a count prefix', () => {
    expect(pendingDocumentTitle('Baize Chat', 2)).toBe('(2) Baize Chat')
  })

  it('replaces an existing prefix instead of stacking it', () => {
    expect(pendingDocumentTitle('(3) Baize Chat', 1)).toBe('(1) Baize Chat')
  })

  it('removes the prefix at zero', () => {
    expect(pendingDocumentTitle('(2) Baize Chat', 0)).toBe('Baize Chat')
  })
})
