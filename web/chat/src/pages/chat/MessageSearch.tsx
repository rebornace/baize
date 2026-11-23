import { useEffect, useRef, useState } from 'react'
import { searchConversationMessages, type MessageSearchHit } from '../../api'
import { Modal } from '../../components/ui'
import { CHAT } from '../../strings'

export type MessageSearchProps = {
  open: boolean
  onClose: () => void
  onOpenConversation: (conversationId: string, messageId: string) => void
  /** Current conversation, used by the "only this conversation" scope. */
  conversationId: string
  /** Test hook; production uses 250ms. */
  debounceMs?: number
}

type RoleFilter = '' | 'user' | 'assistant' | 'system_note'
type TimeFilter = '' | '7' | '30' | '90'

function roleLabel(role: string): string {
  if (role === 'user') return CHAT.messageSearchRoleUser
  if (role === 'system_note') return CHAT.messageSearchRoleSystem
  return CHAT.messageSearchRoleAssistant
}

/** Cross-conversation message search. Results are ACL-filtered by the API. */
export function MessageSearch({
  open,
  onClose,
  onOpenConversation,
  conversationId,
  debounceMs = 250,
}: MessageSearchProps) {
  const [query, setQuery] = useState('')
  const [role, setRole] = useState<RoleFilter>('')
  const [time, setTime] = useState<TimeFilter>('')
  const [onlyCurrent, setOnlyCurrent] = useState(false)
  const [hits, setHits] = useState<MessageSearchHit[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)
  const seq = useRef(0)

  useEffect(() => {
    if (!open) {
      setQuery('')
      setRole('')
      setTime('')
      setOnlyCurrent(false)
      setHits([])
      setLoading(false)
      setError(false)
    }
  }, [open])

  useEffect(() => {
    const id = ++seq.current
    const q = query.trim()
    if (!open || q === '') {
      setHits([])
      setLoading(false)
      setError(false)
      return
    }
    setLoading(true)
    setError(false)
    const timer = window.setTimeout(async () => {
      try {
        const list = await searchConversationMessages(q, {
          limit: 20,
          role: role || undefined,
          days: time ? Number(time) : undefined,
          conversationId: onlyCurrent ? conversationId : undefined,
        })
        if (seq.current === id) setHits(list)
      } catch {
        if (seq.current === id) setError(true)
      } finally {
        if (seq.current === id) setLoading(false)
      }
    }, debounceMs)
    return () => window.clearTimeout(timer)
  }, [query, role, time, onlyCurrent, conversationId, open, debounceMs])

  return (
    <Modal open={open} title={CHAT.messageSearchTitle} onClose={onClose}>
      <input
        className="message-search-input"
        type="text"
        autoFocus
        aria-label={CHAT.messageSearchTitle}
        placeholder={CHAT.messageSearchPlaceholder}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />
      <div className="message-search-filters">
        <select
          aria-label={CHAT.messageSearchFilterRole}
          value={role}
          onChange={(e) => setRole(e.target.value as RoleFilter)}
        >
          <option value="">{CHAT.messageSearchRoleAll}</option>
          <option value="user">{CHAT.messageSearchRoleUser}</option>
          <option value="assistant">{CHAT.messageSearchRoleAssistant}</option>
          <option value="system_note">{CHAT.messageSearchRoleSystem}</option>
        </select>
        <select
          aria-label={CHAT.messageSearchFilterTime}
          value={time}
          onChange={(e) => setTime(e.target.value as TimeFilter)}
        >
          <option value="">{CHAT.messageSearchTimeAll}</option>
          <option value="7">{CHAT.messageSearchTime7d}</option>
          <option value="30">{CHAT.messageSearchTime30d}</option>
          <option value="90">{CHAT.messageSearchTime90d}</option>
        </select>
        <label className="message-search-scope">
          <input
            type="checkbox"
            checked={onlyCurrent}
            onChange={(e) => setOnlyCurrent(e.target.checked)}
          />
          {CHAT.messageSearchOnlyCurrent}
        </label>
      </div>
      <div className="message-search-results" data-testid="message-search-results">
        {query.trim() === '' && <p className="message-search-hint">{CHAT.messageSearchHint}</p>}
        {loading && <p className="message-search-hint">{CHAT.messageSearchLoading}</p>}
        {!loading && error && (
          <p className="message-search-hint" role="alert">
            {CHAT.messageSearchError}
          </p>
        )}
        {!loading && !error && query.trim() !== '' && hits.length === 0 && (
          <p className="message-search-hint">{CHAT.messageSearchEmpty}</p>
        )}
        {hits.map((hit) => (
          <button
            key={hit.message_id}
            type="button"
            className="message-search-hit"
            data-message-id={hit.message_id}
            onClick={() => onOpenConversation(hit.conversation_id, hit.message_id)}
          >
            <span className="message-search-hit-title">
              {hit.conversation_title || hit.conversation_id}
            </span>
            <span className="message-search-hit-snippet">{hit.snippet}</span>
            <span className="message-search-hit-meta">{roleLabel(hit.role)}</span>
          </button>
        ))}
      </div>
    </Modal>
  )
}
