import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { CalendarClock } from 'lucide-react'
import {
  createSchedule,
  deleteSchedule,
  listSchedules,
  pauseSchedule,
  resumeSchedule,
  type ScheduleEntry,
} from '../api'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  PageHeader,
  Textarea,
  ToastRegion,
  useToast,
} from '../components/ui'
import { SCHEDULE, friendlyError } from '../strings'

const CONV_KEY = 'baize.conversation_id'

function loadConversationId(): string {
  return localStorage.getItem(CONV_KEY)?.trim() ?? ''
}

type ScheduleKind = 'once' | 'daily' | 'weekly'

function kindLabel(kind: string): string {
  switch (kind) {
    case 'once':
      return SCHEDULE.kindOnce
    case 'daily':
      return SCHEDULE.kindDaily
    case 'weekly':
      return SCHEDULE.kindWeekly
    default:
      return kind
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case 'active':
      return SCHEDULE.statusActive
    case 'paused':
      return SCHEDULE.statusPaused
    case 'completed':
      return SCHEDULE.statusCompleted
    default:
      return status
  }
}

export function ScheduleSettings() {
  const { toasts, push, dismiss } = useToast()
  const [conversationId] = useState(loadConversationId)
  const [items, setItems] = useState<ScheduleEntry[] | null>(null)
  const [kind, setKind] = useState<ScheduleKind>('daily')
  const [instruction, setInstruction] = useState('')
  const [timezone, setTimezone] = useState('Asia/Shanghai')
  const [fireAt, setFireAt] = useState('')
  const [timeOfDay, setTimeOfDay] = useState('09:00')
  const [weekday, setWeekday] = useState(1)
  const [busy, setBusy] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<ScheduleEntry | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      setItems(await listSchedules())
    } catch (err) {
      setItems(null)
      const f = friendlyError(err)
      push({ tone: 'error', title: SCHEDULE.loadFailed, detail: f.detail ?? f.title })
    }
  }, [push])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    const text = instruction.trim()
    if (!text) {
      push({ tone: 'error', title: SCHEDULE.toastNeedInstruction })
      return
    }
    const conv = conversationId.trim()
    if (!conv) {
      push({ tone: 'error', title: SCHEDULE.toastNeedConversation })
      return
    }
    setBusy(true)
    try {
      const body: Parameters<typeof createSchedule>[0] = {
        conversation_id: conv,
        kind,
        instruction: text,
        timezone: timezone.trim() || undefined,
      }
      if (kind === 'once') {
        body.fire_at = fireAt.trim()
      }
      if (kind === 'daily' || kind === 'weekly') {
        body.time_of_day = timeOfDay.trim()
      }
      if (kind === 'weekly') {
        body.weekday = weekday
      }
      await createSchedule(body)
      setInstruction('')
      await refresh()
      push({ tone: 'success', title: SCHEDULE.toastCreated })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const runAction = async (fn: () => Promise<void>, successTitle: string) => {
    setBusy(true)
    try {
      await fn()
      await refresh()
      push({ tone: 'success', title: successTitle })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setBusy(true)
    setDeleteError(null)
    try {
      await deleteSchedule(pendingDelete.id)
      push({ tone: 'success', title: SCHEDULE.toastDeleted })
      setPendingDelete(null)
      await refresh()
    } catch (err) {
      const f = friendlyError(err)
      const msg = f.detail ?? f.title
      setDeleteError(msg)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const showEmpty = items !== null && items.length === 0

  return (
    <div className="settings-panel settings-schedules">
      <PageHeader title={SCHEDULE.title} description={SCHEDULE.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {items === null && <p className="settings-muted">{SCHEDULE.loading}</p>}

      <form className="settings-form" onSubmit={(e) => void onCreate(e)}>
        <h2 className="settings-subheading">{SCHEDULE.add}</h2>
        <Field label={SCHEDULE.kindLabel}>
          <select
            name="kind"
            className="settings-input"
            value={kind}
            disabled={busy}
            onChange={(e) => setKind(e.target.value as ScheduleKind)}
          >
            <option value="once">{SCHEDULE.kindOnce}</option>
            <option value="daily">{SCHEDULE.kindDaily}</option>
            <option value="weekly">{SCHEDULE.kindWeekly}</option>
          </select>
        </Field>
        <Field label={SCHEDULE.instructionLabel} hint={SCHEDULE.instructionHint}>
          <Textarea
            name="instruction"
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            disabled={busy}
            rows={3}
            placeholder={SCHEDULE.phInstruction}
          />
        </Field>
        <Field label={SCHEDULE.timezoneLabel} hint={SCHEDULE.timezoneHint}>
          <Input
            name="timezone"
            value={timezone}
            onChange={(e) => setTimezone(e.target.value)}
            disabled={busy}
          />
        </Field>
        {kind === 'once' && (
          <Field label={SCHEDULE.fireAtLabel} hint={SCHEDULE.fireAtHint}>
            <Input
              name="fire_at"
              type="datetime-local"
              value={fireAt}
              onChange={(e) => setFireAt(e.target.value)}
              disabled={busy}
            />
          </Field>
        )}
        {(kind === 'daily' || kind === 'weekly') && (
          <Field label={SCHEDULE.timeOfDayLabel}>
            <Input
              name="time_of_day"
              value={timeOfDay}
              onChange={(e) => setTimeOfDay(e.target.value)}
              disabled={busy}
              placeholder="09:00"
            />
          </Field>
        )}
        {kind === 'weekly' && (
          <Field label={SCHEDULE.weekdayLabel}>
            <select
              name="weekday"
              className="settings-input"
              value={weekday}
              disabled={busy}
              onChange={(e) => setWeekday(Number(e.target.value))}
            >
              {[1, 2, 3, 4, 5, 6, 7].map((d) => (
                <option key={d} value={d}>
                  {SCHEDULE.weekdayOption(d)}
                </option>
              ))}
            </select>
          </Field>
        )}
        <Button type="submit" variant="primary" disabled={busy}>
          {SCHEDULE.save}
        </Button>
      </form>

      {showEmpty && (
        <EmptyState
          icon={<CalendarClock size={28} aria-hidden="true" />}
          title={SCHEDULE.emptyTitle}
          description={SCHEDULE.emptyDesc}
        />
      )}

      {items !== null && items.length > 0 && (
        <ul className="settings-list">
          {items.map((entry) => (
            <li key={entry.id} className="settings-list-item">
              <div className="settings-tool-row">
                <div>
                  <p className="settings-tool-name">{entry.instruction}</p>
                  <p className="settings-muted">
                    {kindLabel(entry.kind)}
                    {entry.time_of_day ? ` · ${entry.time_of_day}` : ''}
                    {entry.weekday ? ` · ${SCHEDULE.weekdayOption(entry.weekday)}` : ''}
                    {' · '}
                    {statusLabel(entry.status)}
                  </p>
                </div>
                <div className="accounts-actions">
                  {entry.status === 'active' && (
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={busy}
                      onClick={() =>
                        void runAction(
                          () => pauseSchedule(entry.id).then(() => undefined),
                          SCHEDULE.toastPaused,
                        )
                      }
                    >
                      {SCHEDULE.pause}
                    </Button>
                  )}
                  {entry.status === 'paused' && (
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={busy}
                      onClick={() =>
                        void runAction(
                          () => resumeSchedule(entry.id).then(() => undefined),
                          SCHEDULE.toastResumed,
                        )
                      }
                    >
                      {SCHEDULE.resume}
                    </Button>
                  )}
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="danger-text"
                    disabled={busy}
                    onClick={() => {
                      setDeleteError(null)
                      setPendingDelete(entry)
                    }}
                  >
                    {SCHEDULE.delete}
                  </Button>
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        danger
        title={SCHEDULE.confirmDeleteTitle}
        body={SCHEDULE.confirmDeleteBody}
        confirmText={SCHEDULE.confirmDeleteOk}
        busy={busy}
        error={deleteError}
        onCancel={() => {
          if (busy) return
          setPendingDelete(null)
          setDeleteError(null)
        }}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  )
}
