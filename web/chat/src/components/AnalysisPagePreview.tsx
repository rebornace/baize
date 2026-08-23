import { useEffect, useState } from 'react'
import { authHeaders } from '../controlAuth'
import { useGate } from '../gateContext'

export interface AnalysisPagePreviewProps {
  artifactUrl: string
}

/**
 * Loads the analysis HTML with the same auth as the chat API (iframe cannot
 * send Bearer), then shows it via a blob URL. Falls back to the raw URL when
 * the control-plane gate is off and fetch is unnecessary.
 */
export function AnalysisPagePreview({ artifactUrl }: AnalysisPagePreviewProps) {
  const { gateEnabled } = useGate()
  const [src, setSrc] = useState<string | null>(gateEnabled ? null : artifactUrl)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let revoked: string | null = null
    let cancelled = false

    const load = async () => {
      setError(null)
      if (!gateEnabled) {
        setSrc(artifactUrl)
        return
      }
      try {
        const res = await fetch(artifactUrl, {
          headers: authHeaders(true) as Record<string, string>,
        })
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`)
        }
        const html = await res.text()
        const blob = new Blob([html], { type: 'text/html; charset=utf-8' })
        const url = URL.createObjectURL(blob)
        revoked = url
        if (cancelled) {
          URL.revokeObjectURL(url)
          return
        }
        setSrc(url)
      } catch (err) {
        if (cancelled) return
        setError(err instanceof Error ? err.message : String(err))
        setSrc(null)
      }
    }

    void load()
    return () => {
      cancelled = true
      if (revoked) URL.revokeObjectURL(revoked)
    }
  }, [artifactUrl, gateEnabled])

  const openInNewTab = async () => {
    if (!gateEnabled) {
      window.open(artifactUrl, '_blank', 'noopener,noreferrer')
      return
    }
    try {
      const res = await fetch(artifactUrl, {
        headers: authHeaders(true) as Record<string, string>,
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const html = await res.text()
      const blob = new Blob([html], { type: 'text/html; charset=utf-8' })
      const url = URL.createObjectURL(blob)
      window.open(url, '_blank', 'noopener,noreferrer')
      // Delay revoke so the new tab can read the blob.
      window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }

  return (
    <div className="analysis-page-preview">
      {src ? (
        <iframe
          sandbox="allow-scripts"
          src={src}
          height={480}
          title="分析页预览"
        />
      ) : (
        <div className="analysis-page-preview-loading">
          {error ? `预览加载失败：${error}` : '加载分析页…'}
        </div>
      )}
      <button type="button" className="analysis-page-preview-link" onClick={() => void openInNewTab()}>
        在新标签页打开
      </button>
    </div>
  )
}
