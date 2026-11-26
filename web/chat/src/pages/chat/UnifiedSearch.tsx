import { useEffect, useId, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent as ReactKeyboardEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search } from 'lucide-react'
import { searchConversationMessages, type MessageSearchHit } from '../../api'
import {
  featureEntries,
  searchFeatures,
  type FeatureEntry,
} from '../../featureCatalog'
import { useGate } from '../../gateContext'
import { useLocale } from '../../locale/LocaleContext'
import { CHAT, FEATURE_SEARCH, SEARCH } from '../../strings'

type Tab = 'messages' | 'features'
type RoleFilter = '' | 'user' | 'assistant' | 'system_note'
type TimeFilter = '' | '7' | '30' | '90'

export type UnifiedSearchProps = {
  open: boolean
  onClose: () => void
  conversationId: string
  onOpenConversation: (conversationId: string, messageId: string) => void
  /** Test hook; production uses 250ms. */
  debounceMs?: number
}

function roleLabel(role: string): string {
  if (role === 'user') return CHAT.messageSearchRoleUser
  if (role === 'system_note') return CHAT.messageSearchRoleSystem
  return CHAT.messageSearchRoleAssistant
}

/**
 * One entry for both message search and feature/settings navigation.
 * Chat 页用它替代原来的两个搜索框；设置页仍用 FeatureSearchButton。
 */
