import { useEffect, useState, type KeyboardEvent } from 'react'

export interface ComposerProps {
  disabled?: boolean
  onSend: (text: string) => void
  draft?: string
}

export function Composer({ disabled, onSend, draft }: ComposerProps) {
  const [text, setText] = useState('')

  useEffect(() => {
    if (draft !== undefined) {
      setText(draft)
    }
  }, [draft])

  const submit = () => {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setText('')
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  return (
    <div className="composer">
      <button
        type="button"
        className="btn ghost composer-plus"
        title="连接器在设置中配置"
        disabled
        aria-label="连接器在设置中配置"
      >
        +
      </button>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder="输入消息…"
        rows={1}
        disabled={disabled}
        aria-label="消息输入"
      />
      <button type="button" className="btn primary" onClick={submit} disabled={disabled || !text.trim()}>
        发送
      </button>
    </div>
  )
}
