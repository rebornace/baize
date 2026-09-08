import { authHeaders } from '../controlAuth'
import { useGate } from '../gateContext'

export interface ChannelFileLinkProps {
  name: string
  url: string
}

/**
 * Renders a download link for an inbound channel file (docx/pdf/zip/…). The
 * endpoint is conversation-ACL protected and a plain <a href> cannot send the
 * Bearer token, so we fetch with auth headers and trigger a download through a
 * blob URL (same approach as artifact pages / inline images).
 */
export function ChannelFileLink({ name, url }: ChannelFileLinkProps) {
  const { gateEnabled } = useGate()

  const download = async () => {
    try {
      const res = await fetch(url, {
        headers: gateEnabled ? (authHeaders(true) as Record<string, string>) : undefined,
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const blob = await res.blob()
      const objectUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = objectUrl
      a.download = name
      document.body.appendChild(a)
      a.click()
      a.remove()
      // Revoke after the click has been processed.
      window.setTimeout(() => URL.revokeObjectURL(objectUrl), 30_000)
    } catch {
      // Fallback: open the URL directly (works when the gate is off).
      window.open(url, '_blank', 'noopener,noreferrer')
    }
  }

  return (
    <button type="button" className="channel-file-link" onClick={() => void download()}>
      📎 {name}
      <span className="channel-file-hint">点击下载</span>
    </button>
  )
}
