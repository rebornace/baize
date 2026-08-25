import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { activeMention, replaceMention } from '../skillMention'
import type { SkillSummary } from '../api'

const ACCEPT =
  '.txt,.md,.csv,.docx,.xlsx,.pdf,.png,.jpg,.jpeg,.webp,.gif'

export interface ComposerProps {
  disabled?: boolean
  onSend: (text: string, files: File[]) => void
  draft?: string
  /** Skills available for @-completion. Omit to disable the popup. */
  skills?: SkillSummary[]
}

interface Completion {
  start: number
  end: number
  query: string
  matches: SkillSummary[]
  activeIndex: number
}

export function Composer({ disabled, onSend, draft, skills }: ComposerProps) {
  const [text, setText] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [completion, setCompletion] = useState<Completion | null>(null)
  const taRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

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

  const skillsById = useMemo(() => {
    const map = new Map<string, SkillSummary>()
    for (const s of skills ?? []) map.set(s.id, s)
    return map
  }, [skills])

  const updateCompletion = (value: string, caret: number) => {
    if (!skills || skills.length === 0) {
      setCompletion(null)
      return
    }
    const active = activeMention(value, caret)
    if (!active) {
      setCompletion(null)
      return
    }
    const q = active.query.toLowerCase()
    const matches = skills.filter((s) => s.id.toLowerCase().startsWith(q))
    if (matches.length === 0) {
      setCompletion(null)
      return
    }
    setCompletion({
      start: active.start,
      end: active.end,
      query: active.query,
      matches,
      activeIndex: 0,
    })
  }

  const applyCompletion = (pick: SkillSummary) => {
    if (!completion) return
    const { text: next, caret } = replaceMention(text, completion.start, completion.end, pick.id)
    setText(next)
    setCompletion(null)
    requestAnimationFrame(() => {
      const el = taRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(caret, caret)
    })
  }

  const submit = () => {
    const trimmed = text.trim()
    if ((!trimmed && files.length === 0) || disabled) return
    onSend(trimmed, files)
    setText('')
    setFiles([])
    setCompletion(null)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (completion) {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setCompletion((c) =>
          c ? { ...c, activeIndex: (c.activeIndex + 1) % c.matches.length } : c,
        )
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setCompletion((c) =>
          c
            ? {
                ...c,
                activeIndex: (c.activeIndex - 1 + c.matches.length) % c.matches.length,
              }
            : c,
        )
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        const pick = completion.matches[completion.activeIndex]
        if (pick) applyCompletion(pick)
        return
      }
      if (e.key === 'Escape') {
        e.preventDefault()
        setCompletion(null)
        return
      }
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      submit()
    }
  }

  const addFiles = (list: FileList | null) => {
    if (!list || list.length === 0) return
    const incoming = Array.from(list)
    setFiles((prev) => {
      const merged = [...prev]
      for (const f of incoming) {
        if (!merged.some((m) => m.name === f.name && m.size === f.size)) {
          merged.push(f)
        }
      }
      return merged.slice(0, 5)
    })
    if (fileInputRef.current) fileInputRef.current.value = ''
  }

  const removeFile = (index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index))
  }

  const canSend = !disabled && (text.trim().length > 0 || files.length > 0)

  const openFilePicker = () => {
    fileInputRef.current?.click()
  }

  return (
    <div className="composer">
      {files.length > 0 && (
        <div className="composer-chips" aria-label="附件列表">
          {files.map((f, i) => (
            <span key={`${f.name}-${i}`} className="composer-chip">
              <span className="composer-chip-name" title={f.name}>{f.name}</span>
              <button
                type="button"
                className="composer-chip-remove"
                aria-label={`移除 ${f.name}`}
                onClick={() => removeFile(i)}
                disabled={disabled}
              >
                ×
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="composer-box">
        <input
          ref={fileInputRef}
          type="file"
          multiple
          accept={ACCEPT}
          className="composer-file-input"
          aria-hidden="true"
          tabIndex={-1}
          onChange={(e) => addFiles(e.target.files)}
          disabled={disabled}
        />
        <button
          type="button"
          className="composer-attach"
          aria-label="添加附件"
          title="添加附件"
          onClick={openFilePicker}
          disabled={disabled}
        >
          <svg
            viewBox="0 0 24 24"
            width="18"
            height="18"
            aria-hidden="true"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M21.44 11.05l-9.19 9.19a5 5 0 0 1-7.07-7.07l9.19-9.19a3.5 3.5 0 0 1 4.95 4.95l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48" />
          </svg>
        </button>
        <textarea
          ref={taRef}
          value={text}
          onChange={(e) => {
            setText(e.target.value)
            updateCompletion(e.target.value, e.target.selectionStart ?? e.target.value.length)
          }}
          onKeyUp={(e) => {
            if (!completion) {
              updateCompletion(e.currentTarget.value, e.currentTarget.selectionStart ?? 0)
            }
          }}
          onClick={() => {
            if (completion) setCompletion(null)
          }}
          onKeyDown={onKeyDown}
          onBlur={() => {
            // Defer so click-on-suggestion still fires before we clear.
            window.setTimeout(() => setCompletion(null), 150)
          }}
          placeholder="输入消息，Enter 发送，Shift+Enter 换行；@ 触发 Skill 补全"
          rows={1}
          disabled={disabled}
          aria-label="消息输入"
        />
        <button
          type="button"
          className="btn primary composer-send"
          onClick={submit}
          disabled={!canSend}
        >
          发送
        </button>
      </div>
      {completion && completion.matches.length > 0 && (
        <ul className="composer-complete" role="listbox" aria-label="Skill 补全">
          {completion.matches.map((s, i) => (
            <li
              key={s.id}
              role="option"
              aria-selected={i === completion.activeIndex}
              className={i === completion.activeIndex ? 'composer-complete-item active' : 'composer-complete-item'}
              onMouseDown={(e) => {
                e.preventDefault()
                applyCompletion(s)
              }}
              onMouseEnter={() =>
                setCompletion((c) => (c ? { ...c, activeIndex: i } : c))
              }
            >
              <span className="composer-complete-id">{s.id}</span>
              {s.description ? (
                <span className="composer-complete-desc">{s.description}</span>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      {skillsById.size > 0 && (
        <span className="composer-hint" aria-hidden="true">
          可用 Skill：{Array.from(skillsById.keys()).slice(0, 6).join('、')}
          {skillsById.size > 6 ? '…' : ''}
        </span>
      )}
    </div>
  )
}
