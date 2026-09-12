import type { ModelProfile } from './api'
import { AUTO_MODEL_ID } from './modelSelect'

export const MODEL_CHOICE_KEY = 'baize.model_choice'

/** 读取持久化选择；缺省/异常一律 Auto。 */
export function loadModelChoice(): string {
  try {
    return localStorage.getItem(MODEL_CHOICE_KEY)?.trim() || AUTO_MODEL_ID
  } catch {
    return AUTO_MODEL_ID
  }
}

/** 持久化选择；隐私模式等写入失败时静默（内存态仍生效）。 */
export function saveModelChoice(id: string): void {
  try {
    localStorage.setItem(MODEL_CHOICE_KEY, id.trim() || AUTO_MODEL_ID)
  } catch {
    /* ignore */
  }
}

export interface ResolvedChoice {
  choice: string
  /** 持久化的具体模型已不存在（被删），调用方应提示并回退 Auto。 */
  stale: boolean
}

/** Auto 永远有效；具体模型必须仍存在，否则回退 Auto 并标记 stale。 */
export function resolveModelChoice(profiles: Pick<ModelProfile, 'id'>[]): ResolvedChoice {
  const raw = loadModelChoice()
  if (raw === AUTO_MODEL_ID) return { choice: AUTO_MODEL_ID, stale: false }
  if (profiles.some((p) => p.id === raw)) return { choice: raw, stale: false }
  return { choice: AUTO_MODEL_ID, stale: true }
}
