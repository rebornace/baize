import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { ChatPage } from './pages/ChatPage'
import { ComingSoon } from './pages/ComingSoon'
import { IdentitiesSettings } from './pages/IdentitiesSettings'
import { SettingsLayout } from './pages/SettingsLayout'
import { ToolsSettings } from './pages/ToolsSettings'
import './style.css'

createRoot(document.getElementById('app')!).render(
  <StrictMode>
    <BrowserRouter basename="/ui">
      <Routes>
        <Route path="/" element={<ChatPage />} />
        <Route path="/settings" element={<SettingsLayout />}>
          <Route index element={<Navigate to="tools" replace />} />
          <Route path="tools" element={<ToolsSettings />} />
          <Route path="identities" element={<IdentitiesSettings />} />
          <Route path="mcp" element={<ComingSoon title="MCP" />} />
          <Route path="plugins" element={<ComingSoon title="插件" />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)