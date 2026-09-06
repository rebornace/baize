import type { RuntimeKnobs, RuntimeKnobsPatch } from '../api'

/** Editable form fields for engine knobs (all strings/booleans for inputs). */
export interface KnobsForm {
  max_messages: string
  max_steps: string
  tool_timeout_seconds: string
  compaction_enabled: boolean
  compact_threshold: string
  compact_reserve_tokens: string
  compact_keep_recent: string
  compact_summary_timeout_seconds: string
}

/** Field metadata for rendering + validation. */
export interface KnobFieldSpec {
  key: keyof Omit<KnobsForm, 'compaction_enabled'>
  label: string
  hint: string
  min: number
  max: number
  integer: boolean
}

export const KNOB_FIELDS: KnobFieldSpec[] = [
  { key: 'max_messages', label: '历史窗口消息数（max_messages）', hint: '1-500，默认 40', min: 1, max: 500, integer: true },
  { key: 'max_steps', label: '最大工具步数（max_steps）', hint: '1-100，默认 16', min: 1, max: 100, integer: true },
  { key: 'tool_timeout_seconds', label: '单次工具超时秒数（tool_timeout_seconds）', hint: '1-600，默认 60', min: 1, max: 600, integer: true },
  { key: 'compact_threshold', label: '压缩触发比例（compact_threshold）', hint: '0.1-0.95，默认 0.8', min: 0.1, max: 0.95, integer: false },
  { key: 'compact_reserve_tokens', label: '压缩预留 token（compact_reserve_tokens）', hint: '256-100000，默认 8000', min: 256, max: 100000, integer: true },
  { key: 'compact_keep_recent', label: '保留原文最近消息数（compact_keep_recent）', hint: '0-100，默认 8', min: 0, max: 100, integer: true },
  { key: 'compact_summary_timeout_seconds', label: '压缩摘要 LLM 超时秒数（compact_summary_timeout_seconds）', hint: '1-600，默认 60', min: 1, max: 600, integer: true },
]

export function knobsToForm(k: RuntimeKnobs): KnobsForm {
  return {
    max_messages: String(k.max_messages),
    max_steps: String(k.max_steps),
    tool_timeout_seconds: String(k.tool_timeout_seconds),
    compaction_enabled: k.compaction_enabled,
    compact_threshold: String(k.compact_threshold),
    compact_reserve_tokens: String(k.compact_reserve_tokens),
    compact_keep_recent: String(k.compact_keep_recent),
    compact_summary_timeout_seconds: String(k.compact_summary_timeout_seconds),
  }
}

/** Returns an error message for an invalid numeric field, or null if valid. */
export function validateKnobField(spec: KnobFieldSpec, raw: string): string | null {
  const v = Number(raw)
  if (raw.trim() === '' || Number.isNaN(v)) {
    return `${spec.label} 必须是数字`
  }
  if (spec.integer && !Number.isInteger(v)) {
    return `${spec.label} 必须是整数`
  }
  if (v < spec.min || v > spec.max) {
    return `${spec.label} 必须在 ${spec.min}-${spec.max} 之间`
  }
  return null
}

/**
 * Builds a partial knobs patch containing only the fields that differ from the
 * effective snapshot. Numbers are converted to the wire types.
 */
export function buildKnobsPatch(form: KnobsForm, effective: RuntimeKnobs): RuntimeKnobsPatch {
  const patch: RuntimeKnobsPatch = {}
  const int = (s: string): number | null => {
    const n = Number(s)
    return Number.isFinite(n) ? n : null
  }
  const maxMessages = int(form.max_messages)
  if (maxMessages !== null && maxMessages !== effective.max_messages) patch.max_messages = maxMessages
  const maxSteps = int(form.max_steps)
  if (maxSteps !== null && maxSteps !== effective.max_steps) patch.max_steps = maxSteps
  const toolTimeout = int(form.tool_timeout_seconds)
  if (toolTimeout !== null && toolTimeout !== effective.tool_timeout_seconds) {
    patch.tool_timeout_seconds = toolTimeout
  }
  if (form.compaction_enabled !== effective.compaction_enabled) {
    patch.compaction_enabled = form.compaction_enabled
  }
  const threshold = Number(form.compact_threshold)
  if (Number.isFinite(threshold) && threshold !== effective.compact_threshold) {
    patch.compact_threshold = threshold
  }
  const reserve = int(form.compact_reserve_tokens)
  if (reserve !== null && reserve !== effective.compact_reserve_tokens) {
    patch.compact_reserve_tokens = reserve
  }
  const keep = int(form.compact_keep_recent)
  if (keep !== null && keep !== effective.compact_keep_recent) {
    patch.compact_keep_recent = keep
  }
  const summaryTimeout = int(form.compact_summary_timeout_seconds)
  if (summaryTimeout !== null && summaryTimeout !== effective.compact_summary_timeout_seconds) {
    patch.compact_summary_timeout_seconds = summaryTimeout
  }
  return patch
}
