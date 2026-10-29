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
  context_projection_enabled: boolean
  context_offload_enabled: boolean
  active_goal_enabled: boolean
  active_goal_max_depth: number
  active_goal_max_text_runes: number
  memory_enabled: boolean
  memory_auto_extract: boolean
  // Decision layer (decide).
  decide_enabled: boolean
  decide_memory_enabled: boolean
  decide_profile_id: string
  decide_systemone_base_url: string
  decide_systemone_api_key_set: boolean
  decide_systemone_model: string
  decide_tool_routing_enabled: boolean
  decide_tool_threshold: number
  decide_tool_pre_topk: number
  decide_tool_choice_enabled: boolean
  decide_tool_prune_enabled: boolean
  decide_tool_prune_threshold: number
  decide_tool_prune_max_judged: number
  decide_route_enabled: boolean
  decide_route_min_runes: number
  scoped_subrun_enabled: boolean
}

/** Per-field flags: true when the value is overridden from the YAML baseline. */
export type RuntimeKnobsOverrides = Record<keyof RuntimeKnobs, boolean>

export interface RuntimeKnobsView {
  effective: RuntimeKnobs
  overridden: RuntimeKnobsOverrides
  public_base_url: string
  public_base_url_overridden: boolean
  /** Effective log / OTel (YAML baseline + KV overrides; hot-reloadable). */
  log_format?: string
  otel_otlp_endpoint?: string
  otel_service_name?: string
  otel_enabled?: boolean
  log_format_overridden?: boolean
  otel_otlp_endpoint_overridden?: boolean
  otel_service_name_overridden?: boolean
  config_restart_hint?: boolean
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
  context_projection_enabled: boolean
  context_offload_enabled: boolean
  active_goal_enabled: boolean
  active_goal_max_depth: number
  active_goal_max_text_runes: number
  memory_enabled: boolean
  memory_auto_extract: boolean
  decide_enabled: boolean
  decide_memory_enabled: boolean
  decide_profile_id: string
  decide_systemone_base_url: string
  decide_systemone_api_key: string
  decide_systemone_model: string
  decide_tool_routing_enabled: boolean
  decide_tool_threshold: number
  decide_tool_pre_topk: number
  decide_tool_choice_enabled: boolean
  decide_tool_prune_enabled: boolean
  decide_tool_prune_threshold: number
  decide_tool_prune_max_judged: number
  decide_route_enabled: boolean
  decide_route_min_runes: number
  scoped_subrun_enabled: boolean
  public_base_url: string
  log_format: string
  otel_otlp_endpoint: string
  otel_service_name: string
  observability_reset: boolean
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
