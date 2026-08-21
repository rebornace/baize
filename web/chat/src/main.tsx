import { StrictMode, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { useGate } from './gateContext'
import { ChatPage } from './pages/ChatPage'
import { ComingSoon } from './pages/ComingSoon'
import { GateRoot } from './pages/GateRoot'
import { IdentitiesSettings } from './pages/IdentitiesSettings'
import { SettingsLayout } from './pages/SettingsLayout'
import { SkillsSettings } from './pages/SkillsSettings'
import { ToolsSettings } from './pages/ToolsSettings'
import './style.css'

function AdminOnly({ children }: { children: ReactNode }) {
  const { role } = useGate()
  if (role !== 'admin') return <Navigate to="/settings/identities" replace />
  return children
}

function SettingsIndex() {
  const { role } = useGate()
  return <Navigate to={role === 'admin' ? 'tools' : 'identities'} replace />
}

createRoot(document.getElementById('app')!).render(
  <StrictMode>
    <BrowserRouter basename="/ui">
      <GateRoot>
        <Routes>
          <Route path="/" element={<ChatPage />} />
          <Route path="/settings" element={<SettingsLayout />}>
            <Route index element={<SettingsIndex />} />
            <Route
              path="tools"
              element={
                <AdminOnly>
                  <ToolsSettings />
                </AdminOnly>
              }
            />
            <Route
              path="skills"
              element={
                <AdminOnly>
                  <SkillsSettings />
                </AdminOnly>
              }
            />
            <Route path="identities" element={<IdentitiesSettings />} />
            <Route
              path="mcp"
              element={
                <AdminOnly>
                  <ComingSoon title="MCP" />
                </AdminOnly>
              }
            />
            <Route
              path="plugins"
              element={
                <AdminOnly>
                  <ComingSoon title="插件" />
                </AdminOnly>
              }
            />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </GateRoot>
    </BrowserRouter>
  </StrictMode>,
)
