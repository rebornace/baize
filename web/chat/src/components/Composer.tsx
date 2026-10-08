import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import type { SkillSummary } from '../api'
import type { Locale } from '../locale/types'
import { skillDisplayName, skillLocalizedDescription } from '../pages/skills/skillDisplay'
import {
  activeMention,
  matchesReloadQuery,
  RELOAD_TOKEN,
  replaceMention,
} from '../skillMention'
import { CHAT } from '../strings'

function composerLocale(): Locale {
  if (typeof document !== 'undefined' && document.documentElement.lang === 'en') {
    return 'en'
  }
  return 'zh-CN'
}

const ACCEPT =
  '.txt,.md,.csv,.docx,.xlsx,.pdf,.png,.jpg,.jpeg,.webp,.gif'

export interface ComposerProps {
  disabled?: boolean
  /**
   * 返回 false（同步或 Promise 解析为 false）表示消息被拒收（如模型/图片
   * 门控未过），此时保留文字与附件；返回 true/undefined 视为已接受并清空。
   */
  onSend: (text: string, files: File[]) => void | boolean | Promise<void | boolean>
  draft?: string
  /**
   * Skills available for @-completion. Omit to disable the popup entirely.
   * Pass [] to still surface the reserved `/reload` command.
   */
  skills?: SkillSummary[]
  /** Optional leading slot inside composer-box (before the attach button). */
  toolbar?: ReactNode
}

type CompleteItem =
  | { kind: 'reload'; id: typeof RELOAD_TOKEN }
  | { kind: 'skill'; id: string; skill: SkillSummary }

interface Completion {
  start: number
  end: number
  trigger: string
  query: string
  items: CompleteItem[]
  activeIndex: number
}

export function Composer({
  disabled,
  onSend,
  draft,
  skills,
  toolbar,
}: ComposerProps) {
  const [text, setText] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [completion, setCompletion] = useState<Completion | null>(null)
  const locale = composerLocale()
  const taRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (draft !== undefined) {
      setText(draft)
      // Spec §7.2: after「去登录」writes @login-<id>, focus the composer.
      requestAnimationFrame(() => {
        taRef.current?.focus()
      })
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
    // undefined = completion disabled; [] still shows /reload.
    if (skills === undefined) {
      setCompletion(null)
      return
    }
    const active = activeMention(value, caret)
    if (!active) {
      setCompletion(null)
      return
    }
    const q = active.query.toLowerCase()
    const items: CompleteItem[] = []
    if (matchesReloadQuery(active.query)) {
      items.push({ kind: 'reload', id: RELOAD_TOKEN })
    }
    for (const s of skills) {
      if (s.id === RELOAD_TOKEN) continue
      if (s.id.toLowerCase().startsWith(q)) {
        items.push({ kind: 'skill', id: s.id, skill: s })
      }
    }
    if (items.length === 0) {
      setCompletion(null)
      return
    }
    setCompletion({
      start: active.start,
      end: active.end,
      trigger: active.trigger,
      query: active.query,
      items,
      activeIndex: 0,
    })
  }

  const applyItem = (pick: CompleteItem) => {
    if (!completion) return
    const { text: next, caret } = replaceMention(
      text,
      completion.start,
      completion.end,
      pick.id,
      completion.trigger,
    )
    setText(next)
    setCompletion(null)
    requestAnimationFrame(() => {
      const el = taRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(caret, caret)
    })
  }

  const pickActive = () => {
    if (!completion) return
    const pick = completion.items[completion.activeIndex]
    if (pick) applyItem(pick)
  }

  const submit = async () => {
    const trimmed = text.trim()
    if ((!trimmed && files.length === 0) || disabled) return
    const result = await onSend(trimmed, files)
    // false = rejected by the page's gates; keep draft text and attachments.
    if (result !== false) {
      setText('')
      setFiles([])
      setCompletion(null)
    }
  }

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (completion) {
      const total = completion.items.length
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setCompletion((c) =>
          c && total > 0 ? { ...c, activeIndex: (c.activeIndex + 1) % total } : c,
        )
        return
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault()
        setCompletion((c) =>
          c && total > 0
            ? { ...c, activeIndex: (c.activeIndex - 1 + total) % total }
            : c,
        )
        return
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault()
        pickActive()
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

  const showPopup = completion != null && completion.items.length > 0
  const showHint = skills !== undefined

  return (
    <div className="composer">
      {files.length > 0 && (
        <div className="composer-chips" aria-label={CHAT.attachmentsAria}>
          {files.map((f, i) => (
            <span key={`${f.name}-${i}`} className="composer-chip">
              <span className="composer-chip-name" title={f.name}>{f.name}</span>
              <button
                type="button"
                className="composer-chip-remove"
                aria-label={CHAT.removeAttachmentAria(f.name)}
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
        {toolbar != null && <span className="composer-toolbar">{toolbar}</span>}
        <button
          type="button"
          className="composer-attach"
          aria-label={CHAT.addAttachment}
          title={CHAT.addAttachment}
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
          placeholder={CHAT.composerPlaceholder}
          title={CHAT.composerTitle}
          rows={1}
          disabled={disabled}
          aria-label={CHAT.messageInputAria}
        />
        <button
          type="button"
          className="btn primary composer-send"
          onClick={submit}
          disabled={!canSend}
        >
          {CHAT.send}
        </button>
        {showPopup && completion && (
          <ul className="composer-complete" role="listbox" aria-label={CHAT.skillCompleteAria}>
            {completion.items.map((item, i) => {
              const desc =
                item.kind === 'reload'
                  ? CHAT.reloadCompleteDesc
                  : skillLocalizedDescription(item.skill, locale) ||
                    skillDisplayName(item.skill, locale)
              const showDesc = Boolean(desc && desc !== item.id)
              return (
                <li
                  key={`${item.kind}:${item.id}`}
                  role="option"
                  aria-selected={i === completion.activeIndex}
                  className={
                    i === completion.activeIndex
                      ? 'composer-complete-item active'
                      : 'composer-complete-item'
                  }
                  onMouseDown={(e) => {
                    e.preventDefault()
                    applyItem(item)
                  }}
                  onMouseEnter={() =>
                    setCompletion((c) => (c ? { ...c, activeIndex: i } : c))
                  }
                >
                  <span className="composer-complete-id">
                    {completion.trigger}
                    {item.id}
                  </span>
                  {showDesc ? (
                    <span className="composer-complete-desc">{desc}</span>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
      </div>
      {showHint && (
        <span className="composer-hint" aria-hidden="true">
          {skillsById.size > 0 && (
            <>
              {CHAT.availableSkills}
              {Array.from(skillsById.keys()).slice(0, 6).join(CHAT.skillListSep)}
              {skillsById.size > 6 ? '…' : ''}
              {CHAT.skillListSep}
            </>
          )}
          {CHAT.reloadHint}
        </span>
      )}
    </div>
  )
}
