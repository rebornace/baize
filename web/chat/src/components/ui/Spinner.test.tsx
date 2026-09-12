import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Spinner } from './Spinner'

describe('Spinner', () => {
  it('renders an accessible spinner', () => {
    const html = renderToStaticMarkup(<Spinner label="加载中" />)
    expect(html).toContain('data-testid="ui-spinner"')
    expect(html).toContain('role="status"')
    expect(html).toContain('加载中')
  })
})
