import { useEffect, useId, useRef, useState } from 'react'
import type { GoalStackView } from '../../api/goal'
import { GOAL } from '../../strings'
import type { GoalCandidate } from './goalCandidates'

export type GoalBarProps = {
  view: GoalStackView | null
  candidates?: GoalCandidate[]
  loadError?: boolean
  writeDisabled: boolean
  onRetry?: () => void
  onEdit: () => void
  onPush: () => void
  onPop: () => void
  onStop: () => void
  onResume: () => void
  onPickMain: (text: string) => void
  onPickSide: (text: string) => void
}

type PickerMode = 'main' | 'side'

function hasStack(view: GoalStackView | null): view is GoalStackView {
  return Boolean(view && view.frames.length > 0)
}

function showResume(view: GoalStackView): boolean {
  return view.frames.some((f) => f.status === 'interrupted')
}

function interruptedTipFrame(view: GoalStackView) {
  for (let i = view.frames.length - 1; i >= 0; i--) {
    const f = view.frames[i]
    if (f.status === 'interrupted') return f
  }
  return undefined
}

export function GoalBar({
  view,
  candidates = [],
  loadError = false,
  writeDisabled,
  onRetry,
  onEdit,
  onPush,
  onPop,
  onStop,
  onResume,
  onPickMain,
  onPickSide,
}: GoalBarProps) {
  const stacked = hasStack(view)
  const canPick = candidates.length > 0
  const [menuOpen, setMenuOpen] = useState(false)
  const [picker, setPicker] = useState<PickerMode | null>(null)
  const panelRef = useRef<HTMLDivElement>(null)
  const menuId = useId()
  const listId = useId()

  useEffect(() => {
    if (!menuOpen && !picker) return
    const onDoc = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
        setPicker(null)
      }
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setPicker(null)
        setMenuOpen(false)
      }
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [menuOpen, picker])

  // Empty stack: no chrome (agent seeds / tools). Error-only still shows.
  if (!stacked && !loadError) return null

  const root = stacked ? view.root : undefined
  const tip = stacked ? view.tip : undefined
  const depth = stacked ? view.frames.length : 0
  const sameFocus = Boolean(root && tip && root.id === tip.id)
  const pausedFocus = stacked && sameFocus ? interruptedTipFrame(view) : undefined
  const canPop = depth > 1
  const canStop = tip?.status === 'active'
  const canResume = stacked ? showResume(view) : false
  const editLabel = depth <= 1 ? GOAL.editRoot : GOAL.editFocus
  const canEdit = depth <= 1 ? Boolean(root) : Boolean(tip)

  const primary =
    pausedFocus?.text ?? tip?.text ?? root?.text ?? ''
  const rootPaused = Boolean(root && root.status === 'interrupted' && !tip)
  const detailSrc = pausedFocus ?? tip ?? root
  const titleParts = [
    root && (!sameFocus || pausedFocus) ? `${GOAL.rootLabel}: ${root.text}` : '',
    primary ? `${sameFocus && !pausedFocus ? GOAL.currentLabel : GOAL.focusLabel}: ${primary}` : '',
    detailSrc?.details ? `${GOAL.detailsLabel}: ${detailSrc.details}` : '',
  ].filter(Boolean)
  const titleAttr = titleParts.join('\n')

  const openMainPicker = () => {
    setMenuOpen(false)
    setPicker('main')
  }
  const openSidePicker = () => {
    setMenuOpen(false)
    if (canPick) setPicker('side')
    else onPush()
  }
  const pick = (text: string) => {
    if (picker === 'side') onPickSide(text)
    else onPickMain(text)
    setPicker(null)
  }

  return (
    <div className="goal-bar" data-testid="goal-bar" ref={panelRef}>
      {loadError && (
        <p className="goal-bar-error" data-testid="goal-bar-error">
          <span>{GOAL.loadFailed}</span>
          {onRetry && (
            <button type="button" className="btn sm linkish" onClick={onRetry}>
              {GOAL.retry}
            </button>
          )}
        </p>
      )}
      {stacked && primary && (
        <div className="goal-bar-row">
          <p className="goal-bar-summary" title={titleAttr}>
            <span className="goal-bar-mark">{GOAL.mark}</span>
            {(pausedFocus || !sameFocus) && root && (
              <span className="goal-bar-crumb">{root.text}</span>
            )}
            <span className="goal-bar-text">{pausedFocus ? pausedFocus.text : primary}</span>
            {(rootPaused || pausedFocus) && (
              <span className="goal-bar-paused">{GOAL.pausedHint}</span>
            )}
          </p>
          <button
            type="button"
            className="goal-bar-more"
            disabled={writeDisabled}
            aria-expanded={menuOpen}
            aria-controls={menuId}
            aria-label={GOAL.moreActions}
            title={GOAL.moreActions}
            onClick={() => {
              setPicker(null)
              setMenuOpen((v) => !v)
            }}
          >
            {GOAL.moreActions}
          </button>
        </div>
      )}
      {menuOpen && stacked && (
        <div className="goal-bar-menu" id={menuId} role="menu" data-testid="goal-bar-menu">
          {canPick && (
            <button
              type="button"
              role="menuitem"
              className="goal-bar-menu-item"
              disabled={writeDisabled}
              onClick={openMainPicker}
            >
              {GOAL.pickFromChat}
            </button>
          )}
          <button
            type="button"
            role="menuitem"
            className="goal-bar-menu-item"
            disabled={writeDisabled || !canEdit}
            onClick={() => {
              setMenuOpen(false)
              onEdit()
            }}
          >
            {editLabel}
          </button>
          <button
            type="button"
            role="menuitem"
            className="goal-bar-menu-item"
            disabled={writeDisabled}
            onClick={openSidePicker}
          >
            {GOAL.push}
          </button>
          <button
            type="button"
            role="menuitem"
            className="goal-bar-menu-item"
            disabled={writeDisabled || !canPop}
            onClick={() => {
              setMenuOpen(false)
              onPop()
            }}
          >
            {GOAL.pop}
          </button>
          {canStop && (
            <button
              type="button"
              role="menuitem"
              className="goal-bar-menu-item"
              disabled={writeDisabled}
              onClick={() => {
                setMenuOpen(false)
                onStop()
              }}
            >
              {GOAL.stop}
            </button>
          )}
          {canResume && (
            <button
              type="button"
              role="menuitem"
              className="goal-bar-menu-item"
              disabled={writeDisabled}
              onClick={() => {
                setMenuOpen(false)
                onResume()
              }}
            >
              {GOAL.resume}
            </button>
          )}
        </div>
      )}
      {picker && canPick && (
        <div className="goal-bar-picker" role="listbox" id={listId} data-testid="goal-bar-picker">
          <p className="goal-bar-picker-title">
            {picker === 'side' ? GOAL.pickSideTitle : GOAL.pickMainTitle}
          </p>
          <ul className="goal-bar-picker-list">
            {candidates.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  className="goal-bar-picker-item"
                  role="option"
                  disabled={writeDisabled}
                  title={c.text}
                  onClick={() => pick(c.text)}
                >
                  {c.preview}
                </button>
              </li>
            ))}
          </ul>
          {picker === 'side' && (
            <button
              type="button"
              className="btn sm linkish goal-bar-picker-type"
              disabled={writeDisabled}
              onClick={() => {
                setPicker(null)
                onPush()
              }}
            >
              {GOAL.typeYourself}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
