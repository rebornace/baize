import { useEffect, useState } from 'react'
import { authHeaders } from '../controlAuth'
import { useGate } from '../gateContext'

export interface ChannelImageProps {
  url: string
  alt?: string
}

/**
 * Renders an inbound channel image (e.g. a WeChat photo). The endpoint is
 * conversation-ACL protected and a plain <img src> cannot send the Bearer
 * header, so — like AnalysisPagePreview — we fetch with auth headers and show
 * the bytes through an object URL. Falls back to the raw URL when the
 * control-plane gate is off.
 */
export function ChannelImage({ url, alt }: ChannelImageProps) {
  const { gateEnabled } = useGate()
  // Local optimistic previews use blob: object URLs: render directly (no auth
  // fetch / no second object URL). Persisted media goes through the ACL route.
  const isLocal = url.startsWith('blob:')
  const [src, setSrc] = useState<string | null>(isLocal || !gateEnabled ? url : null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let revoked: string | null = null
    let cancelled = false

    const load = async () => {
      if (isLocal || !gateEnabled) {
        setSrc(url)
        return
      }
      try {
        const res = await fetch(url, {
          headers: authHeaders(true) as Record<string, string>,
        })
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const blob = await res.blob()
        const objectUrl = URL.createObjectURL(blob)
        revoked = objectUrl
        if (cancelled) {
          URL.revokeObjectURL(objectUrl)
          return
        }
        setFailed(false)
        setSrc(objectUrl)
      } catch {
        if (!cancelled) setFailed(true)
      }
    }

    void load()
    return () => {
      cancelled = true
      if (revoked) URL.revokeObjectURL(revoked)
    }
  }, [url, gateEnabled, isLocal])

  if (failed) {
    return <span className="channel-image-failed">（图片加载失败）</span>
  }
  if (!src) {
    return <span className="channel-image-loading">（图片加载中…）</span>
  }
  return <img src={src} alt={alt ?? '图片'} className="channel-image" loading="lazy" />
}
