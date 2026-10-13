import { useEffect, useId, useRef, useState } from 'react'
import type { GoalStackView } from '../../api/goal'
import { GOAL } from '../../strings'
import type { GoalCandidate } from './goalCandidates'

export type GoalSavePayload = {
  text: string
  details: string
  /** root = replace main goal; tip = patch current focus */
  scope: 'root' | 'tip'
}

export type GoalBarProps = {
  view: GoalStackView | null
  candidates?: GoalCandidate[]
  loadError?: boolean
  writeDisabled: boolean
  onRetry?: () => void
  onSave: (payload: GoalSavePayload) => void
  onPush: () => void
  onPop: () => void
  onStop: () => void
  onResume: () => void
  onPickMain: (text: string) => void
  onPickSide: (text: string) => void
}

type PickerMode = 'main' | 'side'
type PanelMode = 'view' | 'edit'

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
  onSave,
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
  const [panel, setPanel] = useState<PanelMode | null>(null)
  const [draftText, setDraftText] = useState('')
  const [draftDetails, setDraftDetails] = useState('')
  const panelRef = useRef<HTMLDivElement>(null)
  const menuId = useId()
  const listId = useId()
  const detailId = useId()

  useEffect(() => {
    if (!menuOpen && !picker && !panel) return
    const onDoc = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setMenuOpen(false)
        setPicker(null)
        if (panel !== 'edit') setPanel(null)
      }
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setPicker(null)
        setMenuOpen(false)
        if (panel === 'edit') setPanel('view')
        else setPanel(null)
      }
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [menuOpen, picker, panel])

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
  const editScope: 'root' | 'tip' = depth <= 1 ? 'root' : 'tip'
  const focusFrame = pausedFocus ?? tip ?? root
  const focusText = focusFrame?.text ?? ''
  const focusDetails = focusFrame?.details?.trim() ?? ''
  const hasDetails = Boolean(focusDetails)
  const rootPaused = Boolean(root && root.status === 'interrupted' && !tip)

  const openMainPicker = () => {
    setMenuOpen(false)
    setPanel(null)
    setPicker('main')
  }
  const openSidePicker = () => {
    setMenuOpen(false)
    setPanel(null)
    if (canPick) setPicker('side')
    else onPush()
  }
  const pick = (text: string) => {
    if (picker === 'side') onPickSide(text)
    else onPickMain(text)
    setPicker(null)
  }

  const openView = () => {
    setMenuOpen(false)
    setPicker(null)
    setPanel((p) => (p ? null : 'view'))
  }

  const startEdit = () => {
    setMenuOpen(false)
    setPicker(null)
    setDraftText(focusText)
    setDraftDetails(focusDetails)
    setPanel('edit')
  }

  const saveEdit = () => {
    const text = draftText.trim()
    if (!text || writeDisabled) return
    onSave({ text, details: draftDetails.trim(), scope: editScope })
    setPanel('view')
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
      {stacked && focusText && (
        <div className="goal-bar-row">
          <button
            type="button"
            className="goal-bar-summary"
            data-testid="goal-bar-summary"
            aria-expanded={panel !== null}
            aria-controls={detailId}
            title={GOAL.openDetails}
            onClick={openView}
          >
            <span className="goal-bar-mark">{GOAL.mark}</span>
            {(pausedFocus || !sameFocus) && root && (
              <span className="goal-bar-crumb">{root.text}</span>
            )}
            <span className="goal-bar-text">{pausedFocus ? pausedFocus.text : focusText}</span>
            {hasDetails && <span className="goal-bar-has-details">{GOAL.hasDetails}</span>}
            {(rootPaused || pausedFocus) && (
              <span className="goal-bar-paused">{GOAL.pausedHint}</span>
            )}
          </button>
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
              setPanel(null)
              setMenuOpen((v) => !v)
            }}
          >
            {GOAL.moreActions}
          </button>
        </div>
      )}
      {panel && stacked && (
        <div className="goal-bar-detail" id={detailId} data-testid="goal-bar-detail">
          {panel === 'view' && (
            <>
              {!sameFocus && root && (
                <p className="goal-bar-detail-line">
                  <span className="goal-bar-detail-label">{GOAL.rootLabel}</span>
                  <span>{root.text}</span>
                </p>
              )}
              <p className="goal-bar-detail-line">
                <span className="goal-bar-detail-label">
                  {sameFocus && !pausedFocus ? GOAL.currentLabel : GOAL.focusLabel}
                </span>
                <span>{focusText}</span>
              </p>
              {hasDetails ? (
                <p className="goal-bar-detail-body" data-testid="goal-bar-details">
                  <span className="goal-bar-detail-label">{GOAL.detailsLabel}</span>
                  <span className="goal-bar-detail-text">{focusDetails}</span>
                </p>
              ) : (
                <p className="goal-bar-detail-empty">{GOAL.noDetails}</p>
              )}
              <div className="goal-bar-detail-actions">
                <button
                  type="button"
                  className="btn sm"
                  disabled={writeDisabled}
                  onClick={startEdit}
                >
                  {GOAL.editRoot}
                </button>
                <button type="button" className="btn sm ghost" onClick={() => setPanel(null)}>
                  {GOAL.closeDetails}
                </button>
              </div>
            </>
          )}
          {panel === 'edit' && (
            <form
              className="goal-bar-edit"
              data-testid="goal-bar-edit"
              onSubmit={(e) => {
                e.preventDefault()
                saveEdit()
              }}
            >
              <label className="goal-bar-edit-field">
                <span>{GOAL.fieldTitle}</span>
                <input
                  value={draftText}
                  disabled={writeDisabled}
                  onChange={(e) => setDraftText(e.target.value)}
                  autoFocus
                />
              </label>
              <label className="goal-bar-edit-field">
                <span>{GOAL.fieldDetails}</span>
                <textarea
                  value={draftDetails}
                  disabled={writeDisabled}
                  rows={3}
                  onChange={(e) => setDraftDetails(e.target.value)}
                  placeholder={GOAL.detailsPlaceholder}
                />
              </label>
              <div className="goal-bar-detail-actions">
                <button
                  type="submit"
                  className="btn sm primary"
                  disabled={writeDisabled || !draftText.trim()}
                >
                  {GOAL.save}
                </button>
                <button
                  type="button"
                  className="btn sm ghost"
                  disabled={writeDisabled}
                  onClick={() => setPanel('view')}
                >
                  {GOAL.cancelEdit}
                </button>
              </div>
            </form>
          )}
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
            disabled={writeDisabled}
            onClick={startEdit}
          >
            {GOAL.editRoot}
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
