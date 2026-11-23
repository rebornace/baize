import type { MemoryEntry } from './api'

export type MemoryGroup = {
  key: string
  entries: MemoryEntry[]
  /** Same key has multiple rows or at least one row is a near-duplicate. */
  conflict: boolean
  /** Entry ids whose text is near-duplicate of another entry. */
  nearDupIds: Set<string>
}

/** Lowercase, remove whitespace and punctuation so "喜欢 绿茶。" == "喜欢绿茶". */
function normalizeText(text: string): string {
  return text.toLowerCase().replace(/[\s\p{P}\p{S}]+/gu, '')
}

function bigrams(text: string): Set<string> {
  const out = new Set<string>()
  for (let i = 0; i < text.length - 1; i++) {
    out.add(text.slice(i, i + 2))
  }
  return out
}

/** Character-bigram Jaccard; same threshold as backend MEM-NEARDUP. */
export function bigramSimilarity(a: string, b: string): number {
  const x = normalizeText(a)
  const y = normalizeText(b)
  if (!x || !y) return 0
  if (x === y) return 1
  if (x.length < 2 || y.length < 2) {
    return x.includes(y) || y.includes(x) ? 1 : 0
  }
  const gx = bigrams(x)
  const gy = bigrams(y)
  let inter = 0
  for (const g of gx) {
    if (gy.has(g)) inter++
  }
  const union = gx.size + gy.size - inter
  return union === 0 ? 0 : inter / union
}

function updatedMs(entry: MemoryEntry): number {
  const t = Date.parse(entry.updated_at)
  return Number.isFinite(t) ? t : 0
}

/**
 * Groups memories by exact key (empty key = ungrouped), marks same-key
 * conflicts and near-duplicate texts, and sorts risky groups first.
 */
export function groupMemory(entries: MemoryEntry[]): MemoryGroup[] {
  const near = new Set<string>()
  for (let i = 0; i < entries.length; i++) {
    for (let j = i + 1; j < entries.length; j++) {
      if (bigramSimilarity(entries[i].text, entries[j].text) >= 0.72) {
        near.add(entries[i].id)
        near.add(entries[j].id)
      }
    }
  }

  const byKey = new Map<string, MemoryEntry[]>()
  for (const entry of entries) {
    const key = (entry.key ?? '').trim()
    const list = byKey.get(key) ?? []
    list.push(entry)
    byKey.set(key, list)
  }

  const groups: MemoryGroup[] = []
  for (const [key, list] of byKey) {
    list.sort((a, b) => updatedMs(b) - updatedMs(a))
    const nearDupIds = new Set(list.filter((e) => near.has(e.id)).map((e) => e.id))
    groups.push({
      key,
      entries: list,
      conflict: list.length > 1 || nearDupIds.size > 0,
      nearDupIds,
    })
  }

  groups.sort((a, b) => {
    if (a.conflict !== b.conflict) return a.conflict ? -1 : 1
    if (a.key === b.key) return 0
    if (!a.key) return 1
    if (!b.key) return -1
    return a.key.localeCompare(b.key)
  })
  return groups
}
