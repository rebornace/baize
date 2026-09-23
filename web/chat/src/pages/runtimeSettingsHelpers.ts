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
  // Decision layer (decide).
  decide_enabled: boolean
  decide_memory_enabled: boolean
  decide_profile_id: string
  decide_tool_routing_enabled: boolean
  decide_tool_threshold: string
  decide_tool_pre_topk: string
  decide_tool_choice_enabled: boolean
  decide_tool_prune_enabled: boolean
  decide_tool_prune_threshold: string
  decide_tool_prune_max_judged: string
  decide_route_enabled: boolean
  decide_route_min_runes: string
}

/** Boolean + free-text keys are not rendered as numeric field specs. */
type NonNumericKnobKeys =
  | 'compaction_enabled'
  | 'memory_enabled'
  | 'memory_auto_extract'
  | 'decide_enabled'
  | 'decide_memory_enabled'
  | 'decide_profile_id'
  | 'decide_tool_routing_enabled'
  | 'decide_tool_choice_enabled'
  | 'decide_tool_prune_enabled'
  | 'decide_route_enabled'

/** Field metadata for rendering + validation. */
export interface KnobFieldSpec {
  key: keyof Omit<KnobsForm, NonNumericKnobKeys>
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

/** Numeric knobs for tool-candidate routing (DP-2a). */
export function decideToolFields(): KnobFieldSpec[] {
  return [
    {
      key: 'decide_tool_threshold',
      label: RUNTIME.fieldDecideToolThreshold,
      hint: RUNTIME.hintDecideToolThreshold,
      min: 1,
      max: 500,
      integer: true,
    },
    {
      key: 'decide_tool_pre_topk',
      label: RUNTIME.fieldDecideToolPreTopK,
      hint: RUNTIME.hintDecideToolPreTopK,
      min: 1,
      max: 500,
      integer: true,
    },
  ]
}

/** Numeric knobs for bulky tool-result pruning (DP-3) and route fallback (DP-4). */
export function decideMiscFields(): KnobFieldSpec[] {
  return [
    {
      key: 'decide_tool_prune_threshold',
      label: RUNTIME.fieldDecidePruneThreshold,
      hint: RUNTIME.hintDecidePruneThreshold,
      min: 1,
      max: 100000,
      integer: true,
    },
    {
      key: 'decide_tool_prune_max_judged',
      label: RUNTIME.fieldDecidePruneMaxJudged,
      hint: RUNTIME.hintDecidePruneMaxJudged,
      min: 1,
      max: 100,
      integer: true,
    },
    {
      key: 'decide_route_min_runes',
      label: RUNTIME.fieldDecideRouteMinRunes,
      hint: RUNTIME.hintDecideRouteMinRunes,
      min: 1,
      max: 100000,
      integer: true,
    },
  ]
}

export function allKnobFieldSpecs(): KnobFieldSpec[] {
  return [
    ...mainKnobFields(),
    ...compactAdvFields(),
    ...decideToolFields(),
    ...decideMiscFields(),
  ]
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
    decide_enabled: k.decide_enabled,
    decide_memory_enabled: k.decide_memory_enabled,
    decide_profile_id: k.decide_profile_id,
    decide_tool_routing_enabled: k.decide_tool_routing_enabled,
    decide_tool_threshold: String(k.decide_tool_threshold),
    decide_tool_pre_topk: String(k.decide_tool_pre_topk),
    decide_tool_choice_enabled: k.decide_tool_choice_enabled,
    decide_tool_prune_enabled: k.decide_tool_prune_enabled,
    decide_tool_prune_threshold: String(k.decide_tool_prune_threshold),
    decide_tool_prune_max_judged: String(k.decide_tool_prune_max_judged),
    decide_route_enabled: k.decide_route_enabled,
    decide_route_min_runes: String(k.decide_route_min_runes),
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
  // Decision layer.
  if (form.decide_enabled !== effective.decide_enabled) {
    patch.decide_enabled = form.decide_enabled
  }
  if (form.decide_memory_enabled !== effective.decide_memory_enabled) {
    patch.decide_memory_enabled = form.decide_memory_enabled
  }
  if (form.decide_profile_id !== effective.decide_profile_id) {
    patch.decide_profile_id = form.decide_profile_id
  }
  if (form.decide_tool_routing_enabled !== effective.decide_tool_routing_enabled) {
    patch.decide_tool_routing_enabled = form.decide_tool_routing_enabled
  }
  if (form.decide_tool_choice_enabled !== effective.decide_tool_choice_enabled) {
    patch.decide_tool_choice_enabled = form.decide_tool_choice_enabled
  }
  const toolThreshold = int(form.decide_tool_threshold)
  if (toolThreshold !== null && toolThreshold !== effective.decide_tool_threshold) {
    patch.decide_tool_threshold = toolThreshold
  }
  const toolPreTopK = int(form.decide_tool_pre_topk)
  if (toolPreTopK !== null && toolPreTopK !== effective.decide_tool_pre_topk) {
    patch.decide_tool_pre_topk = toolPreTopK
  }
  if (form.decide_tool_prune_enabled !== effective.decide_tool_prune_enabled) {
    patch.decide_tool_prune_enabled = form.decide_tool_prune_enabled
  }
  const pruneThreshold = int(form.decide_tool_prune_threshold)
  if (pruneThreshold !== null && pruneThreshold !== effective.decide_tool_prune_threshold) {
    patch.decide_tool_prune_threshold = pruneThreshold
  }
  const pruneMaxJudged = int(form.decide_tool_prune_max_judged)
  if (pruneMaxJudged !== null && pruneMaxJudged !== effective.decide_tool_prune_max_judged) {
    patch.decide_tool_prune_max_judged = pruneMaxJudged
  }
  if (form.decide_route_enabled !== effective.decide_route_enabled) {
    patch.decide_route_enabled = form.decide_route_enabled
  }
  const routeMinRunes = int(form.decide_route_min_runes)
  if (routeMinRunes !== null && routeMinRunes !== effective.decide_route_min_runes) {
    patch.decide_route_min_runes = routeMinRunes
  }
  return patch
}
