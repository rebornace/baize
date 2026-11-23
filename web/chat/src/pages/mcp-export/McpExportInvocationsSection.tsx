import { useCallback, useEffect, useState } from 'react'
import { listMCPExportInvocations, type MCPExportInvocation } from '../../api'
import { Badge, Button } from '../../components/ui'
import { MCP_EXPORTS, friendlyError } from '../../strings'

export function McpExportInvocationsSection() {
  const [rows, setRows] = useState<MCPExportInvocation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      setRows(await listMCPExportInvocations(50))
    } catch (e) {
      const f = friendlyError(e)
      setError(f.detail ? `${f.title}: ${f.detail}` : f.title)
      setRows([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <section className="settings-form" data-testid="mcp-export-invocations">
      <h2 className="settings-subheading">{MCP_EXPORTS.invocationsTitle}</h2>
      <p className="settings-muted">{MCP_EXPORTS.invocationsIntro}</p>
      <div className="settings-toolbar">
        <Button variant="secondary" size="sm" disabled={loading} onClick={() => void load()}>
          {MCP_EXPORTS.invocationsRefresh}
        </Button>
      </div>
      {error ? (
        <p className="ui-inline-error" role="alert">
          {error}
        </p>
      ) : null}
      {loading ? <p className="settings-muted">{MCP_EXPORTS.invocationsRefresh}…</p> : null}
      {!loading && !error && rows.length === 0 ? (
        <p className="settings-muted">{MCP_EXPORTS.invocationsEmpty}</p>
      ) : null}
      {!loading && rows.length > 0 ? (
        <div className="settings-table-wrap">
          <table className="settings-table" data-testid="mcp-export-invocations-table">
            <thead>
              <tr>
                <th>{MCP_EXPORTS.invocationsColAt}</th>
                <th>{MCP_EXPORTS.invocationsColTool}</th>
                <th>{MCP_EXPORTS.invocationsColKey}</th>
                <th>{MCP_EXPORTS.invocationsColIdentity}</th>
                <th>{MCP_EXPORTS.invocationsColHash}</th>
                <th>{MCP_EXPORTS.invocationsColRev}</th>
                <th>{MCP_EXPORTS.invocationsColStatus}</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id}>
                  <td>{r.at ? new Date(r.at).toLocaleString() : '—'}</td>
                  <td>
                    <code>{r.tool_name}</code>
                  </td>
                  <td>
                    <code>{r.key_id}</code>
                  </td>
                  <td>
                    <code>{r.identity_id}</code>
                  </td>
                  <td>
                    <code>{r.contract_hash ? r.contract_hash.slice(0, 12) : '—'}</code>
                  </td>
                  <td>{r.catalog_revision ?? '—'}</td>
                  <td>
                    <Badge tone={r.is_error ? 'danger' : 'success'}>
                      {r.is_error ? MCP_EXPORTS.invocationsErr : MCP_EXPORTS.invocationsOk}
                    </Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </section>
  )
}
