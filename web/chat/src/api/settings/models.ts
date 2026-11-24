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

/** One model entry advertised by an OpenAI-compatible provider. */
export interface UpstreamModel {
  id: string
  object?: string
  created?: number
  owned_by?: string
}

export interface DiscoverModelsInput {
  base_url: string
  api_key?: string
  api_key_env?: string
  profile_id?: string
}

/** Fetches the model list offered at the given endpoint (no persistence). */
export async function discoverModels(input: DiscoverModelsInput): Promise<UpstreamModel[]> {
  const res = await fetch('/v0/settings/models/discover', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(input),
  })
  const body = await parseJSON<{ models: UpstreamModel[] }>(res)
  return body.models ?? []
}

export interface BatchModelChoice {
  id: string
  name?: string
}

export interface BatchImportModelsInput {
  base_url: string
  api_key?: string
  api_key_env?: string
  profile_id?: string
  models: BatchModelChoice[]
  thinking_level?: ThinkingLevel
  thinking_dialect?: ThinkingDialect
  supports_vision?: boolean
  context_tokens?: number
}

export interface BatchSkipped {
  id: string
  name: string
  reason: string
}

export interface BatchImportResult {
  created: ModelProfile[]
  skipped: BatchSkipped[]
}

/** Creates one profile per selected model, sharing one endpoint/credential. */
export async function batchImportModels(
  input: BatchImportModelsInput,
): Promise<BatchImportResult> {
  const res = await fetch('/v0/settings/models/batch', {
    method: 'POST',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(input),
  })
  const body = await parseJSON<{ result: BatchImportResult }>(res)
  return {
    created: body.result.created ?? [],
    skipped: body.result.skipped ?? [],
  }
}

export interface ModelPriceEntry {
  model: string
  input_per_mtok: number
  output_per_mtok: number
  cached_input_per_mtok: number
}

export interface ModelPriceTable {
  currency: string
  source?: string
  entries: ModelPriceEntry[]
}

export async function getModelPrices(): Promise<ModelPriceTable & { defaults: ModelPriceTable }> {
  const res = await fetch('/v0/settings/model-prices', { headers: authInit() })
  return parseJSON(res)
}

export async function putModelPrices(table: ModelPriceTable): Promise<ModelPriceTable> {
  const res = await fetch('/v0/settings/model-prices', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(table),
  })
  return parseJSON(res)
}

export interface CostDay {
  date: string
  total_usd: number
  runs: number
}

export interface CostSummary {
  currency: string
  days: CostDay[]
  today_total_usd: number
  month_total_usd: number
  budget_usd_per_day: number
  budget_usd_per_month: number
  warning_ratio: number
  exceeded_ratio: number
  budget_enforce: boolean
  over_budget: boolean
  month_over_budget: boolean
}

export interface CostStatus {
  currency: string
  today_total_usd: number
  month_total_usd: number
  budget_usd_per_day: number
  budget_usd_per_month: number
  day_ratio: number
  month_ratio: number
  day_alert: '' | 'warning' | 'exceeded'
  month_alert: '' | 'warning' | 'exceeded'
  alert_level: '' | 'warning' | 'exceeded'
  alert_scope: '' | 'day' | 'month'
}

export async function getCostSummary(days = 7): Promise<CostSummary> {
  const res = await fetch(`/v0/settings/cost-summary?days=${days}`, { headers: authInit() })
  return parseJSON(res)
}

export async function getCostStatus(): Promise<CostStatus> {
  const res = await fetch('/v0/settings/cost-status', { headers: authInit() })
  return parseJSON(res)
}

export async function putCostBudget(
  budgetUsdPerDay: number,
  budgetUsdPerMonth: number,
  enforce = false,
  warningRatio = 0.8,
  exceededRatio = 1,
): Promise<void> {
  const res = await fetch('/v0/settings/cost-budget', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({
      budget_usd_per_day: budgetUsdPerDay,
      budget_usd_per_month: budgetUsdPerMonth,
      warning_ratio: warningRatio,
      exceeded_ratio: exceededRatio,
      enforce,
    }),
  })
  await parseJSON(res)
}

export interface AgentCostBudget {
  agent_id: string
  budget_usd_per_day: number
  budget_usd_per_month: number
}

export async function getAgentCostBudgets(): Promise<{ items: AgentCostBudget[] }> {
  const res = await fetch('/v0/settings/agent-cost-budgets', { headers: authInit() })
  return parseJSON(res)
}

export async function putAgentCostBudget(body: AgentCostBudget): Promise<AgentCostBudget> {
  const res = await fetch('/v0/settings/agent-cost-budgets', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify(body),
  })
  return parseJSON(res)
}

