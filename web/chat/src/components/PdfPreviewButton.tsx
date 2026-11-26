import { useEffect, useState } from 'react'
import { Eye } from 'lucide-react'
import { authHeaders } from '../controlAuth'
import { useGate } from '../gateContext'
import { CHAT } from '../strings'
import { ImageLightbox } from './ImageLightbox'

export interface PdfPreviewButtonProps {
  url: string
  name: string
}

/** Inline trigger + full-screen PDF viewer, fetching with auth like images. */
export function PdfPreviewButton({ url, name }: PdfPreviewButtonProps) {
  const { gateEnabled } = useGate()
  const [open, setOpen] = useState(false)
  const [src, setSrc] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    if (!open) return
    let cancelled = false
    let revoked: string | null = null
    setSrc(null)
    setFailed(false)

    const load = async () => {
      if (!gateEnabled || url.startsWith('blob:')) {
        setSrc(url)
        return
      }
      try {
        const res = await fetch(url, {
          headers: authHeaders(true) as Record<string, string>,
        })
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        const buf = await res.arrayBuffer()
        const blob = new Blob([buf], { type: 'application/pdf' })
        const objectUrl = URL.createObjectURL(blob)
        revoked = objectUrl
        if (cancelled) {
          URL.revokeObjectURL(objectUrl)
          return
        }
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
  }, [open, gateEnabled, url])

  return (
    <>
      <button
        type="button"
        className="channel-file-preview"
        title={CHAT.previewPdf}
        aria-label={CHAT.previewPdfAria(name)}
        onClick={(e) => {
          e.stopPropagation()
          setOpen(true)
        }}
      >
        <Eye size={16} aria-hidden="true" />
      </button>
      <ImageLightbox open={open} title={name} onClose={() => setOpen(false)}>
        {failed ? (
          <p className="settings-muted">{CHAT.pdfPreviewFailed}</p>
        ) : src ? (
          <iframe className="pdf-preview-frame" src={src} title={name} />
        ) : (
          <p className="settings-muted">{CHAT.pdfLoading}</p>
        )}
      </ImageLightbox>
    </>
  )
}
