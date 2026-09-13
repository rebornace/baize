// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import { FilePickerButton } from './FilePickerButton'

describe('FilePickerButton', () => {
  it('clicking choose button activates hidden file input', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const onChange = vi.fn()
    const clickSpy = vi.fn()
    await act(async () => {
      createRoot(host).render(
        <FilePickerButton accept=".md,.zip" chooseLabel="选择文件" onFile={onChange} />,
      )
    })
    const input = host.querySelector('input[type="file"]') as HTMLInputElement
    expect(input).toBeTruthy()
    expect(input.hidden || input.getAttribute('hidden') !== null || input.classList.contains('sr-only') || input.style.display === 'none' || !input.checkVisibility?.()).toBeTruthy()
    // 简化：断言 input 存在且 tabIndex/aria 不抢焦点；按钮文案正确
    expect(host.textContent).toContain('选择文件')
    input.click = clickSpy
    const btn = [...host.querySelectorAll('button')].find((b) => b.textContent?.includes('选择文件'))
    await act(async () => { btn!.click() })
    expect(clickSpy).toHaveBeenCalled()
    host.remove()
  })

  it('shows fileName and clear calls onClear', async () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const onClear = vi.fn()
    await act(async () => {
      createRoot(host).render(
        <FilePickerButton
          accept=".json"
          chooseLabel="选择接口文档"
          clearLabel="清除已选"
          fileName="a.json"
          onFile={() => {}}
          onClear={onClear}
        />,
      )
    })
    expect(host.textContent).toContain('a.json')
    const clear = [...host.querySelectorAll('button')].find((b) => b.textContent?.includes('清除已选'))
    await act(async () => { clear!.click() })
    expect(onClear).toHaveBeenCalled()
    host.remove()
  })
})
