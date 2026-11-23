import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Badge } from './Badge'

describe('Badge', () => {
  it('renders neutral tone by default', () => {
    const html = renderToStaticMarkup(<Badge>已开启</Badge>)
    expect(html).toContain('data-testid="ui-badge"')
    expect(html).toContain('ui-badge neutral')
    expect(html).toContain('已开启')
  })

  it.each(['success', 'warning', 'danger', 'info'] as const)('supports tone %s', (tone) => {
    expect(renderToStaticMarkup(<Badge tone={tone} />)).toContain(`ui-badge ${tone}`)
  })
})
