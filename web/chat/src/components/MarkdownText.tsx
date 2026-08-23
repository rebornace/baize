import type { ReactNode } from 'react'

function inlineMarkdown(text: string): ReactNode[] {
  const nodes: ReactNode[] = []
  const re = /(`[^`]+`|\*\*[^*]+\*\*|\*[^*]+\*|\[([^\]]+)\]\(([^)]+)\))/g
  let last = 0
  let m: RegExpExecArray | null
  let key = 0
  while ((m = re.exec(text)) !== null) {
    if (m.index > last) {
      nodes.push(text.slice(last, m.index))
    }
    const raw = m[0]
    if (raw.startsWith('`')) {
      nodes.push(
        <code key={key++} className="md-code">
          {raw.slice(1, -1)}
        </code>,
      )
    } else if (raw.startsWith('**')) {
      nodes.push(<strong key={key++}>{raw.slice(2, -2)}</strong>)
    } else if (raw.startsWith('*')) {
      nodes.push(<em key={key++}>{raw.slice(1, -1)}</em>)
    } else if (m[2] && m[3]) {
      const href = m[3]
      const safe = href.startsWith('http://') || href.startsWith('https://') || href.startsWith('/')
      nodes.push(
        safe ? (
          <a key={key++} href={href} target="_blank" rel="noreferrer" className="md-link">
            {m[2]}
          </a>
        ) : (
          m[2]
        ),
      )
    }
    last = m.index + raw.length
  }
  if (last < text.length) {
    nodes.push(text.slice(last))
  }
  return nodes
}

function renderBlock(block: string, key: number): ReactNode {
  const trimmed = block.trimEnd()
  if (!trimmed.trim()) return null

  const fence = trimmed.match(/^```(\w*)\n?([\s\S]*?)```$/)
  if (fence) {
    return (
      <pre key={key} className="md-pre">
        <code>{fence[2].replace(/\n$/, '')}</code>
      </pre>
    )
  }

  if (/^#{1,3}\s/.test(trimmed)) {
    const level = trimmed.match(/^(#{1,3})\s/)?.[1].length ?? 1
    const text = trimmed.replace(/^#{1,3}\s+/, '')
    if (level === 1) {
      return (
        <h3 key={key} className="md-heading">
          {inlineMarkdown(text)}
        </h3>
      )
    }
    if (level === 2) {
      return (
        <h4 key={key} className="md-heading">
          {inlineMarkdown(text)}
        </h4>
      )
    }
    return (
      <h5 key={key} className="md-heading">
        {inlineMarkdown(text)}
      </h5>
    )
  }

  const lines = trimmed.split('\n')
  if (lines.every((l) => /^\s*[-*]\s+/.test(l) || l.trim() === '')) {
    return (
      <ul key={key} className="md-list">
        {lines
          .filter((l) => l.trim())
          .map((l, i) => (
            <li key={i}>{inlineMarkdown(l.replace(/^\s*[-*]\s+/, ''))}</li>
          ))}
      </ul>
    )
  }

  if (lines.every((l) => /^\s*\d+\.\s+/.test(l) || l.trim() === '')) {
    return (
      <ol key={key} className="md-list">
        {lines
          .filter((l) => l.trim())
          .map((l, i) => (
            <li key={i}>{inlineMarkdown(l.replace(/^\s*\d+\.\s+/, ''))}</li>
          ))}
      </ol>
    )
  }

  return (
    <p key={key} className="md-p">
      {lines.map((line, i) => (
        <span key={i}>
          {i > 0 && <br />}
          {inlineMarkdown(line)}
        </span>
      ))}
    </p>
  )
}

export function MarkdownText({ text, plain }: { text: string; plain?: boolean }) {
  if (plain) {
    return <div className="md-body md-plain">{text}</div>
  }
  const parts = text.split(/(```[\s\S]*?```)/g)
  const blocks: ReactNode[] = []
  let key = 0
  for (const part of parts) {
    if (!part) continue
    if (part.startsWith('```')) {
      const node = renderBlock(part, key++)
      if (node) blocks.push(node)
      continue
    }
    for (const para of part.split(/\n{2,}/)) {
      const node = renderBlock(para, key++)
      if (node) blocks.push(node)
    }
  }
  return <div className="md-body">{blocks}</div>
}
