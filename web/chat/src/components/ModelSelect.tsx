import type { ModelProfile } from '../api'
import { AUTO_MODEL_ID, modelOptions } from '../modelSelect'

export interface ModelSelectProps {
  profiles: ModelProfile[]
  /** AUTO_MODEL_ID (or "") means smart routing; otherwise a concrete id. */
  value: string
  onChange: (id: string) => void
  disabled?: boolean
}

/**
 * Per-message model picker for the chat composer. Rendered only when profiles
 * are available. The first option is "智能路由 (Auto)" (smart routing); every
 * configured profile is a manual choice that pins that exact model.
 */
export function ModelSelect({ profiles, value, onChange, disabled }: ModelSelectProps) {
  if (profiles.length === 0) return null
  const options = modelOptions(profiles)
  const selected = value.trim() === '' ? AUTO_MODEL_ID : value
  return (
    <label className="chat-model-row">
      <span className="chat-model-label">模型</span>
      <select
        className="chat-model-select"
        value={selected}
        disabled={disabled}
        aria-label="选择本次消息使用的模型"
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}
