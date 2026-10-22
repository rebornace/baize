import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { Check, ChevronDown } from 'lucide-react'
import type { ModelProfile } from '../api'
import { useLocale } from '../locale/LocaleContext'
import { AUTO_MODEL_ID } from '../modelSelect'
import { AUTO_LABEL, CHAT, MODELS, tierLabel, VISION_LABEL } from '../strings'

export type ModelThinkingDualMenuProps = {
  profiles: ModelProfile[]
  modelId: string
  thinkingLevel: string
  onChooseModel: (id: string) => void
  onChooseThinking: (level: string) => void
  disabled?: boolean
}

function thinkingLevelLabel(level: string): string {
  switch (level) {
    case 'off':
      return MODELS.thinkingLevelOff
    case 'low':
      return MODELS.thinkingLevelLow
    case 'medium':
      return MODELS.thinkingLevelMedium
    case 'high':
      return MODELS.thinkingLevelHigh
    default:
      return CHAT.thinkingDefault
  }
}

function thinkingOptions(profileDefault?: string) {
  return [
    {
      value: '',
      label: profileDefault
        ? CHAT.thinkingDefaultWith(thinkingLevelLabel(profileDefault))
        : CHAT.thinkingDefault,
    },
    { value: 'off', label: MODELS.thinkingLevelOff },
    { value: 'low', label: MODELS.thinkingLevelLow },
    { value: 'medium', label: MODELS.thinkingLevelMedium },
    { value: 'high', label: MODELS.thinkingLevelHigh },
  ] as const
}

/**
 * Cursor-style model control: ghost text trigger inside the composer; one
 * popover with two sections (models + thinking depth for the current model).
 */
export function ModelThinkingDualMenu({
  profiles,
  modelId,
  thinkingLevel,
  onChooseModel,
  onChooseThinking,
  disabled,
}: ModelThinkingDualMenuProps) {
  const { locale } = useLocale()
  const [open, setOpen] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  const menuId = useId()

  const selected = modelId.trim() === '' ? AUTO_MODEL_ID : modelId
  const chosen = profiles.find((p) => p.id === selected)
  const modelLabel = chosen ? chosen.name : AUTO_LABEL
  const thinkingSelected = thinkingLevel.trim()
  const profileDefault = chosen?.thinking_level

  const thinkingOpts = useMemo(
    () => thinkingOptions(profileDefault),
    [profileDefault, locale],
  )
  const thinkingShort =
    thinkingOpts.find((o) => o.value === thinkingSelected)?.label ?? CHAT.thinkingDefault

  // Closed label mirrors Cursor: model name only when Auto; otherwise name · thinking.
  const triggerLabel =
    selected === AUTO_MODEL_ID
      ? AUTO_LABEL
      : thinkingSelected === ''
        ? modelLabel
        : `${modelLabel} · ${thinkingShort}`

  const close = useCallback(() => setOpen(false), [])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) close()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, close])

  const models = useMemo(() => {
    const rows: { id: string; label: string; hint?: string }[] = [
      { id: AUTO_MODEL_ID, label: AUTO_LABEL },
    ]
    for (const p of profiles) {
      const bits = [tierLabel(p.auto_tier)]
      if (p.supports_vision) bits.push(VISION_LABEL)
      bits.push(thinkingLevelLabel(p.thinking_level))
      rows.push({ id: p.id, label: p.name, hint: bits.join(' · ') })
    }
    return rows
  }, [profiles, locale])

  return (
    <div
      className="model-picker"
      data-testid="model-thinking-dual"
      ref={rootRef}
    >
      <button
        type="button"
        className="model-picker-trigger"
        data-testid="model-chip"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={CHAT.modelThinkingPairAria}
        title={CHAT.modelThinkingPairAria}
        disabled={disabled}
        onClick={() => {
          if (!disabled) setOpen((v) => !v)
        }}
      >
        <span className="model-picker-trigger-label" data-testid="thinking-chip">
          {triggerLabel}
        </span>
        <ChevronDown size={12} aria-hidden className="model-picker-caret" />
      </button>
      {open ? (
        <div
          id={menuId}
          className="model-picker-menu"
          role="menu"
          aria-label={CHAT.modelThinkingPairAria}
        >
          <div className="model-picker-section-label">{CHAT.selectModel}</div>
          {models.map((m) => {
            const active = m.id === selected
            return (
              <button
                key={m.id}
                type="button"
                role="menuitemradio"
                aria-checked={active}
                className={`model-picker-item${active ? ' active' : ''}`}
                onClick={() => {
                  onChooseModel(m.id)
                }}
              >
                <span className="model-picker-check" aria-hidden>
                  {active ? <Check size={14} /> : null}
                </span>
                <span className="model-picker-item-text">
                  <span className="model-picker-item-title">{m.label}</span>
                  {m.hint ? (
                    <span className="model-picker-item-hint">{m.hint}</span>
                  ) : null}
                </span>
              </button>
            )
          })}
          <div className="model-picker-divider" role="separator" />
          <div className="model-picker-section-label">{CHAT.thinkingChip}</div>
          {thinkingOpts.map((o) => {
            const active = o.value === thinkingSelected
            return (
              <button
                key={o.value || 'default'}
                type="button"
                role="menuitemradio"
                aria-checked={active}
                className={`model-picker-item${active ? ' active' : ''}`}
                onClick={() => {
                  onChooseThinking(o.value)
                  close()
                }}
              >
                <span className="model-picker-check" aria-hidden>
                  {active ? <Check size={14} /> : null}
                </span>
                <span className="model-picker-item-text">
                  <span className="model-picker-item-title">{o.label}</span>
                </span>
              </button>
            )
          })}
        </div>
      ) : null}
    </div>
  )
}
