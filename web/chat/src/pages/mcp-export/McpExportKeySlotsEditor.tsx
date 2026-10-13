import { Button, Field, Input, Select } from '../../components/ui'
import type { MCPExportIdentity, MCPExportIdentitySlot } from '../../api'
import { MCP_EXPORTS } from '../../strings'

type Props = {
  identities: MCPExportIdentity[]
  slots: MCPExportIdentitySlot[]
  onChange: (slots: MCPExportIdentitySlot[]) => void
  disabled?: boolean
}

export function McpExportKeySlotsEditor({ identities, slots, onChange, disabled }: Props) {
  const update = (index: number, patch: Partial<MCPExportIdentitySlot>) => {
    onChange(slots.map((s, i) => (i === index ? { ...s, ...patch } : s)))
  }
  const remove = (index: number) => {
    onChange(slots.filter((_, i) => i !== index))
  }
  const add = () => {
    const fallback = identities[0]?.id ?? ''
    onChange([...slots, { connector_id: '', role: '', identity_id: fallback }])
  }

  return (
    <div className="settings-form">
      <h4 className="settings-subheading">{MCP_EXPORTS.keySlotsTitle}</h4>
      <p className="settings-meta">{MCP_EXPORTS.keySlotsIntro}</p>
      {slots.map((slot, index) => {
        const isDefault = !(slot.connector_id ?? '').trim()
        return (
          <div key={index} className="settings-toolbar" style={{ flexWrap: 'wrap', gap: '0.5rem' }}>
            <Field label={isDefault ? MCP_EXPORTS.keySlotDefault : MCP_EXPORTS.keySlotConnector}>
              <Input
                value={slot.connector_id ?? ''}
                onChange={(e) => update(index, { connector_id: e.target.value })}
                disabled={disabled}
                placeholder={MCP_EXPORTS.phKeySlotConnector}
              />
            </Field>
            <Field label={MCP_EXPORTS.keySlotRole}>
              <Input
                value={slot.role ?? ''}
                onChange={(e) => update(index, { role: e.target.value })}
                disabled={disabled}
                placeholder={MCP_EXPORTS.phKeySlotRole}
              />
            </Field>
            <Field label={MCP_EXPORTS.keySlotIdentity} required>
              <Select
                value={slot.identity_id}
                onChange={(e) => update(index, { identity_id: e.target.value })}
                disabled={disabled || identities.length === 0}
              >
                {identities.map((i) => (
                  <option key={i.id} value={i.id}>
                    {i.name} ({i.id})
                  </option>
                ))}
              </Select>
            </Field>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={disabled || slots.length <= 1}
              onClick={() => remove(index)}
            >
              {MCP_EXPORTS.keySlotRemove}
            </Button>
          </div>
        )
      })}
      <div className="settings-toolbar">
        <Button type="button" variant="secondary" size="sm" disabled={disabled} onClick={add}>
          {MCP_EXPORTS.keySlotAdd}
        </Button>
      </div>
    </div>
  )
}
