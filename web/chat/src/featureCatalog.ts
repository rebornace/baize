import {
  Brain,
  Copy,
  Cpu,
  Info,
  LayoutGrid,
  LogIn,
  MessageSquarePlus,
  Paperclip,
  PencilLine,
  RefreshCw,
  Square,
  UserCheck,
  type LucideIcon,
} from 'lucide-react'
import { getPack } from './locale/pack'
import { OVERVIEW_ITEM, visibleNavItems, type SettingsRole } from './settingsNav'

export type FeatureKind = 'page' | 'action'
export type FeatureGroup = 'settings' | 'chat'

export type FeatureEntry = {
  id: string
  kind: FeatureKind
  group: FeatureGroup
  title: string
  description: string
  keywords: string[]
  to?: string
  location?: string
  icon: LucideIcon
  order: number
}

const PAGE_KEYWORDS: Record<string, string[]> = {
  '/settings': ['设置', '总览', 'settings', 'overview', '全部'],
  '/settings/models': ['模型', 'model', 'provider', 'api', 'deepseek', 'ollama'],
  '/settings/tools': ['工具', 'tool', 'action', '权限', '审批'],
  '/settings/skills': ['技能', 'skill', '流程', 'workflow'],
  '/settings/memory': ['记忆', 'memory', '偏好', 'preference', '事实'],
  '/settings/schedules': ['日程', '定时', 'schedule', 'cron', '提醒'],
  '/settings/tool-matching': ['工具匹配', '匹配', 'routing', 'retrieval', '检索'],
  '/settings/openapi': ['连接器', '接口', 'openapi', 'swagger', 'api', '业务系统'],
  '/settings/mcp': ['mcp', '扩展工具', 'server'],
  '/settings/plugins': ['插件', 'plugin', 'http'],
  '/settings/external-login': ['外部登录', '登录', 'login', 'sso', 'oauth'],
  '/settings/mcp-export': ['mcp 导出', '导出', 'export', '对外'],
  '/settings/channels/weixin': ['微信', 'weixin', 'channel', '渠道'],
  '/settings/webhooks': ['webhook', '回调', '事件', 'events'],
  '/settings/inbox': ['inbox', '收件', '外部消息', '邮件'],
  '/settings/identities': ['账号', 'identity', '登录', '凭证'],
  '/settings/storage': ['存储', 'storage', 'sqlite', 's3', '数据库'],
  '/settings/metrics': ['指标', 'metrics', '监控', '日志'],
  '/settings/eval': ['评测', 'eval', '测试', '评分'],
  '/settings/runtime': ['运行参数', 'runtime', '模型路由', '上下文', '压缩'],
}

const DEFAULT_IDS = [
  'page:/settings',
  'action:view_run_binding',
  'page:/settings/memory',
  'page:/settings/schedules',
  'page:/settings/models',
  'action:new_chat',
]

