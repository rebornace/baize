import { useCallback, useEffect, useId, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  getConnectorGroups,
  getRuntimeSettings,
  listConnectors,
  patchRuntimeSettings,
  putConnectorGroups,
  type ConnectorGroup,
  type ConnectorInfo,
} from '../../api'
import { Button, ConfirmDialog, Field, Input } from '../ui'
import { RUNTIME, friendlyError } from '../../strings'

type Props = {
  readOnly: boolean
  onToast: (msg: string, kind?: 'ok' | 'err') => void
}

type DraftGroup = ConnectorGroup & { key: string }

function toastErr(onToast: Props['onToast'], e: unknown) {
  const f = friendlyError(e)
  onToast(f.detail ? `${f.title}: ${f.detail}` : f.title, 'err')
}

function sourceLabel(source: string): string {
  if (source === 'kv') return RUNTIME.connectorGroupsSourceUI
  if (source === 'yaml') return RUNTIME.connectorGroupsSourceFile
  return source
}

export function ConnectorGroupsPanel({ readOnly, onToast }: Props) {
  const uid = useId()
  const [groups, setGroups] = useState<DraftGroup[]>([])
  const [source, setSource] = useState('')
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [scopedOn, setScopedOn] = useState(false)
  const [loading, setLoading] = useState(true)
  const [savingGroups, setSavingGroups] = useState(false)
  const [togglingScoped, setTogglingScoped] = useState(false)
  const [removeIdx, setRemoveIdx] = useState<number | null>(null)

  const reload = useCallback(async () => {
    setLoading(true)
    try {
      const [g, rt, cons] = await Promise.all([
        getConnectorGroups(),
        getRuntimeSettings(),
        listConnectors().catch(() => [] as ConnectorInfo[]),
      ])
      setGroups(
        (g.groups ?? []).map((item, i) => ({
          ...item,
          key: item.id ? `id:${item.id}` : `row:${uid}:${i}`,
        })),
      )
      setSource(g.source ?? '')
      setScopedOn(!!rt.effective?.scoped_subrun_enabled)
      setConnectors(cons)
    } catch (e) {
      toastErr(onToast, e)
    } finally {
      setLoading(false)
    }
  }, [onToast, uid])

  useEffect(() => {
    void reload()
  }, [reload])

  const onSaveGroups = async () => {
    setSavingGroups(true)
    try {
      const payload = groups.map(({ id, connectors: members }) => ({
        id: id.trim(),
        connectors: members,
      }))
      const v = await putConnectorGroups(payload)
      setGroups(
        (v.groups ?? []).map((item, i) => ({
          ...item,
          key: item.id ? `id:${item.id}` : `row:${uid}:${i}`,
        })),
      )
      setSource(v.source ?? 'kv')
      onToast(RUNTIME.connectorGroupsSaved, 'ok')
    } catch (e) {
      toastErr(onToast, e)
    } finally {
      setSavingGroups(false)
    }
  }

  const onToggleScoped = async (next: boolean) => {
    setTogglingScoped(true)
    try {
      const v = await patchRuntimeSettings({ scoped_subrun_enabled: next })
      setScopedOn(!!v.effective?.scoped_subrun_enabled)
      onToast(RUNTIME.connectorGroupsScopedSaved, 'ok')
    } catch (e) {
      toastErr(onToast, e)
    } finally {
      setTogglingScoped(false)
    }
  }

  const updateGroup = (idx: number, patch: Partial<ConnectorGroup>) => {
    setGroups((prev) => prev.map((g, i) => (i === idx ? { ...g, ...patch } : g)))
  }

  const toggleConnector = (idx: number, id: string) => {
    setGroups((prev) =>
      prev.map((g, i) => {
        if (i !== idx) return g
        const has = g.connectors.includes(id)
        return {
          ...g,
          connectors: has ? g.connectors.filter((c) => c !== id) : [...g.connectors, id],
        }
      }),
    )
  }

  const confirmRemove = () => {
    if (removeIdx == null) return
    setGroups((prev) => prev.filter((_, i) => i !== removeIdx))
    setRemoveIdx(null)
  }

  return (
    <div className="connector-groups" data-testid="connector-groups-panel">
      <h2 className="settings-subheading">{RUNTIME.connectorGroupsTitle}</h2>
      <p className="settings-muted">{RUNTIME.connectorGroupsHint}</p>

      <section className="settings-form tool-matching-hero" data-testid="connector-groups-scoped">
        <div className="connector-groups-meta">
          {source ? (
            <p className="settings-meta" data-testid="connector-groups-source">
              {RUNTIME.connectorGroupsSource}: {sourceLabel(source)}
            </p>
          ) : null}
        </div>
        <label className="settings-checkbox" data-testid="scoped-subrun-toggle">
          <input
            type="checkbox"
            checked={scopedOn}
            disabled={readOnly || togglingScoped || loading}
            onChange={(e) => void onToggleScoped(e.target.checked)}
          />
          <span>{RUNTIME.connectorGroupsScopedEnable}</span>
        </label>
        <p className="settings-muted">{RUNTIME.connectorGroupsScopedHint}</p>
        {readOnly ? <p className="settings-muted">{RUNTIME.descriptionOperator}</p> : null}
      </section>

      {loading ? <p className="settings-muted">{RUNTIME.loading}</p> : null}

      {!loading && groups.length === 0 ? (
        <p className="settings-empty" data-testid="connector-groups-empty">
          {RUNTIME.connectorGroupsEmpty}
        </p>
      ) : null}

      {!loading && groups.length > 0 ? (
        <ul className="settings-card-list connector-groups-list">
          {groups.map((g, idx) => (
            <li key={g.key} className="settings-card" data-testid={`connector-group-${idx}`}>
              <Field label={RUNTIME.connectorGroupsId} hint={RUNTIME.connectorGroupsIdHint}>
                <Input
                  value={g.id}
                  disabled={readOnly}
                  placeholder={RUNTIME.connectorGroupsIdPlaceholder}
                  onChange={(e) => updateGroup(idx, { id: e.target.value })}
                  aria-label={RUNTIME.connectorGroupsId}
                />
              </Field>
              <Field label={RUNTIME.connectorGroupsMembers} hint={RUNTIME.connectorGroupsMembersHint}>
                {connectors.length === 0 ? (
                  <p className="settings-muted">
                    {RUNTIME.connectorGroupsNoConnectors}{' '}
                    <Link to="/settings/openapi">{RUNTIME.connectorGroupsGoConnectors}</Link>
                  </p>
                ) : (
                  <div className="connector-groups-members" role="group" aria-label={RUNTIME.connectorGroupsMembers}>
                    {connectors.map((c) => (
                      <label key={c.id} className="settings-checkbox connector-groups-member">
                        <input
                          type="checkbox"
                          checked={g.connectors.includes(c.id)}
                          disabled={readOnly}
                          onChange={() => toggleConnector(idx, c.id)}
                        />
                        <span>
                          <span className="connector-groups-member-id">{c.id}</span>
                          {c.type ? (
                            <span className="settings-muted connector-groups-member-type"> {c.type}</span>
                          ) : null}
                        </span>
                      </label>
                    ))}
                  </div>
                )}
              </Field>
              {!readOnly ? (
                <div className="settings-toolbar">
                  <Button type="button" variant="ghost" size="sm" onClick={() => setRemoveIdx(idx)}>
                    {RUNTIME.connectorGroupsRemove}
                  </Button>
                </div>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}

      {!readOnly ? (
        <div className="settings-actions tool-matching-actions">
          <Button
            type="button"
            variant="secondary"
            disabled={savingGroups || loading}
            onClick={() =>
              setGroups((prev) => [
                ...prev,
                { key: `draft:${uid}:${prev.length}:${Date.now()}`, id: '', connectors: [] },
              ])
            }
          >
            {RUNTIME.connectorGroupsAdd}
          </Button>
          <Button
            type="button"
            variant="primary"
            disabled={savingGroups || loading}
            onClick={() => void onSaveGroups()}
          >
            {savingGroups ? RUNTIME.saving : RUNTIME.connectorGroupsSave}
          </Button>
        </div>
      ) : null}

      <ConfirmDialog
        open={removeIdx != null}
        title={RUNTIME.connectorGroupsRemoveTitle}
        body={RUNTIME.connectorGroupsRemoveBody}
        confirmText={RUNTIME.connectorGroupsRemove}
        danger
        onConfirm={confirmRemove}
        onCancel={() => setRemoveIdx(null)}
      />
    </div>
  )
}
