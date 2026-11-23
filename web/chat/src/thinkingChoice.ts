import type { ThinkingLevel } from './api'

export const THINKING_LEVELS = ['off', 'low', 'medium', 'high'] as const

export function thinkingChoiceKey(conversationId: string): string {
  return `baize.thinkingLevel:${conversationId}`
}

function isThinkingLevel(v: string): v is ThinkingLevel {
  return (THINKING_LEVELS as readonly string[]).includes(v)
}

/** 本会话思考覆盖；无存储 / 非法 → ''（跟模型默认）。 */
export function loadThinkingChoice(conversationId: string): string {
  try {
    const raw = sessionStorage.getItem(thinkingChoiceKey(conversationId))?.trim() ?? ''
    if (!raw) return ''
    return isThinkingLevel(raw) ? raw : ''
  } catch {
    return ''
  }
}

/** 写入本会话覆盖；空串清除 key。隐私模式等写入失败时静默。 */
export function saveThinkingChoice(conversationId: string, level: string): void {
  const key = thinkingChoiceKey(conversationId)
  try {
    const v = level.trim()
    if (!v) {
      sessionStorage.removeItem(key)
      return
    }
    if (!isThinkingLevel(v)) return
    sessionStorage.setItem(key, v)
  } catch {
    /* ignore */
  }
}
