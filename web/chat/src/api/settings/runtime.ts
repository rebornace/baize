import { authInit, parseJSON } from '../http'

/** Effective engine knobs (duration expressed in seconds on the wire). */
export interface RuntimeKnobs {
  max_messages: number
  max_steps: number
  tool_timeout_seconds: number
  compaction_enabled: boolean
  compact_threshold: number
  compact_reserve_tokens: number
  compact_keep_recent: number
  compact_summary_timeout_seconds: number
  memory_enabled: boolean
  memory_auto_extract: boolean
  // Decision layer (decide).
  decide_enabled: boolean
  decide_memory_enabled: boolean
  decide_profile_id: string
  decide_tool_routing_enabled: boolean
  decide_tool_threshold: number
  decide_tool_pre_topk: number
  decide_tool_choice_enabled: boolean
  decide_tool_prune_enabled: boolean
  decide_tool_prune_threshold: number
  decide_tool_prune_max_judged: number
  decide_route_enabled: boolean
  decide_route_min_runes: number
}

/** Per-field flags: true when the value is overridden from the YAML baseline. */
export type RuntimeKnobsOverrides = Record<keyof RuntimeKnobs, boolean>

export interface RuntimeKnobsView {
  effective: RuntimeKnobs
  overridden: RuntimeKnobsOverrides
  public_base_url: string
  public_base_url_overridden: boolean
}

/** Partial engine-knob update; omitted fields are left unchanged.
 * public_base_url: omit = leave; "" = clear override to YAML; non-empty = set. */
export type RuntimeKnobsPatch = Partial<{
  max_messages: number
  max_steps: number
  tool_timeout_seconds: number
  compaction_enabled: boolean
  compact_threshold: number
  compact_reserve_tokens: number
  compact_keep_recent: number
  compact_summary_timeout_seconds: number
  memory_enabled: boolean
  memory_auto_extract: boolean
  decide_enabled: boolean
  decide_memory_enabled: boolean
  decide_profile_id: string
  decide_tool_routing_enabled: boolean
  decide_tool_threshold: number
  decide_tool_pre_topk: number
  decide_tool_choice_enabled: boolean
  decide_tool_prune_enabled: boolean
  decide_tool_prune_threshold: number
  decide_tool_prune_max_judged: number
  decide_route_enabled: boolean
  decide_route_min_runes: number
  public_base_url: string
}>

export async function getRuntimeSettings(): Promise<RuntimeKnobsView> {
  const res = await fetch('/v0/settings/runtime', { headers: authInit() })
  return parseJSON<RuntimeKnobsView>(res)
}

export async function patchRuntimeSettings(
  body: RuntimeKnobsPatch,
): Promise<RuntimeKnobsView> {
  const res = await fetch('/v0/settings/runtime', {
    method: 'PATCH',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON<RuntimeKnobsView>(res)
}
