import { useCallback, useEffect, useState } from 'react'
import { Users } from 'lucide-react'
import {
  clearIdentities,
  deleteIdentity,
  listIdentities,
  setDefaultIdentity,
  type IdentityView,
} from '../api'
import {
  Badge,
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  PageHeader,
  ToastRegion,
  useToast,
} from '../components/ui'
import { redactSensitive } from '../sensitive'
import { ACCOUNTS, friendlyError, identitySourceLabel } from '../strings'
import { uuid } from '../uuid'

const CONV_KEY = 'baize.conversation_id'

function loadConversationId(): string {
  const existing = localStorage.getItem(CONV_KEY)?.trim()
  if (existing) return existing
  const id = `conv_${uuid()}`
  localStorage.setItem(CONV_KEY, id)
  return id
}

function formatClaims(claims: Record<string, unknown> | undefined): string | null {
  if (!claims || Object.keys(claims).length === 0) return null
  try {
    return JSON.stringify(redactSensitive(claims), null, 2)
  } catch {
    return null
  }
}

export function IdentitiesSettings() {
  const [conversationId] = useState(loadConversationId)
  const [identities, setIdentities] = useState<IdentityView[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [confirmClear, setConfirmClear] = useState(false)
  const { toasts, push, dismiss } = useToast()

  const refresh = useCallback(async () => {
    try {
      setIdentities(await listIdentities(conversationId))
    } catch (e) {
      setIdentities(null)
      const f = friendlyError(e)
      push({ tone: 'error', title: ACCOUNTS.loadFailed, detail: f.detail ?? f.title })
    }
  }, [conversationId, push])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const runAction = useCallback(
    async (fn: () => Promise<void>, successTitle: string) => {
      setBusy(true)
      try {
        await fn()
        await refresh()
        push({ tone: 'success', title: successTitle })
      } catch (e) {
        const f = friendlyError(e)
        push({ tone: 'error', title: f.title, detail: f.detail })
      } finally {
        setBusy(false)
      }
    },
    [refresh, push],
  )

  const hasCaptured = (identities ?? []).some((i) => i.source !== 'env')

  return (
    <div className="settings-panel">
      <PageHeader title={ACCOUNTS.title} description={ACCOUNTS.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {identities === null && <p className="settings-muted">加载中…</p>}
      {identities !== null && identities.length === 0 && (
        <EmptyState
          icon={<Users size={28} aria-hidden="true" />}
          title={ACCOUNTS.emptyTitle}
          description={ACCOUNTS.emptyDesc}
        />
      )}

      {identities !== null && identities.length > 0 && (
        <div className="accounts-grid">
          {identities.map((idt) => {
            const claimsText = formatClaims(idt.claims_summary)
            const scheme = idt.scheme ? `${idt.scheme} · ` : ''
            return (
              <Card
                key={idt.id}
                title={idt.label || idt.id}
                description={`${scheme}${identitySourceLabel(idt.source)}`}
                trailing={
                  <div className="accounts-actions">
                    {idt.is_default ? (
                      <Badge tone="success">{ACCOUNTS.defaultBadge}</Badge>
                    ) : (
                      <Button
                        size="sm"
                        variant="secondary"
                        disabled={busy}
                        onClick={() =>
                          void runAction(() => setDefaultIdentity(conversationId, idt.id), ACCOUNTS.toastDefault)
                        }
                      >
                        {ACCOUNTS.setDefault}
                      </Button>
                    )}
                    {idt.source !== 'env' && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="danger-text"
                        disabled={busy}
                        onClick={() =>
                          void runAction(() => deleteIdentity(conversationId, idt.id), ACCOUNTS.toastLogout)
                        }
                      >
                        {ACCOUNTS.logout}
                      </Button>
                    )}
                  </div>
                }
              >
                {claimsText && (
                  <details className="accounts-claims-details">
                    <summary>{ACCOUNTS.details}</summary>
                    <pre className="accounts-claims">{claimsText}</pre>
                  </details>
                )}
              </Card>
            )
          })}
        </div>
      )}

      {hasCaptured && (
        <Button
          className="accounts-clear"
          variant="ghost"
          disabled={busy}
          onClick={() => setConfirmClear(true)}
        >
          {ACCOUNTS.clear}
        </Button>
      )}

      <ConfirmDialog
        open={confirmClear}
        danger
        title={ACCOUNTS.clearConfirmTitle}
        body={ACCOUNTS.clearConfirmBody}
        confirmText={ACCOUNTS.clearConfirmOk}
        busy={busy}
        onCancel={() => setConfirmClear(false)}
        onConfirm={() =>
          void runAction(() => clearIdentities(conversationId), ACCOUNTS.toastCleared).then(() =>
            setConfirmClear(false),
          )
        }
      />

      <details className="settings-developer">
        <summary>{ACCOUNTS.developer}</summary>
        <p className="settings-meta">会话 {conversationId}</p>
      </details>
    </div>
  )
}