export function UnifiedSearch({
  open,
  onClose,
  conversationId,
  onOpenConversation,
  debounceMs = 250,
}: UnifiedSearchProps) {
  const { role } = useGate()
  const { locale } = useLocale()
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('messages')
  const [query, setQuery] = useState('')
  const [roleFilter, setRoleFilter] = useState<RoleFilter>('')
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('')
  const [onlyCurrent, setOnlyCurrent] = useState(false)
  const [hits, setHits] = useState<MessageSearchHit[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)
  const [active, setActive] = useState(0)
  const seq = useRef(0)
  const listId = useId()

  const entries = useMemo(() => featureEntries(role), [role, locale])
  const featureResults = useMemo(() => searchFeatures(entries, query), [entries, query])
  const ordered = useMemo(
    () => [
      ...featureResults.filter((e) => e.kind === 'page'),
      ...featureResults.filter((e) => e.kind === 'action'),
    ],
    [featureResults],
  )

  useEffect(() => {
    if (!open) return
    setTab('messages')
    setQuery('')
    setRoleFilter('')
    setTimeFilter('')
    setOnlyCurrent(false)
    setHits([])
    setLoading(false)
    setError(false)
    setActive(0)
  }, [open])

  useEffect(() => {
    setActive(0)
  }, [query, tab])

  useEffect(() => {
    const id = ++seq.current
    const q = query.trim()
    if (!open || tab !== 'messages' || q === '') {
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
          role: roleFilter || undefined,
          days: timeFilter ? Number(timeFilter) : undefined,
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
  }, [query, roleFilter, timeFilter, onlyCurrent, conversationId, open, tab, debounceMs])

  if (!open) return null

  const pickMessage = (hit: MessageSearchHit) => {
    onOpenConversation(hit.conversation_id, hit.message_id)
    onClose()
  }

  const pickFeature = (entry: FeatureEntry) => {
    if (entry.to) navigate(entry.to)
    onClose()
  }

  const pickActive = () => {
    if (tab === 'messages') {
      const hit = hits[Math.min(active, hits.length - 1)]
      if (hit) pickMessage(hit)
      return
    }
    const entry = ordered[Math.min(active, ordered.length - 1)]
    if (entry) pickFeature(entry)
  }

  const count = tab === 'messages' ? hits.length : ordered.length
  const onKeyDown = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((i) => Math.min(i + 1, Math.max(count - 1, 0)))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      pickActive()
    }
  }

  const pages = ordered.filter((e) => e.kind === 'page')
  const actions = ordered.filter((e) => e.kind === 'action')

  return (
    <div
      className="feature-search-overlay"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div
        className="feature-search-panel"
        role="dialog"
        aria-modal="true"
        aria-label={SEARCH.entry}
      >
        <div className="feature-search-input-row">
          <Search size={16} aria-hidden />
          <input
            data-testid="unified-search-input"
            autoFocus
            value={query}
            placeholder={SEARCH.placeholder}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            role="combobox"
            aria-expanded="true"
            aria-controls={listId}
            aria-activedescendant={count > 0 ? `unified-option-${active}` : undefined}
          />
          <kbd>{FEATURE_SEARCH.esc}</kbd>
        </div>

        <div className="unified-search-tabs" role="tablist" aria-label={SEARCH.entry}>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'messages'}
            className={`unified-search-tab${tab === 'messages' ? ' active' : ''}`}
            onClick={() => setTab('messages')}
          >
            {SEARCH.messagesTab}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === 'features'}
            className={`unified-search-tab${tab === 'features' ? ' active' : ''}`}
            onClick={() => setTab('features')}
          >
            {SEARCH.featuresTab}
          </button>
        </div>

        {tab === 'messages' ? (
          <>
            <div className="message-search-filters unified-search-filters">
              <select
                aria-label={CHAT.messageSearchFilterRole}
                value={roleFilter}
                onChange={(e) => setRoleFilter(e.target.value as RoleFilter)}
              >
                <option value="">{CHAT.messageSearchRoleAll}</option>
                <option value="user">{CHAT.messageSearchRoleUser}</option>
                <option value="assistant">{CHAT.messageSearchRoleAssistant}</option>
                <option value="system_note">{CHAT.messageSearchRoleSystem}</option>
              </select>
              <select
                aria-label={CHAT.messageSearchFilterTime}
                value={timeFilter}
                onChange={(e) => setTimeFilter(e.target.value as TimeFilter)}
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
            <div className="message-search-results" id={listId} role="listbox">
              {query.trim() === '' && (
                <p className="message-search-hint">{CHAT.messageSearchHint}</p>
              )}
              {loading && <p className="message-search-hint">{CHAT.messageSearchLoading}</p>}
              {!loading && error && (
                <p className="message-search-hint" role="alert">
                  {CHAT.messageSearchError}
                </p>
              )}
              {!loading && !error && query.trim() !== '' && hits.length === 0 && (
                <p className="message-search-hint">{CHAT.messageSearchEmpty}</p>
              )}
              {hits.map((hit, i) => (
                <button
                  key={hit.message_id}
                  type="button"
                  id={`unified-option-${i}`}
                  role="option"
                  aria-selected={active === i}
                  className={`message-search-hit${active === i ? ' active' : ''}`}
                  data-message-id={hit.message_id}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => pickMessage(hit)}
                >
                  <span className="message-search-hit-title">
                    {hit.conversation_title || hit.conversation_id}
                  </span>
                  <span className="message-search-hit-snippet">{hit.snippet}</span>
                  <span className="message-search-hit-meta">{roleLabel(hit.role)}</span>
                </button>
              ))}
            </div>
          </>
        ) : (
          <div className="feature-search-results" id={listId} role="listbox">
            {ordered.length === 0 ? (
              <div className="feature-search-empty">
                <p>{FEATURE_SEARCH.empty}</p>
                <p>{FEATURE_SEARCH.emptyHint}</p>
              </div>
            ) : null}
            {pages.length > 0 ? (
              <p className="feature-search-group">{FEATURE_SEARCH.pageGroup}</p>
            ) : null}
            {pages.map((entry, i) => (
              <FeatureRow
                key={entry.id}
                entry={entry}
                index={i}
                active={active === i}
                onHover={setActive}
                onPick={pickFeature}
              />
            ))}
            {actions.length > 0 ? (
              <p className="feature-search-group">{FEATURE_SEARCH.actionGroup}</p>
            ) : null}
            {actions.map((entry, i) => {
              const index = pages.length + i
              return (
                <FeatureRow
                  key={entry.id}
                  entry={entry}
                  index={index}
                  active={active === index}
                  onHover={setActive}
                  onPick={pickFeature}
                />
              )
            })}
          </div>
        )}

        <div className="feature-search-footer">{FEATURE_SEARCH.footer}</div>
      </div>
    </div>
  )
}

function FeatureRow({
  entry,
  index,
  active,
  onHover,
  onPick,
}: {
  entry: FeatureEntry
  index: number
  active: boolean
  onHover: (i: number) => void
  onPick: (e: FeatureEntry) => void
}) {
  const Icon = entry.icon
  return (
    <button
      type="button"
      id={`unified-option-${index}`}
      role="option"
      aria-selected={active}
      className={`feature-search-row${active ? ' active' : ''}`}
      onMouseEnter={() => onHover(index)}
      onClick={() => onPick(entry)}
    >
      <Icon size={16} aria-hidden />
      <span className="feature-search-copy">
        <span className="feature-search-title">{entry.title}</span>
        <span className="feature-search-desc">{entry.description}</span>
        {entry.location ? <span className="feature-search-location">{entry.location}</span> : null}
      </span>
      <span className="feature-search-badge">
        {entry.kind === 'page' ? FEATURE_SEARCH.pageBadge : FEATURE_SEARCH.actionBadge}
      </span>
    </button>
  )
}
