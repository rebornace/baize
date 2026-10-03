/** 多行 KEY=VALUE 文本 <-> 对象；空文本得到空对象，非法行返回中文错误。 */
export function parseKeyValueLines(text: string): { ok: true; value: Record<string, string> } | { ok: false; message: string } {
  const trimmed = text.trim()
  if (trimmed === '') return { ok: true, value: {} }
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const row = line.trim()
    if (row === '') continue
    const eq = row.indexOf('=')
    if (eq <= 0) return { ok: false, message: `无效键值行：${row}` }
    const key = row.slice(0, eq).trim()
    const value = row.slice(eq + 1).trim()
    if (!key) return { ok: false, message: `无效键值行：${row}` }
    out[key] = value
  }
  return { ok: true, value: out }
}

export function formatKeyValueMap(map: Record<string, string> | undefined): string {
  if (!map) return ''
  return Object.entries(map).map(([k, v]) => `${k}=${v}`).join('\n')
}

export function formatStringList(list: string[] | undefined): string {
  if (!list || list.length === 0) return ''
  return list.join('\n')
}

/** 单行按空白拆，多行按每行一个拆（兼容旧 args 输入习惯）。 */
export function parseArgsText(text: string): string[] {
  const trimmed = text.trim()
  if (trimmed === '') return []
  if (trimmed.includes('\n')) {
    return trimmed.split('\n').map((s) => s.trim()).filter((s) => s !== '')
  }
  return trimmed.split(/\s+/).filter((s) => s !== '')
}

export function parseLineList(text: string): string[] {
  return text.split('\n').map((s) => s.trim()).filter((s) => s !== '')
}
