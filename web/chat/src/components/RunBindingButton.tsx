import { useState } from 'react'
import { getRun, getRunBinding, listRunChildren } from '../api'
import type { Run } from '../api'
import { ApiError } from '../api/http'
import { Button, Modal } from './ui'
import { CHAT, friendlyError } from '../strings'

export type RunBindingButtonProps = {
  runId: string
}

function shortRunId(id: string): string {
  if (id.length <= 10) return id
  return `${id.slice(0, 8)}…`
}

function statusLabel(status: string): string {
  switch (status) {
    case 'queued':
      return CHAT.statusQueued
    case 'running':
      return CHAT.statusRunning
    case 'waiting_human':
      return CHAT.statusWaitingHuman
    case 'succeeded':
      return CHAT.statusSucceeded
    case 'failed':
      return CHAT.statusFailed
    case 'cancelled':
      return CHAT.statusCancelled
    case 'rejected':
      return CHAT.statusRejected
    default:
      return status
  }
}

function modeLabel(mode?: string): string {
  if (mode === 'readonly') return CHAT.bindingModeReadonly
  if (mode === 'readwrite') return CHAT.bindingModeReadwrite
  return ''
}

function childCaption(child: Run): string {
  const group = (child.group_id ?? '').trim()
  const mode = modeLabel(child.spawn_mode)
  const base = CHAT.bindingChildLink(group || shortRunId(child.id), statusLabel(child.status))
  return mode ? `${base} · ${mode}` : base
}

export function RunBindingButton({ runId }: RunBindingButtonProps) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [jsonText, setJsonText] = useState('')
  const [hidden, setHidden] = useState(false)
  const [viewRunId, setViewRunId] = useState(runId)
  const [parentRunId, setParentRunId] = useState<string | null>(null)
  const [children, setChildren] = useState<Run[]>([])
  const [navHint, setNavHint] = useState<string | null>(null)

  const loadBindingFor = async (targetId: string, opts?: { hideOnMissing?: boolean }) => {
    setBusy(true)
    setError(null)
    setNavHint(null)
    try {
      const [bindingBody, runRec] = await Promise.all([
        getRunBinding(targetId),
        getRun(targetId).catch(() => null),
      ])
      setJsonText(JSON.stringify(bindingBody.binding ?? bindingBody, null, 2))
      setViewRunId(targetId)

      const parentId = (runRec?.parent_run_id ?? '').trim()
      setParentRunId(parentId || null)

      const rootId = parentId || targetId
      try {
        const kids = await listRunChildren(rootId)
        setChildren(kids)
        if (parentId && runRec && !kids.some((k) => k.id === targetId)) {
          setChildren([...kids, runRec])
        }
      } catch {
        setChildren([])
        setNavHint(CHAT.bindingNavUnavailable)
      }
      setOpen(true)
    } catch (e) {
      if (
        opts?.hideOnMissing !== false &&
        e instanceof ApiError &&
        (e.status === 404 || e.code === 'binding_not_found')
      ) {
        setHidden(true)
        return
      }
      const f = friendlyError(e)
      setError(f.detail ? `${f.title}: ${f.detail}` : f.title)
      setViewRunId(targetId)
      setOpen(true)
    } finally {
      setBusy(false)
    }
  }

  const openBinding = async () => {
    setViewRunId(runId)
    setParentRunId(null)
    setChildren([])
    await loadBindingFor(runId, { hideOnMissing: true })
  }

  const switchTo = async (targetId: string) => {
    if (!targetId || targetId === viewRunId || busy) return
    await loadBindingFor(targetId, { hideOnMissing: false })
  }

  if (hidden || !runId.trim()) return null

  const showNav = Boolean(parentRunId) || children.length > 0

  return (
    <>
      <button
        type="button"
        className="btn ghost sm"
        data-testid="run-binding-open"
        disabled={busy}
        onClick={() => void openBinding()}
      >
        {busy ? CHAT.bindingLoading : CHAT.viewBinding}
      </button>
      <Modal
        open={open}
        title={CHAT.bindingTitle}
        onClose={() => setOpen(false)}
        footer={
          <Button variant="primary" onClick={() => setOpen(false)}>
            {CHAT.bindingClose}
          </Button>
        }
      >
        <p className="settings-muted">{CHAT.bindingHint}</p>
        <p className="settings-muted" data-testid="run-binding-viewing">
          {CHAT.bindingViewingRun(shortRunId(viewRunId))}
        </p>
        {showNav ? (
          <div className="run-binding-nav" data-testid="run-binding-nav">
            {parentRunId ? (
              <button
                type="button"
                className="btn ghost sm"
                data-testid="run-binding-open-parent"
                disabled={busy || viewRunId === parentRunId}
                onClick={() => void switchTo(parentRunId)}
              >
                {CHAT.bindingOpenParent}
              </button>
            ) : null}
            {children.length > 0 ? (
              <div className="run-binding-children">
                <p className="run-binding-children-title">{CHAT.bindingChildrenTitle}</p>
                <ul className="run-binding-child-list">
                  {children.map((child) => {
                    const current = child.id === viewRunId
                    return (
                      <li key={child.id} className="run-binding-child-item">
                        <span className="run-binding-child-caption">{childCaption(child)}</span>
                        {current ? (
                          <span className="run-binding-child-current">{CHAT.bindingViewing}</span>
                        ) : (
                          <button
                            type="button"
                            className="btn ghost sm"
                            data-testid={`run-binding-open-child-${child.id}`}
                            disabled={busy}
                            onClick={() => void switchTo(child.id)}
                          >
                            {CHAT.bindingChildOpen}
                          </button>
                        )}
                      </li>
                    )
                  })}
                </ul>
              </div>
            ) : null}
            {navHint ? (
              <p className="settings-muted" role="status">
                {navHint}
              </p>
            ) : null}
          </div>
        ) : null}
        {error ? (
          <p className="ui-inline-error" role="alert">
            {error}
          </p>
        ) : (
          <pre
            className="settings-code-sample"
            data-testid="run-binding-body"
            style={{ maxHeight: '50vh', overflow: 'auto' }}
          >
            {jsonText}
          </pre>
        )}
      </Modal>
    </>
  )
}
