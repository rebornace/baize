import { useMemo } from 'react'
import { Cpu } from 'lucide-react'
import type { ModelProfile } from '../api'
import { AUTO_MODEL_ID } from '../modelSelect'
import { AUTO_LABEL, tierLabel, VISION_LABEL } from '../strings'
import { DropdownMenu, type MenuItem } from './ui'

export interface ModelChipProps {
  profiles: ModelProfile[]
  value: string
  onChange: (id: string) => void
  disabled?: boolean
}

export function ModelChip({ profiles, value, onChange, disabled }: ModelChipProps) {
  const selected = value.trim() === '' ? AUTO_MODEL_ID : value
  const chosen = profiles.find((p) => p.id === selected)
  const label = chosen ? chosen.name : AUTO_LABEL

  const items: MenuItem[] = useMemo(() => {
    const autoItem: MenuItem = {
      id: AUTO_MODEL_ID,
      label: AUTO_LABEL,
      onSelect: () => onChange(AUTO_MODEL_ID),
    }
    const rest: MenuItem[] = profiles.map((p) => ({
      id: p.id,
      label: `${p.name} · ${tierLabel(p.auto_tier)}${p.supports_vision ? ` · ${VISION_LABEL}` : ''}`,
      onSelect: () => onChange(p.id),
    }))
    return [autoItem, ...rest]
  }, [profiles, onChange])

  return (
    <DropdownMenu
      triggerLabel="选择模型"
      triggerClassName="model-chip"
      items={items}
      disabled={disabled}
    >
      <span data-testid="model-chip" className="model-chip-inner">
        <Cpu size={14} aria-hidden /> {label}
      </span>
    </DropdownMenu>
  )
}
