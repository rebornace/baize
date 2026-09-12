import { MarkdownText } from './MarkdownText'
import { ChannelImage } from './ChannelImage'
import { ChannelFileLink } from './ChannelFileLink'
import { splitChannelAttachments } from '../channelImages'

/**
 * Renders a user (or system_note) bubble, splitting out inbound channel
 * attachment references (WeChat photos stored at /v0/channels/media/...) so
 * images render inline and files render as download links rather than literal
 * marker text.
 */
export function UserBubble({ content }: { content: string }) {
  const { text, images, files } = splitChannelAttachments(content)
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
      {files.length > 0 && (
        <div className="channel-files">
          {files.map((f) => (
            <ChannelFileLink key={f.url} name={f.name} url={f.url} />
          ))}
        </div>
      )}
    </>
  )
}
