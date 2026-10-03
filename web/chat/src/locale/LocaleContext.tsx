import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { packs, type StringsPack } from '../locales'
import { setPack } from './pack'
import { resolveLocale } from './resolve'
import { writeStoredLocale } from './storage'
import type { Locale } from './types'

export type LocaleContextValue = {
  locale: Locale
  setLocale: (next: Locale) => void
  strings: StringsPack
}

const LocaleContext = createContext<LocaleContextValue | null>(null)

function applyDocumentLang(locale: Locale): void {
  if (typeof document === 'undefined') return
  document.documentElement.lang = locale
}

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(() => {
    const initial = resolveLocale()
    setPack(packs[initial])
    applyDocumentLang(initial)
    return initial
  })

  // Keep module pack + <html lang> in sync with React state (sync on change, not only after paint).
  setPack(packs[locale])
  applyDocumentLang(locale)

  const setLocale = useCallback((next: Locale) => {
    writeStoredLocale(next)
    setPack(packs[next])
    applyDocumentLang(next)
    setLocaleState(next)
  }, [])

  const value = useMemo<LocaleContextValue>(
    () => ({
      locale,
      setLocale,
      strings: packs[locale],
    }),
    [locale, setLocale],
  )

  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>
}

export function useLocale(): LocaleContextValue {
  const ctx = useContext(LocaleContext)
  if (!ctx) {
    throw new Error('useLocale must be used within LocaleProvider')
  }
  return ctx
}

export function useStrings(): StringsPack {
  return useLocale().strings
}
