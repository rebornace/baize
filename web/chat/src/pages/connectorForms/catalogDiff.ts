import type { CatalogDiff } from '../../api/connectors'
import { CONNECTORS } from '../../strings'

/** Human-readable catalog diff for save toasts; empty when no changes. */
export function catalogDiffToastLine(diff?: CatalogDiff | null): string {
  if (!diff) return ''
  const added = diff.added?.length ?? 0
  const removed = diff.removed?.length ?? 0
  const changed = diff.changed?.length ?? 0
  if (added === 0 && removed === 0 && changed === 0) return ''
  return CONNECTORS.catalogDiffLine(added, removed, changed)
}
