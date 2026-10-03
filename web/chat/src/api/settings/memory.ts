import { authInit, parseJSON } from '../http'

export interface MemoryEntry {
  id: string
  owner_id: string
  key?: string
  text: string
  source: string
  created_at: string
  updated_at: string
}

export async function listMemory(opts?: {
  limit?: number
  offset?: number
}): Promise<MemoryEntry[]> {
  const qs = new URLSearchParams()
  if (opts?.limit != null) qs.set('limit', String(opts.limit))
  if (opts?.offset != null) qs.set('offset', String(opts.offset))
  const suffix = qs.toString() ? `?${qs}` : ''
  const res = await fetch(`/v0/settings/memory${suffix}`, { headers: authInit() })
  const body = await parseJSON<{ items: MemoryEntry[] }>(res)
  return body.items ?? []
}

export async function createMemory(body: {
  text: string
  key?: string
}): Promise<MemoryEntry> {
  const res = await fetch('/v0/settings/memory', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<MemoryEntry>(res)
}

export async function patchMemory(
  id: string,
  body: { text?: string; key?: string },
): Promise<MemoryEntry> {
  const res = await fetch(`/v0/settings/memory/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<MemoryEntry>(res)
}

export async function deleteMemory(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/memory/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}
