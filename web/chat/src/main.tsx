import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Link, Navigate, Route, Routes } from 'react-router-dom'
import './style.css'

function ChatPage() {
  return (
    <main style={{ padding: '1.5rem', fontFamily: 'inherit' }}>
      <h1>Baize Chat</h1>
      <p>对话页脚手架（任务 5 实现完整 UI）</p>
      <p>
        <Link to="/settings/tools">设置 · Tools</Link>
      </p>
    </main>
  )
}

function ToolsSettingsPage() {
  return (
    <main style={{ padding: '1.5rem', fontFamily: 'inherit' }}>
      <h1>Tools</h1>
      <p>设置页脚手架（任务 5 实现只读列表）</p>
      <p>
        <Link to="/">返回对话</Link>
      </p>
    </main>
  )
}

createRoot(document.getElementById('app')!).render(
  <StrictMode>
    <BrowserRouter basename="/ui">
      <Routes>
        <Route path="/" element={<ChatPage />} />
        <Route path="/settings/tools" element={<ToolsSettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)
