export interface SSEFrame {
  event: string
  id: string
  data: string
}

export function consumeSSE(buffer: string): { frames: SSEFrame[]; rest: string } {
  const normalized = buffer.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const frames: SSEFrame[] = []
  let start = 0
  while (true) {
    const sep = normalized.indexOf('\n\n', start)
    if (sep < 0) break
    const block = normalized.slice(start, sep)
    start = sep + 2
    const frame = parseBlock(block)
    if (frame) frames.push(frame)
  }
  return { frames, rest: normalized.slice(start) }
}

function parseBlock(block: string): SSEFrame | null {
  let event = ''
  let id = ''
  const dataLines: string[] = []
  for (const line of block.split('\n')) {
    if (line === '' || line.startsWith(':')) continue
    const colon = line.indexOf(':')
    let field: string
    let value: string
    if (colon < 0) {
      field = line
      value = ''
    } else {
      field = line.slice(0, colon)
      value = line.slice(colon + 1)
      if (value.startsWith(' ')) value = value.slice(1)
    }
    switch (field) {
      case 'event':
        event = value
        break
      case 'id':
        id = value
        break
      case 'data':
        dataLines.push(value)
        break
      default:
        break
    }
  }
  if (dataLines.length === 0 && event === '' && id === '') return null
  return { event, id, data: dataLines.join('\n') }
}
