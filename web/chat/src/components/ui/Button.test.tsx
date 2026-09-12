import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Button } from './Button'

describe('Button', () => {
  it('renders a primary button with testid and children', () => {
    const html = renderToStaticMarkup(<Button variant="primary">保存</Button>)
    expect(html).toContain('data-testid="ui-button"')
    expect(html).toContain('btn primary')
    expect(html).toContain('保存')
  })

  it('supports secondary/ghost/danger variants and sm size', () => {
    expect(renderToStaticMarkup(<Button variant="secondary" />)).toContain('btn secondary')
    expect(renderToStaticMarkup(<Button variant="ghost" />)).toContain('btn ghost')
    expect(renderToStaticMarkup(<Button variant="danger" />)).toContain('btn danger')
    expect(renderToStaticMarkup(<Button size="sm" />)).toContain('btn sm')
  })

  it('renders native type and disabled, and merges extra class', () => {
    const html = renderToStaticMarkup(
      <Button type="submit" disabled className="extra" />,
    )
    expect(html).toContain('type="submit"')
    expect(html).toContain('disabled=""')
    expect(html).toContain('extra')
  })
})
