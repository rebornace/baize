import { useEffect, useMemo, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import { activeMention, replaceMention } from '../skillMention'
import { filterLoginEntries } from '../loginEntry'
import { LOGIN_AT } from '../strings'
import type { LoginEntry, SkillSummary } from '../api'
import { LoginParamsModal } from './LoginParamsModal'

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
  /** Skills available for @-completion. Omit to disable the popup. */
  skills?: SkillSummary[]
  /** Login catalog entries mixed into the @/ popup (above skills). */
  loginEntries?: LoginEntry[]
  /** Fired when a login entry is chosen (after optional params modal). */
  onPickLogin?: (entry: LoginEntry, args?: Record<string, unknown>) => void | Promise<void>
  /** Optional leading slot inside composer-box (before the attach button). */
  toolbar?: ReactNode
}

interface Completion {
  start: number
  end: number
  query: string
  loginMatches: LoginEntry[]
  skillMatches: SkillSummary[]
  /** Flat index across loginMatches then skillMatches. */
  activeIndex: number
}

function flatCount(c: Completion): number {
  return c.loginMatches.length + c.skillMatches.length
}

export function Composer({
  disabled,
  onSend,
  draft,
  skills,
  loginEntries,
  onPickLogin,
  toolbar,
}: ComposerProps) {
  const [text, setText] = useState('')
  const [files, setFiles] = useState<File[]>([])
  const [completion, setCompletion] = useState<Completion | null>(null)
  const [paramsEntry, setParamsEntry] = useState<LoginEntry | null>(null)
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

  const clearMention = (start: number, end: number) => {
    const before = text.slice(0, start)
    const after = text.slice(end)
    const next = before + after
    setText(next)
    requestAnimationFrame(() => {
      const el = taRef.current
      if (!el) return
      el.focus()
      el.setSelectionRange(before.length, before.length)
    })
  }

  const updateCompletion = (value: string, caret: number) => {
    const skillList = skills ?? []
    const loginList = loginEntries ?? []
    if (skillList.length === 0 && loginList.length === 0) {
      setCompletion(null)
      return
    }
    const active = activeMention(value, caret)
    if (!active) {
      setCompletion(null)
      return
    }
    const q = active.query.toLowerCase()
    const skillMatches = skillList.filter((s) => s.id.toLowerCase().startsWith(q))
    const loginMatches = filterLoginEntries(loginList, active.query)
    if (skillMatches.length === 0 && loginMatches.length === 0) {
      setCompletion(null)
      return
    }
    setCompletion({
      start: active.start,
      end: active.end,
      query: active.query,
      loginMatches,
      skillMatches,
      activeIndex: 0,
    })
  }

  const applySkill = (pick: SkillSummary) => {
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

  const applyLogin = (pick: LoginEntry) => {
    if (!completion) return
    clearMention(completion.start, completion.end)
    setCompletion(null)
    if ((pick.required ?? []).length === 0) {
      void onPickLogin?.(pick)
      return
    }
    setParamsEntry(pick)
  }

  const pickActive = () => {
    if (!completion) return
    const nLogin = completion.loginMatches.length
    if (completion.activeIndex < nLogin) {
      const pick = completion.loginMatches[completion.activeIndex]
      if (pick) applyLogin(pick)
      return
    }
    const pick = completion.skillMatches[completion.activeIndex - nLogin]
    if (pick) applySkill(pick)
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
      const total = flatCount(completion)
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

  const showPopup =
    completion != null &&
    (completion.loginMatches.length > 0 || completion.skillMatches.length > 0)

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
        {toolbar != null && <span className="composer-toolbar">{toolbar}</span>}
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
          placeholder="输入消息，Enter 发送，Shift+Enter 换行；输入 @ 或 / 选择技能"
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
        {showPopup && completion && (
          <ul className="composer-complete" role="listbox" aria-label="补全">
            {completion.loginMatches.length > 0 && (
              <li className="composer-complete-section" role="presentation">
                {LOGIN_AT.sectionLogin}
              </li>
            )}
            {completion.loginMatches.map((entry, i) => (
              <li
                key={`login:${entry.id}`}
                role="option"
                aria-selected={i === completion.activeIndex}
                className={
                  i === completion.activeIndex
                    ? 'composer-complete-item active'
                    : 'composer-complete-item'
                }
                onMouseDown={(e) => {
                  e.preventDefault()
                  applyLogin(entry)
                }}
                onMouseEnter={() =>
                  setCompletion((c) => (c ? { ...c, activeIndex: i } : c))
                }
              >
                <span className="composer-complete-id">
                  {entry.title}
                  {entry.logged_in ? (
                    <span className="composer-complete-badge">{LOGIN_AT.loggedInBadge}</span>
                  ) : null}
                </span>
              </li>
            ))}
            {completion.skillMatches.length > 0 && completion.loginMatches.length > 0 && (
              <li className="composer-complete-section" role="presentation">
                {LOGIN_AT.sectionSkills}
              </li>
            )}
            {completion.skillMatches.map((s, i) => {
              const flat = completion.loginMatches.length + i
              return (
                <li
                  key={`skill:${s.id}`}
                  role="option"
                  aria-selected={flat === completion.activeIndex}
                  className={
                    flat === completion.activeIndex
                      ? 'composer-complete-item active'
                      : 'composer-complete-item'
                  }
                  onMouseDown={(e) => {
                    e.preventDefault()
                    applySkill(s)
                  }}
                  onMouseEnter={() =>
                    setCompletion((c) => (c ? { ...c, activeIndex: flat } : c))
                  }
                >
                  <span className="composer-complete-id">{s.id}</span>
                  {s.description ? (
                    <span className="composer-complete-desc">{s.description}</span>
                  ) : null}
                </li>
              )
            })}
          </ul>
        )}
      </div>
      {skillsById.size > 0 && (
        <span className="composer-hint" aria-hidden="true">
          可用 Skill：{Array.from(skillsById.keys()).slice(0, 6).join('、')}
          {skillsById.size > 6 ? '…' : ''}
        </span>
      )}
      <LoginParamsModal
        open={paramsEntry != null}
        entry={paramsEntry}
        onCancel={() => setParamsEntry(null)}
        onSubmit={async (args) => {
          const picked = paramsEntry
          if (!picked) return
          await onPickLogin?.(picked, args)
          setParamsEntry(null)
        }}
      />
    </div>
  )
}
