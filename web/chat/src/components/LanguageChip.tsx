import { Languages } from 'lucide-react'
import { useLocale } from '../locale/LocaleContext'
import type { Locale } from '../locale/types'

export function LanguageChip() {
  const { locale, setLocale, strings } = useLocale()
  const L = strings.LOCALE

  const choose = (next: Locale) => {
    if (next !== locale) setLocale(next)
  }

  return (
    <div
      className="model-chip language-chip"
      role="group"
      aria-label={L.chipAria}
      data-testid="language-chip"
    >
      <span className="model-chip-inner language-chip-inner">
        <Languages size={14} aria-hidden />
        <button
          type="button"
          className="language-chip-opt"
          aria-pressed={locale === 'zh-CN'}
          onClick={() => choose('zh-CN')}
        >
          {L.chipZh}
        </button>
        <span className="language-chip-sep" aria-hidden>
          /
        </span>
        <button
          type="button"
          className="language-chip-opt"
          aria-pressed={locale === 'en'}
          onClick={() => choose('en')}
        >
          {L.chipEn}
        </button>
      </span>
    </div>
  )
}
