import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, X } from 'lucide-react'
import { useGate, type GateRole } from '../gateContext'
import { useLocale } from '../locale/LocaleContext'
import {
  featureEntries,
  navigationForEntry,
  searchFeatures,
  type FeatureEntry,
} from '../featureCatalog'
import { FEATURE_SEARCH } from '../strings'

type Ctx = { open: () => void; setOverride: (fn: (() => void) | null) => void }
const FeatureSearchContext = createContext<Ctx | null>(null)

export function useFeatureSearch(): Ctx {
  const v = useContext(FeatureSearchContext)
  if (!v) throw new Error('useFeatureSearch must be used within FeatureSearchProvider')
  return v
}

function isEditable(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  return (
    el.tagName === 'INPUT' ||
    el.tagName === 'TEXTAREA' ||
    el.tagName === 'SELECT' ||
    el.isContentEditable
  )
}

function dialogOpen(): boolean {
  return Boolean(document.querySelector('[role="dialog"]'))
}

export function FeatureSearchProvider({ children }: { children: ReactNode }) {
  const { role } = useGate()
  const navigate = useNavigate()
  const { strings, locale } = useLocale()
  const [open, setOpen] = useState(false)
  const [hint, setHint] = useState<FeatureEntry | null>(null)
  const restoreRef = useRef<HTMLElement | null>(null)
  // Chat 页注册自己的统一搜索；设置页没有覆盖时仍打开功能面板。
  const overrideRef = useRef<(() => void) | null>(null)

  const openPalette = useCallback(() => {
    restoreRef.current = document.activeElement as HTMLElement | null
    setOpen(true)
  }, [])

  const setOverride = useCallback((fn: (() => void) | null) => {
    overrideRef.current = fn
  }, [])

  const closePalette = useCallback((restore = true) => {
    setOpen(false)
    if (restore) {
      requestAnimationFrame(() => restoreRef.current?.focus())
    }
  }, [])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        if (!dialogOpen()) (overrideRef.current ?? openPalette)()
        return
      }
      if (e.key === '/' && !isEditable(e.target) && !dialogOpen()) {
        e.preventDefault()
        ;(overrideRef.current ?? openPalette)()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [openPalette])

  useEffect(() => {
    if (!hint) return
    const id = window.setTimeout(() => setHint(null), 5000)
    return () => window.clearTimeout(id)
  }, [hint])

  const value = useMemo(() => ({ open: openPalette, setOverride }), [openPalette, setOverride])

  return (
    <FeatureSearchContext.Provider value={value}>
      {children}
      {open ? (
        <FeatureSearchPalette
          role={role}
          locale={locale}
          onClose={closePalette}
          onSelect={(entry) => {
            setOpen(false)
            const nav = navigationForEntry(entry)
            if (nav.to) navigate(nav.to)
            if (nav.showHint) setHint(entry)
          }}
        />
      ) : null}
      {hint ? (
        <div className="feature-search-hint" role="status">
          <div>
            <strong>{hint.title}</strong>
            <span>{hint.location}</span>
          </div>
          <button
            type="button"
            aria-label={strings.FEATURE_SEARCH.hintClose}
            onClick={() => setHint(null)}
          >
            <X size={14} />
          </button>
        </div>
      ) : null}
    </FeatureSearchContext.Provider>
  )
}

export function FeatureSearchButton({ className = '' }: { className?: string }) {
  const ctx = useContext(FeatureSearchContext)
  const { strings } = useLocale()
  const open = ctx?.open
  return (
    <button
      type="button"
      data-testid="feature-search-trigger"
      className={`btn ghost feature-search-trigger ${className}`}
      onClick={open}
      disabled={!open}
    >
      <Search size={15} aria-hidden />
      <span>{strings.FEATURE_SEARCH.button}</span>
      <kbd>{strings.FEATURE_SEARCH.shortcut}</kbd>
    </button>
  )
}

function FeatureSearchPalette({
  role,
  locale,
  onClose,
  onSelect,
}: {
  role: GateRole
  locale: string
  onClose: (restore?: boolean) => void
  onSelect: (entry: FeatureEntry) => void
}) {
  const { strings } = useLocale()
  const copy = strings.FEATURE_SEARCH
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listId = useId()

  const entries = useMemo(() => featureEntries(role), [role, locale])
  const results = useMemo(() => searchFeatures(entries, query), [entries, query])
  const pages = results.filter((e) => e.kind === 'page')
  const actions = results.filter((e) => e.kind === 'action')
  const ordered = [...pages, ...actions]

  useEffect(() => {
    inputRef.current?.focus()
  }, [])
  useEffect(() => {
    setActive(0)
  }, [query])
  useEffect(() => {
    const el = document.getElementById(`feature-option-${active}`)
    if (el && typeof el.scrollIntoView === 'function') {
      el.scrollIntoView({ block: 'nearest' })
    }
  }, [active])

  const onKey = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    const max = ordered.length - 1
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((i) => Math.min(i + 1, max))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const entry = ordered[Math.min(active, max)]
      if (entry) onSelect(entry)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    }
  }

  return (
    <div
      className="feature-search-overlay"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="feature-search-panel" role="dialog" aria-modal="true" aria-label={copy.button}>
        <div className="feature-search-input-row">
          <Search size={16} aria-hidden />
          <input
            ref={inputRef}
            data-testid="feature-search-input"
            value={query}
            placeholder={copy.placeholder}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKey}
            role="combobox"
            aria-expanded="true"
            aria-controls={listId}
            aria-activedescendant={ordered[active] ? `feature-option-${active}` : undefined}
          />
          <kbd>{copy.esc}</kbd>
        </div>
        <div className="feature-search-results" id={listId} role="listbox">
          {ordered.length === 0 ? (
            <div className="feature-search-empty">
              <p>{copy.empty}</p>
              <p>{copy.emptyHint}</p>
            </div>
          ) : null}
          {pages.length > 0 ? <p className="feature-search-group">{copy.pageGroup}</p> : null}
          {pages.map((entry, i) => (
            <FeatureSearchRow
              key={entry.id}
              entry={entry}
              active={active === i}
              index={i}
              copy={copy}
              onHover={setActive}
              onPick={onSelect}
            />
          ))}
          {actions.length > 0 ? <p className="feature-search-group">{copy.actionGroup}</p> : null}
          {actions.map((entry, i) => {
            const index = pages.length + i
            return (
              <FeatureSearchRow
                key={entry.id}
                entry={entry}
                active={active === index}
                index={index}
                copy={copy}
                onHover={setActive}
                onPick={onSelect}
              />
            )
          })}
        </div>
        <div className="feature-search-footer">{copy.footer}</div>
      </div>
    </div>
  )
}

function FeatureSearchRow({
  entry,
  active,
  index,
  copy,
  onHover,
  onPick,
}: {
  entry: FeatureEntry
  active: boolean
  index: number
  copy: typeof FEATURE_SEARCH
  onHover: (i: number) => void
  onPick: (e: FeatureEntry) => void
}) {
  const Icon = entry.icon
  return (
    <button
      type="button"
      id={`feature-option-${index}`}
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
        {entry.kind === 'page' ? copy.pageBadge : copy.actionBadge}
      </span>
    </button>
  )
}
