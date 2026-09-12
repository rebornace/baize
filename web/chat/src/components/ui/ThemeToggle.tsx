import { Monitor, Moon, Sun } from 'lucide-react'
import { useTheme } from '../../useTheme'
import type { ThemeChoice } from '../../theme'

const OPTIONS: Array<{ value: ThemeChoice; label: string; Icon: typeof Sun }> = [
  { value: 'light', label: '浅色', Icon: Sun },
  { value: 'system', label: '跟随系统', Icon: Monitor },
  { value: 'dark', label: '深色', Icon: Moon },
]

export function ThemeToggle() {
  const { choice, setChoice } = useTheme()
  return (
    <div className="ui-theme-toggle" role="group" aria-label="主题外观" data-testid="ui-theme-toggle">
      {OPTIONS.map(({ value, label, Icon }) => (
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
      ))}
    </div>
  )
}
