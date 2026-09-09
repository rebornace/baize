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
 * manual choice. Profiles are tagged with their task-aware Auto tier
 * (轻量 / 标准 / 强力) and 视觉 when the model accepts images. A deliberate
 * manual pick always pins that exact model with no auto rerouting.
 */
export function modelOptions(profiles: ModelProfile[]): ModelOption[] {
  const auto: ModelOption = { value: AUTO_MODEL_ID, label: '智能路由（Auto）' }
  const rest = profiles.map((p) => {
    const tags = [
      tierLabel(p.auto_tier),
      p.supports_vision ? '视觉' : '',
    ].filter(Boolean)
    const suffix = tags.length ? ` · ${tags.join('·')}` : ''
    return { value: p.id, label: `${p.name}（${p.model}）${suffix}` }
  })
  return [auto, ...rest]
}

/** Chinese label for a model capability tier; unknown values read as 标准. */
export function tierLabel(tier?: string): string {
  switch (tier) {
    case 'light':
      return '轻量'
    case 'power':
      return '强力'
    case 'standard':
    default:
      return '标准'
  }
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
 * - Auto mode: allowed when ANY configured profile is vision-capable (the
 *   router picks it). Otherwise the user must add a vision model in Settings.
 * - Manual mode: allowed only when the specifically chosen profile is
 *   vision-capable. A manual pick is NEVER silently rerouted, so a text-only
 *   choice on an image turn is rejected with guidance to switch to Auto or a
 *   vision model.
 *
 * Non-image turns are always allowed. `anyVisionModel` is a fast signal from
 * /v0/ui_config used before the profile list has loaded; when profiles are
 * present their own flags are authoritative.
 */
export function visionGate(
  profiles: ModelProfile[],
  selectedId: string,
  hasImages: boolean,
  anyVisionModel: boolean,
): VisionGateResult {
  if (!hasImages) return { allowed: true }
  if (isAutoChoice(selectedId)) {
    const canVision = anyVisionModel || profiles.some((p) => p.supports_vision)
    if (canVision) return { allowed: true }
    return {
      allowed: false,
      message: '当前没有可用的视觉模型，请在「设置 → 模型」中添加支持视觉的模型，或移除图片后再发送。',
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
