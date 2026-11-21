import type { PendingApproval } from './api'

/** Items whose run id was not in the previously known set. */
export function freshPendingApprovals(
  current: PendingApproval[],
  known: ReadonlySet<string>,
): PendingApproval[] {
  return current.filter((item) => !known.has(item.run_id))
}

/** Browser title with a "(N) " prefix; an existing prefix is replaced. */
export function pendingDocumentTitle(base: string, count: number): string {
  const bare = base.replace(/^\(\d+\)\s*/, '')
  return count > 0 ? `(${count}) ${bare}` : bare
}
