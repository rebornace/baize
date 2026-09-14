import { useEffect, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { ChatBlock } from '../foldEvents'
import { CHAT } from '../strings'
import { TypewriterText } from './TypewriterText'

export type ThinkingChatBlock = Extract<ChatBlock, { kind: 'thinking' }>

export interface ThinkingBlockProps {
  block: ThinkingChatBlock
  /** History / message fallback: never typewriter-active. */
  readOnly?: boolean
}

export function ThinkingBlock({ block, readOnly = false }: ThinkingBlockProps) {
  const streaming = !readOnly && block.status === 'streaming'
  const [open, setOpen] = useState(streaming)

  useEffect(() => {
    if (streaming) {
      setOpen(true)
      return
    }
    if (block.collapsed || block.status === 'done' || block.status === 'redacted') {
      setOpen(false)
    }
  }, [streaming, block.collapsed, block.status])

  if (block.status === 'redacted') {
    return (
      <div className="thinking-block thinking-block-redacted" data-testid="thinking-block">
        <p className="thinking-block-redacted-text">{CHAT.thinkingRedacted}</p>
      </div>
    )
  }

  const label = streaming ? CHAT.thinking : CHAT.thoughtDone
  const showBody = streaming || open

  return (
    <div
      className={`thinking-block${showBody ? ' thinking-block-open' : ''}`}
      data-testid="thinking-block"
      data-status={block.status}
    >
      <button
        type="button"
        className="thinking-block-toggle"
        aria-expanded={showBody}
        onClick={() => {
          if (streaming) return
          setOpen((v) => !v)
        }}
        disabled={streaming}
      >
        {showBody ? (
          <ChevronDown size={14} aria-hidden />
        ) : (
          <ChevronRight size={14} aria-hidden />
        )}
        <span>{label}</span>
      </button>
      {showBody && (
        <div className="thinking-block-body">
          <TypewriterText text={block.text} active={streaming} />
        </div>
      )}
    </div>
  )
}

/** Message-field fallback when run events have no per-turn thinking blocks. */
export function MessageThinkingFallback({
  thinking,
  redacted,
}: {
  thinking?: string | null
  redacted?: boolean
}) {
  const [open, setOpen] = useState(false)

  if (redacted) {
    return (
      <div className="thinking-block thinking-block-redacted" data-testid="message-thinking">
        <p className="thinking-block-redacted-text">{CHAT.thinkingRedacted}</p>
      </div>
    )
  }

  const text = thinking?.trim()
  if (!text) return null

  return (
    <div
      className={`thinking-block${open ? ' thinking-block-open' : ''}`}
      data-testid="message-thinking"
    >
      <button
        type="button"
        className="thinking-block-toggle"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
      >
        {open ? <ChevronDown size={14} aria-hidden /> : <ChevronRight size={14} aria-hidden />}
        <span>{open ? CHAT.thinking : CHAT.viewThinking}</span>
      </button>
      {open && (
        <div className="thinking-block-body">
          <TypewriterText text={text} active={false} />
        </div>
      )}
    </div>
  )
}
