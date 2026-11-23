import type { SkillSummary } from '../../api'
import type { Locale } from '../../locale/types'
import { SKILLS } from '../../strings'

function builtinTitle(id: string): string {
  return SKILLS.builtinTitles[id]?.trim() ?? ''
}

/**
 * Catalog blurb for the active UI locale.
 * English UI must not fall back to a Chinese `description` when `description_en`
 * is missing (e.g. skill has no SKILL.en.md yet) — that freezes Settings on zh.
 */
export function skillLocalizedDescription(s: SkillSummary, locale: Locale): string {
  if (locale === 'en') {
    return s.description_en?.trim() ?? ''
  }
  return (s.description ?? '').trim()
}

/**
 * Locale-aware display title for skills list / toasts / completion.
 */
export function skillDisplayName(s: SkillSummary, locale: Locale = 'zh-CN'): string {
  if (s.source === 'managed' || s.id.startsWith('login-')) {
    const connectorId = s.id.startsWith('login-') ? s.id.slice('login-'.length) : s.id
    return SKILLS.managedLoginTitle(connectorId || s.id)
  }

  if (locale === 'en') {
    const en = s.description_en?.trim()
    if (en) return en
    // No English overlay: use UI map for builtins instead of the Chinese
    // description the API falls back to when DescriptionEN is empty.
    if (s.source === 'builtin') {
      const mapped = builtinTitle(s.id)
      if (mapped) return mapped
    }
    const name = s.name?.trim()
    if (name) return name
    return s.id
  }

  const desc = (s.description ?? '').trim()
  if (desc) return desc
  if (s.source === 'builtin') {
    const mapped = builtinTitle(s.id)
    if (mapped) return mapped
  }
  const name = s.name?.trim()
  if (name) return name
  return s.id
}

/** Whether Settings should render a second description line under the title. */
export function skillShowSecondaryDescription(s: SkillSummary, locale: Locale): boolean {
  if (s.source === 'managed' || s.id.startsWith('login-')) return false
  const title = skillDisplayName(s, locale)
  const desc = skillLocalizedDescription(s, locale)
  return Boolean(desc && desc !== title)
}

export function skillToolsCountLabel(count: number): string {
  if (count <= 0) return ''
  return SKILLS.toolsCount(count)
}

export function skillSourceLabel(source: SkillSummary['source']): string {
  switch (source) {
    case 'builtin':
      return SKILLS.sourceBuiltin
    case 'user':
      return SKILLS.sourceUser
    case 'managed':
      return SKILLS.sourceManaged
    default: {
      const _exhaustive: never = source
      return _exhaustive
    }
  }
}
