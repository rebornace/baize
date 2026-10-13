import { Button } from '../../components/ui'
import { COMMON, MCP_EXPORTS } from '../../strings'
import { groupToolsTree } from '../../toolCatalog'
import type { ToolInfo } from '../../api'
import {
  readOnlyExportCandidateNames,
  selectAllConnectorTools,
  toggleAllowedTool,
} from './mcpExportAllowlist'

type Props = {
  tools: ToolInfo[] | null
  selected: Set<string>
  onSelectedChange: (next: Set<string>) => void
  disabled?: boolean
}

export function McpExportKeyAllowlistPicker({
  tools,
  selected,
  onSelectedChange,
  disabled = false,
}: Props) {
  const groups = tools != null ? groupToolsTree(tools) : []

  const onPrefillReadonly = () => {
    if (tools == null) return
    onSelectedChange(new Set(readOnlyExportCandidateNames(tools)))
  }

  return (
    <div className="settings-form">
      <h4 className="settings-subheading">{MCP_EXPORTS.keyAllowlistTitle}</h4>
      <p className="settings-meta">{MCP_EXPORTS.keyAllowlistIntro}</p>
      <p className="settings-muted">{MCP_EXPORTS.keyAllowlistFailClosedHint}</p>
      <p className="settings-muted">{MCP_EXPORTS.keyAllowlistSelectedCount(selected.size)}</p>
      <div className="settings-toolbar">
        <Button
          type="button"
          variant="secondary"
          size="sm"
          disabled={disabled || tools == null || tools.length === 0}
          onClick={onPrefillReadonly}
        >
          {MCP_EXPORTS.keyAllowlistPrefillReadonly}
        </Button>
      </div>
      {tools == null && !disabled && <p className="settings-muted">{COMMON.loading}</p>}
      {tools != null && tools.length === 0 && (
        <p className="settings-empty">{MCP_EXPORTS.keyAllowlistNoTools}</p>
      )}
      {groups.map((group) => {
        const connectorLabel = group.connectorId.trim()
          ? group.connectorId
          : MCP_EXPORTS.keyAllowlistConnectorOther
        const connectorToolNames = group.prefixes.flatMap((p) => p.tools.map((t) => t.name))
        const allSelected =
          connectorToolNames.length > 0 && connectorToolNames.every((n) => selected.has(n))
        return (
          <section key={group.connectorId || '__other'} className="settings-form">
            <div className="settings-toolbar">
              <h5 className="settings-subheading">{connectorLabel}</h5>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={disabled || connectorToolNames.length === 0}
                onClick={() => {
                  if (tools == null) return
                  if (allSelected) {
                    const next = new Set(selected)
                    for (const n of connectorToolNames) next.delete(n)
                    onSelectedChange(next)
                  } else {
                    onSelectedChange(
                      selectAllConnectorTools(tools, group.connectorId, selected),
                    )
                  }
                }}
              >
                {allSelected
                  ? MCP_EXPORTS.keyAllowlistClearConnector(connectorLabel)
                  : MCP_EXPORTS.keyAllowlistSelectConnector(connectorLabel)}
              </Button>
            </div>
            <ul className="settings-list">
              {group.prefixes.flatMap((prefixGroup) =>
                prefixGroup.tools.map((t) => (
                  <li key={t.name} className="settings-list-item settings-skill-row">
                    <label className="settings-login-toggle settings-skill-check">
                      <input
                        type="checkbox"
                        checked={selected.has(t.name)}
                        disabled={disabled}
                        aria-label={MCP_EXPORTS.keyAllowlistToolAria(t.title || t.name)}
                        onChange={(e) => {
                          onSelectedChange(toggleAllowedTool(selected, t.name, e.target.checked))
                        }}
                      />
                      <span className="settings-skill-line">
                        <span className="settings-tool-title">{t.title || t.name}</span>
                        <span className="settings-tool-sub">{t.name}</span>
                        {t.method ? (
                          <span className="settings-muted">
                            {t.method}
                            {t.path ? ` ${t.path}` : ''}
                          </span>
                        ) : null}
                      </span>
                    </label>
                  </li>
                )),
              )}
            </ul>
          </section>
        )
      })}
    </div>
  )
}
