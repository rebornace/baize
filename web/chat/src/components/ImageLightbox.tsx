import { useEffect, type ReactNode } from 'react'
import { X } from 'lucide-react'
import { CHAT } from '../strings'

export interface ImageLightboxProps {
  open: boolean
  title: string
  onClose: () => void
  children: ReactNode
}

/** Full-screen overlay used by image and PDF previews. */
export function ImageLightbox({ open, title, onClose, children }: ImageLightboxProps) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null
  return (
    <div
      className="media-lightbox"
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onClick={onClose}
    >
      <button
        type="button"
        className="media-lightbox-close"
        aria-label={CHAT.closePreview}
        title={CHAT.closePreview}
        onClick={(e) => {
          e.stopPropagation()
          onClose()
        }}
      >
        <X size={18} aria-hidden="true" />
      </button>
      <div className="media-lightbox-body" onClick={(e) => e.stopPropagation()}>
        {children}
      </div>
    </div>
  )
}
