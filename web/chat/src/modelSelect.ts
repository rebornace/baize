import type { CreateRunOptions, ModelProfile } from './api'

/**
 * AUTO_MODEL_ID is the sentinel value for the "smart routing (Auto)" choice.
 * It is not a real model profile: the server picks a concrete model based on
 * the turn's content (e.g. routing image turns to a vision-capable profile).
 * It is the default selection and the only mode for channel (WeChat) inbound.
 */
export const AUTO_MODEL_ID = 'auto'

export interface ModelOption {
  /** AUTO_MODEL_ID means smart routing; otherwise a concrete profile id. */
  value: string
  label: string
}

/**
 * Build the dropdown options for the chat composer model picker. The first
 * option is always "智能路由 (Auto)". Every configured profile follows as a
 * manual choice (including the default profile, which is tagged "默认"), so a
 * deliberate pick always pins that exact model with no auto rerouting.
 */
export function modelOptions(profiles: ModelProfile[]): ModelOption[] {
  const auto: ModelOption = { value: AUTO_MODEL_ID, label: '智能路由（Auto）' }
  const rest = profiles.map((p) => {
    const tags = [
      p.is_default ? '默认' : '',
      p.supports_vision ? '视觉' : '',
    ].filter(Boolean)
    const suffix = tags.length ? ` · ${tags.join('·')}` : ''
    return { value: p.id, label: `${p.name}（${p.model}）${suffix}` }
  })
  return [auto, ...rest]
}

/** A blank or "auto" selection means smart routing. */
export function isAutoChoice(id: string): boolean {
  const t = id.trim().toLowerCase()
  return t === '' || t === AUTO_MODEL_ID
}

export interface VisionGateResult {
  allowed: boolean
  /** User-facing reason when allowed is false. */
  message?: string
}

/**
 * Decide whether an image attachment may be sent under the current model
 * choice.
 *
 * - Auto mode: allowed when the default model is vision-capable OR any
 *   vision-capable profile exists (the router picks it). Otherwise the user
 *   must add a vision model in Settings.
 * - Manual mode: allowed only when the specifically chosen profile is
 *   vision-capable. A manual pick is NEVER silently rerouted, so a text-only
 *   choice on an image turn is rejected with guidance to switch to Auto or a
 *   vision model.
 *
 * Non-image turns are always allowed.
 */
export function visionGate(
  profiles: ModelProfile[],
  selectedId: string,
  hasImages: boolean,
  defaultSupportsVision: boolean,
): VisionGateResult {
  if (!hasImages) return { allowed: true }
  if (isAutoChoice(selectedId)) {
    const canVision = defaultSupportsVision || profiles.some((p) => p.supports_vision)
    if (canVision) return { allowed: true }
    return {
      allowed: false,
      message: '当前没有可用的视觉模型，请在模型设置中添加支持视觉的模型，或移除图片后再发送。',
    }
  }
  const chosen = profiles.find((p) => p.id === selectedId.trim())
  if (chosen?.supports_vision) return { allowed: true }
  return {
    allowed: false,
    message: '所选模型不支持图片附件。请改用「智能路由（Auto）」或选择带「视觉」标记的模型，或移除图片。',
  }
}

/**
 * Merge the per-message model choice into createRun options. Auto is sent
 * explicitly as model_profile_id="auto"; a manual pick sends its concrete id.
 * The choice is never persisted across messages.
 */
export function buildRunOptions(
  selectedId: string,
  base: CreateRunOptions = {},
): CreateRunOptions {
  const id = selectedId.trim()
  const modelProfileId = isAutoChoice(id) ? AUTO_MODEL_ID : id
  return {
    ...base,
    modelProfileId,
  }
}
