import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Link, Navigate, Route, Routes } from 'react-router-dom'
import { ChatPage } from './pages/ChatPage'
import './style.css'

function ToolsSettingsPage() {
  return (
    <main style={{ padding: '1.5rem', fontFamily: 'inherit' }}>
      <h1>Tools</h1>
      <p>设置页脚手架（任务 6 实现只读列表）</p>
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
