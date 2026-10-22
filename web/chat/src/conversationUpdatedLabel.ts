import type { Locale } from './locale/types'

function localeTag(locale: Locale): string {
  return locale === 'en' ? 'en-US' : 'zh-CN'
}

/** Sidebar meta line for conversation `updated_at` (locale-aware, compact). */
export function conversationUpdatedLabel(iso: string, locale: Locale): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''

  const now = new Date()
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()

  const tag = localeTag(locale)
  if (sameDay) {
    return new Intl.DateTimeFormat(tag, { hour: 'numeric', minute: '2-digit' }).format(date)
  }
  if (date.getFullYear() === now.getFullYear()) {
    return new Intl.DateTimeFormat(tag, { month: 'short', day: 'numeric' }).format(date)
  }
  return new Intl.DateTimeFormat(tag, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date)
}
