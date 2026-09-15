import type { RuntimeKnobs, RuntimeKnobsPatch } from '../api'
import { RUNTIME } from '../strings'

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
  memory_enabled: boolean
  memory_auto_extract: boolean
}

/** Field metadata for rendering + validation. */
export interface KnobFieldSpec {
  key: keyof Omit<KnobsForm, 'compaction_enabled' | 'memory_enabled' | 'memory_auto_extract'>
  label: string
  hint: string
  min: number
  max: number
  integer: boolean
}

/** Rebuild on each call so labels/hints follow the active locale pack. */
export function mainKnobFields(): KnobFieldSpec[] {
  return [
    {
      key: 'max_messages',
      label: RUNTIME.fieldMaxMessages,
      hint: RUNTIME.hintMaxMessages,
      min: 1,
      max: 500,
      integer: true,
    },
    {
      key: 'max_steps',
      label: RUNTIME.fieldMaxSteps,
      hint: RUNTIME.hintMaxSteps,
      min: 1,
      max: 100,
      integer: true,
    },
    {
      key: 'tool_timeout_seconds',
      label: RUNTIME.fieldToolTimeout,
      hint: RUNTIME.hintToolTimeout,
      min: 1,
      max: 600,
      integer: true,
    },
  ]
}

export function compactAdvFields(): KnobFieldSpec[] {
  return [
    {
      key: 'compact_threshold',
      label: RUNTIME.fieldCompactThreshold,
      hint: RUNTIME.hintCompactThreshold,
      min: 0.1,
      max: 0.95,
      integer: false,
    },
    {
      key: 'compact_reserve_tokens',
      label: RUNTIME.fieldCompactReserve,
      hint: RUNTIME.hintCompactReserve,
      min: 256,
      max: 100000,
      integer: true,
    },
    {
      key: 'compact_keep_recent',
      label: RUNTIME.fieldCompactKeep,
      hint: RUNTIME.hintCompactKeep,
      min: 0,
      max: 100,
      integer: true,
    },
    {
      key: 'compact_summary_timeout_seconds',
      label: RUNTIME.fieldCompactSummaryTimeout,
      hint: RUNTIME.hintCompactSummaryTimeout,
      min: 1,
      max: 600,
      integer: true,
    },
  ]
}

/** @deprecated Prefer mainKnobFields() — live labels. */
export const MAIN_KNOB_FIELDS = mainKnobFields()

/** @deprecated Prefer compactAdvFields() — live labels. */
export const COMPACT_ADV_FIELDS = compactAdvFields()

export function allKnobFieldSpecs(): KnobFieldSpec[] {
  return [...mainKnobFields(), ...compactAdvFields()]
}

/** @deprecated 勿用；保留一版别名以免遗漏引用时可 grep */
export const KNOB_FIELDS = allKnobFieldSpecs()

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
    memory_enabled: k.memory_enabled,
    memory_auto_extract: k.memory_auto_extract,
  }
}

/** Returns an error message for an invalid numeric field, or null if valid. */
export function validateKnobField(spec: KnobFieldSpec, raw: string): string | null {
  const v = Number(raw)
  if (raw.trim() === '' || Number.isNaN(v)) {
    return `${spec.label} ${RUNTIME.errMustNumber}`
  }
  if (spec.integer && !Number.isInteger(v)) {
    return `${spec.label} ${RUNTIME.errMustInt}`
  }
  if (v < spec.min || v > spec.max) {
    return `${spec.label} ${RUNTIME.errOutOfRange} (${spec.min}–${spec.max})`
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
  if (form.memory_enabled !== effective.memory_enabled) {
    patch.memory_enabled = form.memory_enabled
  }
  if (form.memory_auto_extract !== effective.memory_auto_extract) {
    patch.memory_auto_extract = form.memory_auto_extract
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
