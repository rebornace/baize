import type { ModelProfile } from '../api'
import { modelOptions } from '../modelSelect'

export interface ModelSelectProps {
  profiles: ModelProfile[]
  /** "" means use the server-side default model. */
  value: string
  onChange: (id: string) => void
  disabled?: boolean
}

/**
 * Per-message model picker for the chat composer. Rendered only when profiles
 * are available; the first option ("") always means the default model so an
 * explicit id is sent solely for a deliberate choice.
 */
export function ModelSelect({ profiles, value, onChange, disabled }: ModelSelectProps) {
  if (profiles.length === 0) return null
  const options = modelOptions(profiles)
  return (
    <label className="chat-model-row">
      <span className="chat-model-label">模型</span>
      <select
        className="chat-model-select"
        value={value}
        disabled={disabled}
        aria-label="选择本次消息使用的模型"
        onChange={(e) => onChange(e.target.value)}
      >
        {options.map((o) => (
          <option key={o.value || 'default'} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  )
}
