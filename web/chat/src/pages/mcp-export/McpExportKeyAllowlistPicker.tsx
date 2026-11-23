import { useState } from 'react'
import { Button, Field, Input } from '../../components/ui'
import { COMMON, MCP_EXPORTS } from '../../strings'
import { groupToolsTree, toolMatchesQuery } from '../../toolCatalog'
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

function connectorToolNames(tools: ToolInfo[], connectorId: string): string[] {
  return tools.filter((t) => (t.connector_id || '') === connectorId).map((t) => t.name)
}

export function McpExportKeyAllowlistPicker({
  tools,
  selected,
  onSelectedChange,
  disabled = false,
}: Props) {
  const [query, setQuery] = useState('')
  const filtered =
    tools == null ? [] : tools.filter((t) => toolMatchesQuery(t, query))
  const groups = groupToolsTree(filtered)
  const searching = query.trim() !== ''

  const onPrefillReadonly = () => {
    if (tools == null) return
    onSelectedChange(new Set(readOnlyExportCandidateNames(tools)))
  }

  return (
    <div className="settings-form mcp-export-allowlist">
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
      {tools != null && tools.length > 0 && (
        <Field label={MCP_EXPORTS.keyAllowlistSearch}>
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={MCP_EXPORTS.keyAllowlistSearch}
            aria-label={MCP_EXPORTS.keyAllowlistSearch}
            disabled={disabled}
          />
        </Field>
      )}
      {tools != null && tools.length > 0 && filtered.length === 0 && (
        <p className="settings-empty">{MCP_EXPORTS.keyAllowlistNoMatch}</p>
      )}
      {groups.length > 0 && (
        <div
          className="settings-scroll-list mcp-export-allowlist-tree"
          data-testid="mcp-export-allowlist-tree"
        >
          {groups.map((group, groupIndex) => {
            const connectorLabel = group.connectorId.trim()
              ? group.connectorId
              : MCP_EXPORTS.keyAllowlistConnectorOther
            const allNames =
              tools == null ? [] : connectorToolNames(tools, group.connectorId)
            const visibleNames = group.prefixes.flatMap((p) => p.tools.map((t) => t.name))
            const selectedInGroup = allNames.filter((n) => selected.has(n)).length
            const allSelected =
              allNames.length > 0 && allNames.every((n) => selected.has(n))
            const startOpen =
              searching || selectedInGroup > 0 || groups.length <= 2 || groupIndex === 0
            return (
              <details
                key={`${searching ? 'q' : 'n'}:${group.connectorId || '__other'}`}
                className="mcp-export-allowlist-group"
                ref={(node) => {
                  if (!node || !startOpen || node.dataset.baizeOpenInit === '1') return
                  node.open = true
                  node.dataset.baizeOpenInit = '1'
                }}
              >
                <summary className="mcp-export-allowlist-summary">
                  <span className="settings-tool-title">{connectorLabel}</span>
                  <span className="settings-muted">
                    {MCP_EXPORTS.keyAllowlistGroupCount(selectedInGroup, allNames.length)}
                  </span>
                </summary>
                <div className="settings-toolbar mcp-export-allowlist-group-actions">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={disabled || visibleNames.length === 0}
                    onClick={() => {
                      if (tools == null) return
                      if (allSelected) {
                        const next = new Set(selected)
                        for (const n of allNames) next.delete(n)
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
                              onSelectedChange(
                                toggleAllowedTool(selected, t.name, e.target.checked),
                              )
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
              </details>
            )
          })}
        </div>
      )}
    </div>
  )
}
