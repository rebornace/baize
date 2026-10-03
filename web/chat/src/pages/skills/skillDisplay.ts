import type { SkillSummary } from '../../api'
import { SKILLS } from '../../strings'

/** Locale-aware display title for skills list / toasts. */
export function skillDisplayName(s: SkillSummary): string {
  if (s.source === 'builtin') {
    const localized = SKILLS.builtinTitles[s.id]
    if (localized?.trim()) return localized.trim()
  }
  if (s.source === 'managed' || s.id.startsWith('login-')) {
    const connectorId = s.id.startsWith('login-') ? s.id.slice('login-'.length) : s.id
    return SKILLS.managedLoginTitle(connectorId || s.id)
  }
  // User-authored: keep author text (manual).
  const desc = s.description?.trim()
  if (desc) return desc
  const name = s.name?.trim()
  if (name) return name
  return s.id
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
