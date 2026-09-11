import { StrictMode, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { useGate } from './gateContext'
import { ChatPage } from './pages/ChatPage'
import { McpExportSettings } from './pages/McpExportSettings'
import { McpSettings } from './pages/McpSettings'
import { ModelSettings } from './pages/ModelSettings'
import { OpenApiSettings } from './pages/OpenApiSettings'
import { PluginSettings } from './pages/PluginSettings'
import { RuntimeSettings } from './pages/RuntimeSettings'
import { InboxSettings } from './pages/InboxSettings'
import { WebhookSettings } from './pages/WebhookSettings'
import { GateRoot } from './pages/GateRoot'
import { IdentitiesSettings } from './pages/IdentitiesSettings'
import { SettingsHome } from './pages/SettingsHome'
import { SettingsLayout } from './pages/SettingsLayout'
import { SkillsSettings } from './pages/SkillsSettings'
import { StorageSettings } from './pages/StorageSettings'
import { ToolsSettings } from './pages/ToolsSettings'
import { WeixinChannelSettings } from './pages/WeixinChannelSettings'
import './styles/tokens.css'
import './styles/base.css'
import './styles/components.css'
import './styles/layout.css'
import './styles/settings.css'
import './style.css'
import { initSidebarWidth } from './sidebarResize'

initSidebarWidth()

function AdminOnly({ children }: { children: ReactNode }) {
  const { role } = useGate()
  if (role !== 'admin') return <Navigate to="/settings/identities" replace />
  return children
}

createRoot(document.getElementById('app')!).render(
  <StrictMode>
    <BrowserRouter basename="/ui">
      <GateRoot>
        <Routes>
          <Route path="/" element={<ChatPage />} />
          <Route path="/settings" element={<SettingsLayout />}>
            <Route index element={<SettingsHome />} />
            <Route path="tools" element={<ToolsSettings />} />
            <Route
              path="openapi"
              element={
                <AdminOnly>
                  <OpenApiSettings />
                </AdminOnly>
              }
            />
            <Route path="skills" element={<SkillsSettings />} />
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
              path="mcp-export"
              element={
                <AdminOnly>
                  <McpExportSettings />
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
            <Route path="channels/weixin" element={<WeixinChannelSettings />} />
            <Route path="models" element={<ModelSettings />} />
            <Route
              path="storage"
              element={
                <AdminOnly>
                  <StorageSettings />
                </AdminOnly>
              }
            />
            <Route path="runtime" element={<RuntimeSettings />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </GateRoot>
    </BrowserRouter>
  </StrictMode>,
)
