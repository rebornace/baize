import { useEffect, useRef, useState, type KeyboardEvent } from 'react'

export interface ComposerProps {
  disabled?: boolean
  onSend: (text: string) => void
  draft?: string
}

export function Composer({ disabled, onSend, draft }: ComposerProps) {
  const [text, setText] = useState('')
  const taRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (draft !== undefined) {
      setText(draft)
    }
  }, [draft])

  useEffect(() => {
    const el = taRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`
  }, [text])

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
      <div className="composer-box">
        <textarea
          ref={taRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          placeholder="输入消息，Enter 发送，Shift+Enter 换行"
          rows={1}
          disabled={disabled}
          aria-label="消息输入"
        />
        <button
          type="button"
          className="btn primary composer-send"
          onClick={submit}
          disabled={disabled || !text.trim()}
        >
          发送
        </button>
      </div>
    </div>
  )
}
