import { MarkdownText } from './MarkdownText'
import { ChannelImage } from './ChannelImage'
import { splitChannelImages } from '../channelImages'

/**
 * Renders a user (or system_note) bubble, splitting out any inbound channel
 * image references (WeChat photos stored at /v0/channels/media/...) so they
 * render as images rather than literal markdown text.
 */
export function UserBubble({ content }: { content: string }) {
  const { text, images } = splitChannelImages(content)
  return (
    <>
      {text ? <MarkdownText text={text} plain /> : null}
      {images.length > 0 && (
        <div className="channel-images">
          {images.map((url) => (
            <ChannelImage key={url} url={url} />
          ))}
        </div>
      )}
    </>
  )
}
