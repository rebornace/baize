import { Monitor, Moon, Sun } from 'lucide-react'
import { useTheme } from '../../useTheme'
import type { ThemeChoice } from '../../theme'
import { THEME } from '../../strings'

const ICONS: Record<ThemeChoice, typeof Sun> = {
  light: Sun,
  system: Monitor,
  dark: Moon,
}

export function ThemeToggle() {
  const { choice, setChoice } = useTheme()
  const options: Array<{ value: ThemeChoice; label: string }> = [
    { value: 'light', label: THEME.light },
    { value: 'system', label: THEME.system },
    { value: 'dark', label: THEME.dark },
  ]
  return (
    <div className="ui-theme-toggle" role="group" aria-label={THEME.groupAria} data-testid="ui-theme-toggle">
      {options.map(({ value, label }) => {
        const Icon = ICONS[value]
        return (
          <button
            key={value}
            type="button"
            className="ui-theme-btn"
            aria-label={label}
            aria-pressed={choice === value}
            onClick={() => setChoice(value)}
          >
            <Icon size={15} aria-hidden="true" />
          </button>
        )
      })}
    </div>
  )
}
