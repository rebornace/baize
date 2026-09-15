import { authInit, parseJSON } from '../http'
import type { ThinkingLevel } from '../types'

/** Wire dialect for thinking fields; `auto` lets the server infer. */
export type ThinkingDialect = 'auto' | 'openai' | 'deepseek' | 'qwen' | 'omit'

export interface ModelProfile {
  id: string
  name: string
  provider: string
  base_url: string
  model: string
  /** Redacted mask in list/detail responses; never sent verbatim by the server. */
  api_key?: string
  api_key_env?: string
  /** Derived: thinking_level === 'off'. Kept for older clients. */
  disable_thinking: boolean
  thinking_level: ThinkingLevel
  thinking_dialect: ThinkingDialect
  supports_vision: boolean
  context_tokens: number
  /** Auto-routing capability tier: "light" | "standard" | "power". */
  auto_tier: ModelTier
  created_at?: string
  updated_at?: string
}

/** Capability tiers understood by the task-aware Auto router. */
export type ModelTier = 'light' | 'standard' | 'power'

/** Editable fields of a model profile. Booleans omitted on PATCH are kept.
 * `auto_tier` may also be "auto" on write to request server-side inference. */
export type ModelProfileInput = Partial<{
  name: string
  provider: string
  base_url: string
  model: string
  api_key: string
  api_key_env: string
  disable_thinking: boolean
  thinking_level: ThinkingLevel
  thinking_dialect: ThinkingDialect
  supports_vision: boolean
  context_tokens: number
  auto_tier: ModelTier | 'auto'
}>

export async function listModelProfiles(): Promise<ModelProfile[]> {
  const res = await fetch('/v0/settings/models', { headers: authInit() })
  const body = await parseJSON<{ profiles: ModelProfile[] }>(res)
  return body.profiles ?? []
}

export async function createModelProfile(p: ModelProfileInput): Promise<ModelProfile> {
  const res = await fetch('/v0/settings/models', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(p),
  })
  const body = await parseJSON<{ profile: ModelProfile }>(res)
  return body.profile
}

export async function updateModelProfile(
  id: string,
  p: ModelProfileInput,
): Promise<ModelProfile> {
  const res = await fetch(`/v0/settings/models/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(p),
  })
  const body = await parseJSON<{ profile: ModelProfile }>(res)
  return body.profile
}

export async function deleteModelProfile(id: string): Promise<void> {
  const res = await fetch(`/v0/settings/models/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: authInit(),
  })
  await parseJSON<{ status: string }>(res)
}
