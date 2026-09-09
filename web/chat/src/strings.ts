import { ApiError } from './api'

// ---- 模型档位 / 能力（内部 id 不变，仅展示更名）----
export type TierId = 'light' | 'standard' | 'power'

export const TIER_LABELS: Record<TierId, string> = {
  light: '快速',
  standard: '标准',
  power: '深度思考',
}

/** 内部档位 id -> 对外叫法；未知值按「标准」。 */
export function tierLabel(tier?: string): string {
  if (tier === 'light' || tier === 'standard' || tier === 'power') {
    return TIER_LABELS[tier]
  }
  return TIER_LABELS.standard
}

export const AUTO_LABEL = '智能选择'
export const VISION_LABEL = '能看图'

// ---- 聊天动作 ----
export const ACTIONS = {
  copy: '复制',
  regenerate: '重新回答',
  editAndReanswer: '编辑后重新回答',
  forkAsNew: '复制成新对话',
  rollbackHere: '回到这里',
  more: '更多操作',
} as const

// ---- HITL ----
export const HITL = {
  title: '需要你确认',
  approve: '同意',
  reject: '拒绝',
  commentPlaceholder: '留言（选填）',
  approved: '已同意',
  rejected: '已拒绝',
} as const

// ---- 工作流 ----
export const WORKFLOW_PREPARING = '工作流准备中'

// ---- 欢迎区 ----
export const WELCOME = {
  title: '有什么可以帮你？',
  subtitle: '我可以查询数据、调用业务系统、处理文件与图片，直接说出你的需求即可。',
} as const

// ---- 高级项 ----
export const ADVANCED = {
  summary: '高级',
  tokenLabel: '临时访问凭证（选填）',
  tokenHint: '需要带身份访问时填写，仅本次会话使用。',
  webhookLabel: '本次结果回调地址（选填）',
} as const

export interface FriendlyError {
  title: string
  /** 技术细节，默认收起，供「详情」展开。 */
  detail?: string
}

const CODE_TITLE: Record<string, string> = {
  conversation_busy: '上一条还在处理中，请稍候再发。',
  no_model_configured: '还没有可用的 AI 模型，请到「设置 → AI 模型」添加一个。',
  vision_unsupported: '当前模型看不了图片，请改用「智能选择」或带「能看图」标记的模型。',
  invalid_signature: '连接校验未通过，请刷新页面后重试。',
  not_found: '内容不存在或已被删除。',
  internal_error: '服务暂时出了点问题，请稍后重试。',
}

function isNetworkish(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? '')
  return /failed to fetch|networkerror|load failed|network request/i.test(msg)
}

/** 把任意抛出值翻译为人话标题；未知错误附带可展开的技术 detail。 */
export function friendlyError(e: unknown): FriendlyError {
  if (e instanceof ApiError) {
    // invalid_signature 通常以 401 返回，但其指引是「刷新页面」，
    // 需先于通用 401/403「重新解锁」文案按错误码精确映射。
    const codeTitle = CODE_TITLE[e.code]
    if (codeTitle) return { title: codeTitle }
    if (e.status === 401 || e.status === 403) {
      return { title: '没有访问权限或登录已失效，请重新解锁后再试。' }
    }
    if (e.status >= 500) {
      return { title: CODE_TITLE.internal_error, detail: `${e.code}: ${e.message}` }
    }
    return { title: '操作未能完成，请稍后重试。', detail: `${e.code}: ${e.message}` }
  }
  if (isNetworkish(e)) {
    return { title: '网络连接失败，请检查服务是否正在运行。' }
  }
  const detail = e instanceof Error ? e.message : String(e ?? '')
  return { title: '出现了未知问题。', detail: detail || undefined }
}
