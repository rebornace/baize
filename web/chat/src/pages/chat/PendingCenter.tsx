import { useEffect, useState } from 'react'
import { ApiError, type PendingApproval } from '../../api'
import { Button, Modal } from '../../components/ui'
import {
  desktopNotifyEnabled,
  notificationsSupported,
  requestDesktopNotifyPermission,
  setDesktopNotifyEnabled,
} from '../../desktopNotify'
import { CHAT, HITL } from '../../strings'
import { waitedLabel } from '../../waitTime'

export type PendingDecision = 'approve' | 'reject'

export type PendingCenterProps = {
  open: boolean
  items: PendingApproval[]
  onClose: () => void
  onOpenConversation: (conversationId: string) => void
  /** Approve/reject one tool approval. The parent removes the row on success. */
  onDecide: (runId: string, decision: PendingDecision, comment: string) => Promise<void>
}

function kindLabel(kind: string): string {
  switch (kind) {
    case 'login':
      return CHAT.pendingKindLogin
    case 'waiting_child':
      return CHAT.pendingKindChild
    default:
      return CHAT.pendingKindApproval
  }
}

/** Cross-conversation list of waiting_human runs. Approval rows can be decided
 *  inline; login/child waits still open the chat. */
export function PendingCenter({
  open,
  items,
  onClose,
  onOpenConversation,
  onDecide,
}: PendingCenterProps) {
  // The parent re-renders on locale changes and CHAT reads the live pack.
  const now = Date.now()
  const [comments, setComments] = useState<Record<string, string>>({})
  const [busyRunId, setBusyRunId] = useState<string | null>(null)
  const [confirmRejectId, setConfirmRejectId] = useState<string | null>(null)
  const [rowError, setRowError] = useState<{ id: string; message: string } | null>(null)
  const [notifyOn, setNotifyOn] = useState(() => desktopNotifyEnabled())
  const [notifyMsg, setNotifyMsg] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setComments({})
    setBusyRunId(null)
    setConfirmRejectId(null)
    setRowError(null)
  }, [open])

  const toggleNotify = async (next: boolean) => {
    if (!next) {
      setDesktopNotifyEnabled(false)
      setNotifyOn(false)
      setNotifyMsg(null)
      return
    }
    if (!notificationsSupported()) {
      setDesktopNotifyEnabled(false)
      setNotifyOn(false)
      setNotifyMsg(CHAT.pendingNotifyUnsupported)
      return
    }
    const permission = await requestDesktopNotifyPermission()
    if (permission === 'granted') {
      setDesktopNotifyEnabled(true)
      setNotifyOn(true)
      setNotifyMsg(null)
    } else {
      setDesktopNotifyEnabled(false)
      setNotifyOn(false)
      setNotifyMsg(CHAT.pendingNotifyDenied)
    }
  }

  const decide = async (item: PendingApproval, decision: PendingDecision) => {
    if (busyRunId) return
    setBusyRunId(item.run_id)
    setRowError(null)
    try {
      await onDecide(item.run_id, decision, (comments[item.run_id] || '').trim())
    } catch (e) {
      const stale = e instanceof ApiError && e.status === 409
      setRowError({ id: item.run_id, message: stale ? CHAT.pendingStale : CHAT.pendingActionFailed })
      setConfirmRejectId(null)
    } finally {
      setBusyRunId(null)
    }
  }

  const itemActions = (item: PendingApproval) => {
    const busy = busyRunId === item.run_id
    if (item.kind !== 'approval') {
      return <span className="pending-login-hint">{CHAT.pendingLoginHint}</span>
    }
    return (
      <>
        <Button
          size="sm"
          variant="primary"
          disabled={busy}
          onClick={() => void decide(item, 'approve')}
        >
          {HITL.approve}
        </Button>
        <Button
          size="sm"
          variant="danger"
          disabled={busy}
          onClick={() => {
            if (confirmRejectId === item.run_id) {
              void decide(item, 'reject')
            } else {
              setConfirmRejectId(item.run_id)
            }
          }}
        >
          {confirmRejectId === item.run_id ? HITL.confirmReject : HITL.reject}
        </Button>
      </>
    )
  }

  return (
    <Modal
      open={open}
      title={CHAT.pendingCenterTitle}
      onClose={onClose}
      footer={
        <div className="pending-footer">
          <div className="pending-notify">
            <label className="pending-notify-label">
              <input
                type="checkbox"
                checked={notifyOn}
                onChange={(e) => void toggleNotify(e.target.checked)}
              />
              {CHAT.pendingNotifyToggle}
            </label>
            <span className={notifyMsg ? 'pending-notify-error' : 'pending-notify-hint'}>
              {notifyMsg || CHAT.pendingNotifyHint}
            </span>
          </div>
          <Button variant="secondary" onClick={onClose}>
            {CHAT.pendingCenterClose}
          </Button>
        </div>
      }
    >
      {items.length === 0 ? (
        <p className="pending-empty" data-testid="pending-empty">
          {CHAT.pendingCenterEmpty}
        </p>
      ) : (
        <ul className="pending-list" data-testid="pending-list">
          {items.map((item) => {
            const waited = waitedLabel(item.waiting_since, now, HITL)
            const title = item.conversation_title || item.conversation_id
            const hasArgs = item.arguments && Object.keys(item.arguments).length > 0
            const busy = busyRunId === item.run_id
            return (
              <li className="pending-item" key={item.run_id}>
                <div className="pending-item-body">
                  <p className="pending-item-title">{title}</p>
                  <p className="pending-item-meta">
                    <span className={`pending-kind pending-kind-${item.kind}`}>{kindLabel(item.kind)}</span>
                    {item.origin === 'scheduled' && (
                      <span className="pending-tag">{CHAT.pendingOriginScheduled}</span>
                    )}
                    {waited && <span className="pending-waited">{waited}</span>}
                  </p>
                  {item.tool_name && <p className="pending-tool">{item.tool_name}</p>}
                  {item.prompt && <p className="pending-prompt">{item.prompt}</p>}
                  {item.workflow_skill && (
                    <p className="pending-workflow">
                      {CHAT.pendingWorkflow(item.workflow_skill, item.workflow_step || '')}
                    </p>
                  )}
                  {hasArgs && (
                    <details className="pending-args">
                      <summary>{HITL.viewParams}</summary>
                      <pre>{JSON.stringify(item.arguments, null, 2)}</pre>
                    </details>
                  )}
                  {item.kind === 'approval' && (
                    <input
                      className="pending-comment"
                      type="text"
                      aria-label={HITL.commentPlaceholder}
                      placeholder={HITL.commentPlaceholder}
                      value={comments[item.run_id] || ''}
                      disabled={busy}
                      onChange={(e) => {
                        const value = e.target.value
                        setComments((prev) => ({ ...prev, [item.run_id]: value }))
                      }}
                    />
                  )}
                  {rowError?.id === item.run_id && (
                    <p className="pending-error" role="alert">
                      {rowError.message}
                    </p>
                  )}
                </div>
                <div className="pending-item-actions">
                  {itemActions(item)}
                  <Button
                    size="sm"
                    variant="secondary"
                    disabled={busy}
                    onClick={() => onOpenConversation(item.conversation_id)}
                  >
                    {CHAT.pendingCenterOpen}
                  </Button>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </Modal>
  )
}
