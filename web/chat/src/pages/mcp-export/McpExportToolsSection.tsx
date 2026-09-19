import { Field, Input, Select } from '../../components/ui'
import { COMMON, MCP_EXPORTS } from '../../strings'
import type { ToolExportMode } from '../../api'
import { exportOptions, toolExportMode } from './mcpExportHelpers'
import type { McpExportSettingsController } from './useMcpExportSettings'

type Props = Pick<
  McpExportSettingsController,
  | 'tools'
  | 'toolsError'
  | 'toolQuery'
  | 'setToolQuery'
  | 'filteredTools'
  | 'exportBusy'
  | 'onExportChange'
>

export function McpExportToolsSection({
  tools,
  toolsError,
  toolQuery,
  setToolQuery,
  filteredTools,
  exportBusy,
  onExportChange,
}: Props) {
  return (
    <section className="settings-form">
      <h2 className="settings-subheading">{MCP_EXPORTS.toolsExportTitle}</h2>
      <p className="settings-meta">{MCP_EXPORTS.toolsExportIntro}</p>
      {toolsError && (
        <p className="ui-inline-error" role="alert">
          {toolsError}
        </p>
      )}
      {tools != null && tools.length > 0 && (
        <Field label={MCP_EXPORTS.toolsExportSearch}>
          <Input
            value={toolQuery}
            onChange={(e) => setToolQuery(e.target.value)}
            placeholder={MCP_EXPORTS.toolsExportSearch}
            aria-label={MCP_EXPORTS.toolsExportSearch}
          />
        </Field>
      )}
      {tools == null && !toolsError && <p className="settings-muted">{COMMON.loading}</p>}
      {tools != null && tools.length === 0 && !toolsError && (
        <p className="settings-empty">{MCP_EXPORTS.toolsExportEmpty}</p>
      )}
      {tools != null && tools.length > 0 && filteredTools.length === 0 && (
        <p className="settings-empty">{MCP_EXPORTS.toolsExportEmpty}</p>
      )}
      {filteredTools.length > 0 && (
        <ul className="settings-list">
          {filteredTools.map((t) => (
            <li key={t.name} className="settings-list-item">
              <span className="settings-tool-line">
                <span className="settings-tool-title">{t.title || t.name}</span>
                {t.description ? <span className="settings-tool-desc">{t.description}</span> : null}
              </span>
              <div className="settings-toolbar">
                <Select
                  value={toolExportMode(t)}
                  disabled={exportBusy === t.name}
                  aria-label={MCP_EXPORTS.exportPolicyAria(t.title || t.name)}
                  onChange={(e) => {
                    void onExportChange(t.name, e.target.value as ToolExportMode)
                  }}
                >
                  {exportOptions().map((opt) => (
                    <option key={opt.value} value={opt.value}>
                      {opt.label}
                    </option>
                  ))}
                </Select>
              </div>
              {t.source === 'mcp' ? (
                <p className="settings-muted">{MCP_EXPORTS.toolsExportMcpWriteHint}</p>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
