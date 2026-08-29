import { StrictMode, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { useGate } from './gateContext'
import { ChatPage } from './pages/ChatPage'
import { McpSettings } from './pages/McpSettings'
import { OpenApiSettings } from './pages/OpenApiSettings'
import { PluginSettings } from './pages/PluginSettings'
import { InboxSettings } from './pages/InboxSettings'
import { WebhookSettings } from './pages/WebhookSettings'
import { GateRoot } from './pages/GateRoot'
import { IdentitiesSettings } from './pages/IdentitiesSettings'
import { SettingsLayout } from './pages/SettingsLayout'
import { SkillsSettings } from './pages/SkillsSettings'
import { StorageSettings } from './pages/StorageSettings'
import { ToolsSettings } from './pages/ToolsSettings'
import { WeixinChannelSettings } from './pages/WeixinChannelSettings'
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
              path="openapi"
              element={
                <AdminOnly>
                  <OpenApiSettings />
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
                  <McpSettings />
                </AdminOnly>
              }
            />
            <Route
              path="plugins"
              element={
                <AdminOnly>
                  <PluginSettings />
                </AdminOnly>
              }
            />
            <Route
              path="webhooks"
              element={
                <AdminOnly>
                  <WebhookSettings />
                </AdminOnly>
              }
            />
            <Route
              path="inbox"
              element={
                <AdminOnly>
                  <InboxSettings />
                </AdminOnly>
              }
            />
            <Route
              path="channels/weixin"
              element={
                <AdminOnly>
                  <WeixinChannelSettings />
                </AdminOnly>
              }
            />
            <Route
              path="storage"
              element={
                <AdminOnly>
                  <StorageSettings />
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
