import { useMemo } from 'react'
import { Brain } from 'lucide-react'
import { DropdownMenu, type MenuItem } from './ui'
import { CHAT, MODELS } from '../strings'

export interface ThinkingChipProps {
  /** '' = follow model default; otherwise off|low|medium|high. */
  value: string
  onChange: (level: string) => void
  disabled?: boolean
}

const OPTIONS: { value: string; label: string }[] = [
  { value: '', label: CHAT.thinkingDefault },
  { value: 'off', label: MODELS.thinkingLevelOff },
  { value: 'low', label: MODELS.thinkingLevelLow },
  { value: 'medium', label: MODELS.thinkingLevelMedium },
  { value: 'high', label: MODELS.thinkingLevelHigh },
]

function labelFor(value: string): string {
  return OPTIONS.find((o) => o.value === value)?.label ?? CHAT.thinkingDefault
}

export function ThinkingChip({ value, onChange, disabled }: ThinkingChipProps) {
  const selected = value.trim()
  const label = labelFor(selected)

  const items: MenuItem[] = useMemo(
    () =>
      OPTIONS.map((o) => ({
        id: o.value || 'default',
        label: o.label,
        onSelect: () => onChange(o.value),
      })),
    [onChange],
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
