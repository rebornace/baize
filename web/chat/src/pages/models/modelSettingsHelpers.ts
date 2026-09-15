import type {
  ModelProfile,
  ModelTier,
  ThinkingDialect,
  ThinkingLevel,
} from '../../api'
import { MODELS } from '../../strings'

export const CREATE_ID = '__new__'

export function tierOptions(): { value: ModelTier | 'auto'; label: string }[] {
  return [
    { value: 'auto', label: MODELS.tierOptionAuto },
    { value: 'light', label: MODELS.tierOptionLight },
    { value: 'standard', label: MODELS.tierOptionStandard },
    { value: 'power', label: MODELS.tierOptionPower },
  ]
}

export function thinkingLevelOptions(): { value: ThinkingLevel; label: string }[] {
  return [
    { value: 'off', label: MODELS.thinkingLevelOff },
    { value: 'low', label: MODELS.thinkingLevelLow },
    { value: 'medium', label: MODELS.thinkingLevelMedium },
    { value: 'high', label: MODELS.thinkingLevelHigh },
  ]
}

export function thinkingDialectOptions(): { value: ThinkingDialect; label: string }[] {
  return [
    { value: 'auto', label: MODELS.thinkingDialectAuto },
    { value: 'openai', label: MODELS.thinkingDialectOpenai },
    { value: 'deepseek', label: MODELS.thinkingDialectDeepseek },
    { value: 'qwen', label: MODELS.thinkingDialectQwen },
    { value: 'omit', label: MODELS.thinkingDialectOmit },
  ]
}

export interface ProfileFormState {
  name: string
  baseUrl: string
  model: string
  apiKey: string
  apiKeyEnv: string
  supportsVision: boolean
  thinkingLevel: ThinkingLevel
  thinkingDialect: ThinkingDialect
  contextTokens: number
  tier: ModelTier | 'auto'
}

export const EMPTY_PROFILE_FORM: ProfileFormState = {
  name: '',
  baseUrl: '',
  model: '',
  apiKey: '',
  apiKeyEnv: '',
  supportsVision: false,
  thinkingLevel: 'medium',
  thinkingDialect: 'auto',
  contextTokens: 128000,
  tier: 'auto',
}

export interface ModelProfilePayload {
  name?: string
  base_url?: string
  model?: string
  api_key?: string
  api_key_env?: string
  supports_vision?: boolean
  disable_thinking?: boolean
  thinking_level?: ThinkingLevel
  thinking_dialect?: ThinkingDialect
  context_tokens?: number
  auto_tier?: ModelTier | 'auto'
}

export function resolveThinkingLevel(p: ModelProfile): ThinkingLevel {
  if (p.thinking_level) return p.thinking_level
  return p.disable_thinking ? 'off' : 'medium'
}

export function profileToForm(p: ModelProfile): ProfileFormState {
  return {
    name: p.name,
    baseUrl: p.base_url,
    model: p.model,
    // Never prefill: the list value is a redacted mask and must not be
    // echoed back; an empty field means "keep the stored key".
    apiKey: '',
    apiKeyEnv: p.api_key_env ?? '',
    supportsVision: p.supports_vision,
    thinkingLevel: resolveThinkingLevel(p),
    thinkingDialect: p.thinking_dialect ?? 'auto',
    contextTokens: p.context_tokens > 0 ? p.context_tokens : 128000,
    tier: p.auto_tier ?? 'standard',
  }
}

export function buildCreatePayload(form: ProfileFormState):
  | { ok: true; payload: ModelProfilePayload }
  | { ok: false; message: string } {
  const name = form.name.trim()
  if (!name) return { ok: false, message: MODELS.errNameRequired }
  const baseUrl = form.baseUrl.trim()
  if (!baseUrl) return { ok: false, message: MODELS.errBaseUrlRequired }
  const model = form.model.trim()
  if (!model) return { ok: false, message: MODELS.errModelRequired }
  const apiKey = form.apiKey.trim()
  const apiKeyEnv = form.apiKeyEnv.trim()
  if (!apiKey && !apiKeyEnv) {
    return { ok: false, message: MODELS.errApiKeyRequired }
  }
  return {
    ok: true,
    payload: {
      name,
      base_url: baseUrl,
      model,
      ...(apiKey ? { api_key: apiKey } : {}),
      ...(apiKeyEnv ? { api_key_env: apiKeyEnv } : {}),
      supports_vision: form.supportsVision,
      thinking_level: form.thinkingLevel,
      thinking_dialect: form.thinkingDialect,
      context_tokens: form.contextTokens > 0 ? form.contextTokens : 128000,
      auto_tier: form.tier,
    },
  }
}

// buildPatchPayload returns only the fields that differ from the stored
// profile. The backend merges field-level (pointer fields), so omitted
// fields — including an empty api_key — keep their stored value.
export function buildPatchPayload(
  form: ProfileFormState,
  original: ModelProfile,
): ModelProfilePayload {
  const payload: ModelProfilePayload = {}
  const name = form.name.trim()
  if (name !== original.name) payload.name = name
  const baseUrl = form.baseUrl.trim()
  if (baseUrl !== original.base_url) payload.base_url = baseUrl
  const model = form.model.trim()
  if (model !== original.model) payload.model = model
  const apiKey = form.apiKey.trim()
  if (apiKey) payload.api_key = apiKey
  const apiKeyEnv = form.apiKeyEnv.trim()
  if (apiKeyEnv !== (original.api_key_env ?? '')) payload.api_key_env = apiKeyEnv
  if (form.supportsVision !== original.supports_vision) {
    payload.supports_vision = form.supportsVision
  }
  if (form.thinkingLevel !== resolveThinkingLevel(original)) {
    payload.thinking_level = form.thinkingLevel
  }
  if (form.thinkingDialect !== (original.thinking_dialect ?? 'auto')) {
    payload.thinking_dialect = form.thinkingDialect
  }
  const contextTokens = Math.floor(Number(form.contextTokens))
  if (contextTokens > 0 && contextTokens !== original.context_tokens) {
    payload.context_tokens = contextTokens
  }
  if (form.tier !== (original.auto_tier ?? 'standard')) {
    payload.auto_tier = form.tier
  }
  return payload
}

export function credentialHint(p: ModelProfile): string {
  if (p.api_key_env) return `env:${p.api_key_env}`
  if (p.api_key) return `key:${p.api_key}`
  return '无凭据'
}
