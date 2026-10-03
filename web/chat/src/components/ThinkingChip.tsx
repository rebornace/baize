import { useMemo } from 'react'
import { Brain } from 'lucide-react'
import { useLocale } from '../locale/LocaleContext'
import { CHAT, MODELS } from '../strings'
import { DropdownMenu, type MenuItem } from './ui'

export interface ThinkingChipProps {
  /** '' = follow model default; otherwise off|low|medium|high. */
  value: string
  onChange: (level: string) => void
  disabled?: boolean
}

function thinkingOptions() {
  return [
    { value: '', label: CHAT.thinkingDefault },
    { value: 'off', label: MODELS.thinkingLevelOff },
    { value: 'low', label: MODELS.thinkingLevelLow },
    { value: 'medium', label: MODELS.thinkingLevelMedium },
    { value: 'high', label: MODELS.thinkingLevelHigh },
  ] as const
}

export function ThinkingChip({ value, onChange, disabled }: ThinkingChipProps) {
  const { locale } = useLocale()
  const selected = value.trim()
  const options = useMemo(() => thinkingOptions(), [locale])
  const label = options.find((o) => o.value === selected)?.label ?? CHAT.thinkingDefault

  const items: MenuItem[] = useMemo(
    () =>
      options.map((o) => ({
        id: o.value || 'default',
        label: o.label,
        onSelect: () => onChange(o.value),
      })),
    [options, onChange],
  )

  return (
    <DropdownMenu
      triggerLabel={CHAT.thinkingChip}
      triggerClassName="model-chip thinking-chip"
      items={items}
      disabled={disabled}
    >
      <span data-testid="thinking-chip" className="model-chip-inner">
        <Brain size={14} aria-hidden /> {label}
      </span>
    </DropdownMenu>
  )
}
