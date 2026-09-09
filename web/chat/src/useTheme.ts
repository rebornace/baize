import { useCallback, useEffect, useState } from 'react'
import {
  THEME_KEY,
  applyTheme,
  readStoredTheme,
  resolveTheme,
  systemTheme,
  type ThemeChoice,
  type ThemeMode,
} from './theme'

function isChoice(raw: string): raw is ThemeChoice {
  return raw === 'light' || raw === 'dark' || raw === 'system'
}

function systemNow(): ThemeMode {
  // 仅用于把当前系统明暗解析成可应用模式；system 选择项本身不被持久化为 light/dark。
  return systemTheme(window.matchMedia('(prefers-color-scheme: dark)').matches)
}

/** 主题选择的唯一状态入口：持久化 + 应用 + 跟随系统变化。 */
export function useTheme() {
  const [choice, setChoiceState] = useState<ThemeChoice>(() => {
    const raw = readStoredTheme(
      typeof localStorage === 'undefined' ? undefined : localStorage,
    )
    return isChoice(raw) ? raw : 'system'
  })

  useEffect(() => {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const apply = () =>
      applyTheme(
        resolveTheme(readStoredTheme(localStorage), () => systemTheme(mq.matches)),
        document.documentElement,
      )
    apply()
    mq.addEventListener('change', apply)
    return () => mq.removeEventListener('change', apply)
  }, [choice])

  const setChoice = useCallback((next: ThemeChoice) => {
    try {
      localStorage.setItem(THEME_KEY, next)
    } catch {
      /* 隐私模式等：本次仍生效，只是不持久化 */
    }
    applyTheme(resolveTheme(next, () => systemNow()), document.documentElement)
    setChoiceState(next)
  }, [])

  return { choice, setChoice }
}
