import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { authHeaders } from '../controlAuth'
import { useGate } from '../gateContext'
import { CHAT } from '../strings'
import { deleteChannelMedia, isDeletableMediaUrl } from '../api/channelMedia'
import { ImageLightbox } from './ImageLightbox'

export interface ChannelImageProps {
  url: string
  name?: string
  alt?: string
  /** Marks a page/embedded image extracted from a document. */
  document?: boolean
  /** Set false to hide the per-image delete affordance (e.g. inside a bulk manager). */
  deletable?: boolean
}

/**
 * Renders an inbound channel image (e.g. a WeChat photo). The endpoint is
 * conversation-ACL protected and a plain <img src> cannot send the Bearer
 * header, so — like AnalysisPagePreview — we fetch with auth headers and show
 * the bytes through an object URL. Falls back to the raw URL when the
 * control-plane gate is off.
 */
export function ChannelImage({ url, name = '', alt, document = false, deletable = true }: ChannelImageProps) {
  const { gateEnabled } = useGate()
  // Local optimistic previews use blob: object URLs: render directly (no auth
  // fetch / no second object URL). Persisted media goes through the ACL route.
  const isLocal = url.startsWith('blob:')
  const [src, setSrc] = useState<string | null>(isLocal || !gateEnabled ? url : null)
  const [failed, setFailed] = useState(false)
  const [deleted, setDeleted] = useState(false)
  const [deleteFailed, setDeleteFailed] = useState(false)
  const [open, setOpen] = useState(false)
  const canDelete = deletable && !document && !isLocal && isDeletableMediaUrl(url)

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

  const remove = async () => {
    if (!window.confirm(CHAT.deleteAttachmentConfirm)) return
    const ok = await deleteChannelMedia(url, name, gateEnabled)
    if (ok) {
      setDeleted(true)
      setDeleteFailed(false)
    } else {
      setDeleteFailed(true)
    }
  }

  if (deleted) {
    return <span className="channel-image-failed">{CHAT.attachmentDeleted}</span>
  }
  if (failed) {
    return <span className="channel-image-failed">{CHAT.imageLoadFailed}</span>
  }
  if (!src) {
    return <span className="channel-image-loading">{CHAT.imageLoading}</span>
  }
  return (
    <span className="channel-image-wrap">
      <button
        type="button"
        className="channel-image-open"
        title={CHAT.enlargeImage}
        aria-label={CHAT.enlargeImageAria(name || alt || CHAT.imageAlt)}
        onClick={() => setOpen(true)}
      >
        <img src={src} alt={alt ?? CHAT.imageAlt} className="channel-image" loading="lazy" />
      </button>
      {document && name ? (
        <span className="channel-image-caption">{CHAT.documentImageCaption(name)}</span>
      ) : null}
      {canDelete ? (
        <button
          type="button"
          className="channel-attachment-delete"
          aria-label={CHAT.deleteAttachment}
          title={CHAT.deleteAttachment}
          onClick={() => void remove()}
        >
          <X size={12} aria-hidden="true" />
        </button>
      ) : null}
      {deleteFailed ? (
        <span className="channel-attachment-error">{CHAT.deleteAttachmentFailed}</span>
      ) : null}
      <ImageLightbox open={open} title={name || alt || CHAT.imageAlt} onClose={() => setOpen(false)}>
        <img src={src} alt={alt ?? CHAT.imageAlt} className="media-lightbox-image" />
      </ImageLightbox>
    </span>
  )
}
