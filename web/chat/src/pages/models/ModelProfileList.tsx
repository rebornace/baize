import type { ModelProfile } from '../../api'
import { Badge, Button, Card } from '../../components/ui'
import { tierLabel } from '../../modelSelect'
import { MODELS, VISION_LABEL } from '../../strings'
import { credentialHint, resolveThinkingLevel } from './modelSettingsHelpers'

export interface ModelProfileListProps {
  profiles: ModelProfile[]
  busy: boolean
  readOnly?: boolean
  onEdit: (p: ModelProfile) => void
  onDelete: (p: ModelProfile) => void
}

export function ModelProfileList({
  profiles,
  busy,
  readOnly = false,
  onEdit,
  onDelete,
}: ModelProfileListProps) {
  if (profiles.length === 0) return null
  return (
    <div className="connector-list">
      {profiles.map((p) => (
        <Card
          key={p.id}
          title={p.name}
          description={`${p.model} · ${p.base_url}`}
          trailing={
            <div className="accounts-actions">
              <Badge tone="info">{tierLabel(p.auto_tier)}</Badge>
              {p.supports_vision && <Badge tone="info">{VISION_LABEL}</Badge>}
            </div>
          }
        >
          <p className="settings-muted">
            {credentialHint(p)}
            {resolveThinkingLevel(p) === 'off' ? ` · ${MODELS.listThinkingOff}` : ''}
            {p.context_tokens > 0 ? ` · ${p.context_tokens} ctx` : ''}
          </p>
          {!readOnly && (
            <div className="settings-toolbar">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={busy}
                onClick={() => onEdit(p)}
              >
                {MODELS.edit}
              </Button>
              <Button
                type="button"
                variant="danger"
                size="sm"
                disabled={busy}
                onClick={() => onDelete(p)}
              >
                {MODELS.delete}
              </Button>
            </div>
          )}
        </Card>
      ))}
    </div>
  )
}