export function featureEntries(role: SettingsRole): FeatureEntry[] {
  if (role === 'viewer') return []
  const pack = getPack().FEATURE_SEARCH
  const overview: FeatureEntry = {
    id: 'page:/settings',
    kind: 'page',
    group: 'settings',
    title: OVERVIEW_ITEM.label,
    description: pack.overviewDesc,
    keywords: PAGE_KEYWORDS['/settings'] ?? [],
    to: '/settings',
    icon: LayoutGrid,
    order: 0,
  }
  const pages = visibleNavItems(role).map((item, idx): FeatureEntry => ({
    id: `page:${item.to}`,
    kind: 'page',
    group: 'settings',
    title: item.label,
    description: item.desc,
    keywords: PAGE_KEYWORDS[item.to] ?? [],
    to: item.to,
    icon: item.icon,
    order: 10 + idx,
  }))
  const a = pack.actions
  const actions: FeatureEntry[] = [
    {
      id: 'action:view_run_binding', kind: 'action', group: 'chat',
      title: a.viewRunBinding.title, description: a.viewRunBinding.desc,
      keywords: ['运行绑定', 'binding', 'run binding', '绑定', '模型', '工具契约'],
      to: '/', location: a.viewRunBinding.location, icon: Info, order: 100,
    },
    {
      id: 'action:regenerate', kind: 'action', group: 'chat',
      title: a.regenerate.title, description: a.regenerate.desc,
      keywords: ['重新回答', 'regenerate', '重试'],
      to: '/', location: a.regenerate.location, icon: RefreshCw, order: 101,
    },
    {
      id: 'action:copy_reply', kind: 'action', group: 'chat',
      title: a.copyReply.title, description: a.copyReply.desc,
      keywords: ['复制', 'copy', '复制回复'],
      to: '/', location: a.copyReply.location, icon: Copy, order: 102,
    },
    {
      id: 'action:edit_reanswer', kind: 'action', group: 'chat',
      title: a.editReanswer.title, description: a.editReanswer.desc,
      keywords: ['编辑', 'edit', '重新回答'],
      to: '/', location: a.editReanswer.location, icon: PencilLine, order: 103,
    },
    {
      id: 'action:stop_run', kind: 'action', group: 'chat',
      title: a.stopRun.title, description: a.stopRun.desc,
      keywords: ['停止', 'stop', '取消', 'cancel'],
      to: '/', location: a.stopRun.location, icon: Square, order: 104,
    },
    {
      id: 'action:new_chat', kind: 'action', group: 'chat',
      title: a.newChat.title, description: a.newChat.desc,
      keywords: ['新建对话', 'new chat', '新会话'],
      to: '/', location: a.newChat.location, icon: MessageSquarePlus, order: 105,
    },
    {
      id: 'action:model_select', kind: 'action', group: 'chat',
      title: a.modelSelect.title, description: a.modelSelect.desc,
      keywords: ['模型', 'model', '切换模型', '智能选择'],
      to: '/', location: a.modelSelect.location, icon: Cpu, order: 106,
    },
    {
      id: 'action:thinking_depth', kind: 'action', group: 'chat',
      title: a.thinkingDepth.title, description: a.thinkingDepth.desc,
      keywords: ['思考深度', 'thinking', 'reasoning'],
      to: '/', location: a.thinkingDepth.location, icon: Brain, order: 107,
    },
    {
      id: 'action:approve_reject', kind: 'action', group: 'chat',
      title: a.approveReject.title, description: a.approveReject.desc,
      keywords: ['审批', 'approve', 'reject', '拒绝', '同意', 'hitl'],
      to: '/', location: a.approveReject.location, icon: UserCheck, order: 108,
    },
    {
      id: 'action:browser_login', kind: 'action', group: 'chat',
      title: a.browserLogin.title, description: a.browserLogin.desc,
      keywords: ['浏览器登录', 'login', 'browser', '登录'],
      to: '/', location: a.browserLogin.location, icon: LogIn, order: 109,
    },
    {
      id: 'action:upload_attachment', kind: 'action', group: 'chat',
      title: a.uploadAttachment.title, description: a.uploadAttachment.desc,
      keywords: ['上传附件', '附件', '图片', 'upload', 'image', 'file'],
      to: '/', location: a.uploadAttachment.location, icon: Paperclip, order: 110,
    },
  ]
  return [overview, ...pages, ...actions]
}

export function searchFeatures(entries: FeatureEntry[], query: string): FeatureEntry[] {
  const q = query.trim().toLowerCase()
  if (!q) {
    const byId = new Map(entries.map((e) => [e.id, e]))
    return DEFAULT_IDS.map((id) => byId.get(id)).filter((e): e is FeatureEntry => Boolean(e))
  }
  const tokens = q.split(/\s+/).filter(Boolean)
  const scored = entries.flatMap((e) => {
    const title = e.title.toLowerCase()
    const desc = e.description.toLowerCase()
    const keywords = e.keywords.join(' ').toLowerCase()
    const location = (e.location ?? '').toLowerCase()
    const path = (e.to ?? '').toLowerCase()
    const hay = [title, desc, keywords, location, path].join('\n')
    if (!tokens.every((t) => hay.includes(t))) return []
    let score = 0
    if (title === q) score += 100
    else if (title.startsWith(q)) score += 80
    else if (title.includes(q)) score += 60
    if (keywords.includes(q)) score += 40
    if (desc.includes(q)) score += 20
    if (location.includes(q)) score += 10
    return [{ e, score }]
  })
  scored.sort((x, y) => y.score - x.score || x.e.order - y.e.order)
  return scored.slice(0, 10).map((x) => x.e)
}

export function navigationForEntry(entry: FeatureEntry): { to?: string; showHint: boolean } {
  return { to: entry.to, showHint: entry.kind === 'action' }
}
