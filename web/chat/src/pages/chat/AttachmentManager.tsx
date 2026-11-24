import { useCallback, useEffect, useState } from 'react'
import { File as FileIcon } from 'lucide-react'
import { Button, Input, Modal } from '../../components/ui'
import { ChannelImage } from '../../components/ChannelImage'
import { fileKind } from '../../components/ChannelFileLink'
import {
  deleteConversationAttachments,
  getConversationCostBudget,
  getConversationRetention,
  listConversationAttachments,
  putConversationCostBudget,
  putConversationRetention,
  type ConversationAttachment,
  type ConversationCostBudget,
  type ConversationRetention,
} from '../../api/conversationAttachments'
import { CHAT } from '../../strings'

export interface AttachmentManagerProps {
  conversationId: string
  open: boolean
  onClose: () => void
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

function formatTime(raw: string): string {
  const d = new Date(raw)
  return Number.isNaN(d.getTime()) ? raw : d.toLocaleString()
}

export function AttachmentManager({ conversationId, open, onClose }: AttachmentManagerProps) {
  const [items, setItems] = useState<ConversationAttachment[] | null>(null)
  const [selected, setSelected] = useState<Record<string, boolean>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [retention, setRetention] = useState<ConversationRetention | null>(null)
  const [retentionDays, setRetentionDays] = useState('-1')
  const [retentionBusy, setRetentionBusy] = useState(false)
  const [costBudget, setCostBudget] = useState<ConversationCostBudget | null>(null)
  const [budgetDay, setBudgetDay] = useState('0')
  const [budgetMonth, setBudgetMonth] = useState('0')
  const [budgetBusy, setBudgetBusy] = useState(false)

  const load = useCallback(async () => {
    const [listRes, retRes, budgetRes] = await Promise.allSettled([
      listConversationAttachments(conversationId),
      getConversationRetention(conversationId),
      getConversationCostBudget(conversationId),
    ])
    if (listRes.status === 'fulfilled') {
      setItems(listRes.value.items)
      setSelected({})
      setError(null)
    } else {
      setItems([])
      setError(CHAT.attachmentManagerLoadFailed)
    }
    if (retRes.status === 'fulfilled') {
      setRetention(retRes.value)
      setRetentionDays(String(retRes.value.days))
    }
    if (budgetRes.status === 'fulfilled') {
      setCostBudget(budgetRes.value)
      setBudgetDay(String(budgetRes.value.budget_usd_per_day))
      setBudgetMonth(String(budgetRes.value.budget_usd_per_month))
    }
  }, [conversationId])

  useEffect(() => {
    if (open && conversationId) void load()
  }, [open, conversationId, load])

  const objects = Object.keys(selected).filter((k) => selected[k])
  const allSelected = items != null && items.length > 0 && objects.length === items.length

  const toggleAll = () => {
    if (!items) return
    if (allSelected) {
      setSelected({})
      return
    }
    const next: Record<string, boolean> = {}
    for (const it of items) next[it.object] = true
    setSelected(next)
  }

  const remove = async () => {
    if (objects.length === 0) return
    if (!window.confirm(CHAT.attachmentManagerConfirm(objects.length))) return
    setBusy(true)
    try {
      const res = await deleteConversationAttachments(conversationId, objects)
      await load()
      if (res.failed > 0) {
        setError(CHAT.attachmentManagerPartial(res.deleted, res.failed))
      }
    } catch {
      setError(CHAT.attachmentManagerFailed)
    } finally {
      setBusy(false)
    }
  }

  const saveRetention = async () => {
    const days = Number.parseInt(retentionDays, 10)
    if (!Number.isFinite(days) || days < -1 || days > 3650) {
      setError(CHAT.attachmentRetentionInvalid)
      return
    }
    setRetentionBusy(true)
    try {
      const next = await putConversationRetention(conversationId, days)
      setRetention(next)
      setRetentionDays(String(next.days))
      setError(null)
    } catch {
      setError(CHAT.attachmentRetentionFailed)
    } finally {
      setRetentionBusy(false)
    }
  }

  const saveBudget = async () => {
    const d = Number.parseFloat(budgetDay)
    const m = Number.parseFloat(budgetMonth)
    if (!Number.isFinite(d) || d < 0 || !Number.isFinite(m) || m < 0) {
      setError(CHAT.conversationBudgetInvalid)
      return
    }
    setBudgetBusy(true)
    try {
      const next = await putConversationCostBudget(conversationId, d, m)
      setCostBudget(next)
      setBudgetDay(String(next.budget_usd_per_day))
      setBudgetMonth(String(next.budget_usd_per_month))
      setError(null)
    } catch {
      setError(CHAT.conversationBudgetFailed)
    } finally {
      setBudgetBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      title={CHAT.attachmentManagerTitle}
      onClose={onClose}
      footer={
        <>
          <Button type="button" size="sm" variant="secondary" onClick={onClose} disabled={busy}>
            {CHAT.attachmentManagerClose}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="danger"
            onClick={() => void remove()}
            disabled={busy || objects.length === 0}
            data-testid="attachment-manager-delete"
          >
            {CHAT.attachmentManagerDelete}
          </Button>
        </>
      }
    >
      <p className="settings-muted">{CHAT.attachmentManagerHint}</p>
      {retention ? (
        <div className="attachment-retention">
          <span className="settings-muted">
            {CHAT.attachmentRetentionGlobal(retention.global_days)}
            {' · '}
            {CHAT.attachmentRetentionEffective(retention.effective_days)}
          </span>
          <label className="settings-muted">
            {CHAT.attachmentRetentionLabel}
            <Input
              type="number"
              min={-1}
              max={3650}
              value={retentionDays}
              disabled={retentionBusy}
              onChange={(e) => setRetentionDays(e.target.value)}
              data-testid="attachment-retention-days"
            />
          </label>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={retentionBusy}
            onClick={() => void saveRetention()}
            data-testid="attachment-retention-save"
          >
            {CHAT.attachmentRetentionSave}
          </Button>
        </div>
      ) : null}
      {costBudget ? (
        <div className="attachment-retention">
          <span className="settings-muted">{CHAT.conversationBudgetTitle}</span>
          <label className="settings-muted">
            {CHAT.conversationBudgetDay}
            <Input
              type="number"
              min={0}
              value={budgetDay}
              disabled={budgetBusy}
              onChange={(e) => setBudgetDay(e.target.value)}
              data-testid="conversation-budget-day"
            />
          </label>
          <label className="settings-muted">
            {CHAT.conversationBudgetMonth}
            <Input
              type="number"
              min={0}
              value={budgetMonth}
              disabled={budgetBusy}
              onChange={(e) => setBudgetMonth(e.target.value)}
              data-testid="conversation-budget-month"
            />
          </label>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            disabled={budgetBusy}
            onClick={() => void saveBudget()}
            data-testid="conversation-budget-save"
          >
            {CHAT.conversationBudgetSave}
          </Button>
        </div>
      ) : null}
      {error ? <p className="settings-error">{error}</p> : null}
      {items == null ? (
        <p className="settings-muted">{CHAT.attachmentManagerLoading}</p>
      ) : items.length === 0 ? (
        <p className="settings-muted">{CHAT.attachmentManagerEmpty}</p>
      ) : (
        <>
          <div className="attachment-manager-toolbar">
            <label className="settings-muted">
              <input
                type="checkbox"
                checked={allSelected}
                disabled={busy}
                onChange={toggleAll}
                data-testid="attachment-manager-select-all"
              />{' '}
              {CHAT.attachmentManagerSelectAll}
            </label>
            <span className="settings-muted">
              {CHAT.attachmentManagerSelected(objects.length)}
            </span>
          </div>
          <ul className="attachment-manager-list" data-testid="attachment-manager-list">
            {items.map((it) => {
              const isImage = it.media_type.startsWith('image/')
              const Kind = isImage ? null : fileKind(it.filename).icon
              return (
                <li key={it.object} className="attachment-manager-item">
                  <label>
                    <input
                      type="checkbox"
                      checked={!!selected[it.object]}
                      disabled={busy}
                      onChange={(e) =>
                        setSelected((prev) => ({ ...prev, [it.object]: e.target.checked }))
                      }
                    />
                  </label>
                  <div className="attachment-manager-thumb">
                    {isImage ? (
                      <ChannelImage
                        url={it.url}
                        name={it.filename}
                        deletable={false}
                        alt={it.filename}
                      />
                    ) : Kind ? (
                      <Kind size={22} />
                    ) : (
                      <FileIcon size={22} />
                    )}
                  </div>
                  <div className="attachment-manager-meta">
                    <span className="attachment-manager-name">{it.filename}</span>
                    <span className="settings-muted">
                      {formatBytes(it.size_bytes)} · {formatTime(it.created_at)}
                    </span>
                  </div>
                </li>
              )
            })}
          </ul>
        </>
      )}
    </Modal>
  )
}
