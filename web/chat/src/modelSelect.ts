import type { CreateRunOptions, ModelProfile } from './api'

export interface ModelOption {
  /** "" means "use the server-side default model". */
  value: string
  label: string
}

/**
 * Build the dropdown options for the chat composer model picker. The first
 * option always represents the server default (value "") so an explicit
 * model_profile_id is only sent when the user picks a named profile.
 */
export function modelOptions(profiles: ModelProfile[]): ModelOption[] {
  const def = profiles.find((p) => p.is_default)
  const first: ModelOption = {
    value: '',
    label: def ? `默认：${def.name}` : '默认模型',
  }
  const rest = profiles
    .filter((p) => !p.is_default)
    .map((p) => ({ value: p.id, label: `${p.name}（${p.model}）` }))
  return [first, ...rest]
}

/**
 * Merge the per-message model choice into createRun options. An empty
 * selection omits modelProfileId entirely so the backend falls back to the
 * default profile; the choice is never persisted across messages.
 */
export function buildRunOptions(
  selectedId: string,
  base: CreateRunOptions = {},
): CreateRunOptions {
  const id = selectedId.trim()
  return {
    ...base,
    ...(id ? { modelProfileId: id } : {}),
  }
}
