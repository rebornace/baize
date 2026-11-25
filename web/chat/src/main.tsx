import { StrictMode, Suspense, lazy, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
import { useGate } from './gateContext'
import { GateRoot } from './pages/GateRoot'
import { SettingsLayout } from './pages/SettingsLayout'
import { Spinner } from './components/ui'
import { LocaleProvider } from './locale/LocaleContext'
import { FeatureSearchProvider } from './components/FeatureSearch'
import './styles/tokens.css'
import './styles/base.css'
import './styles/components.css'
import './styles/layout.css'
import './styles/settings.css'
import './styles/chat.css'
import './styles/feature-search.css'
import { initSidebarWidth } from './sidebarResize'
import { installApiOrigin } from './api/origin'
import { routerBasename } from './routerBasename'

// Settings pages are split into their own chunks so the initial chat bundle
// does not carry every admin screen.
const ChatPage = lazy(() => import('./pages/ChatPage').then((m) => ({ default: m.ChatPage })))
const McpExportSettings = lazy(() =>
  import('./pages/McpExportSettings').then((m) => ({ default: m.McpExportSettings })),
)
const McpSettings = lazy(() =>
  import('./pages/McpSettings').then((m) => ({ default: m.McpSettings })),
)
const ModelSettings = lazy(() =>
  import('./pages/ModelSettings').then((m) => ({ default: m.ModelSettings })),
)
const OpenApiSettings = lazy(() =>
  import('./pages/OpenApiSettings').then((m) => ({ default: m.OpenApiSettings })),
)
const PluginSettings = lazy(() =>
  import('./pages/PluginSettings').then((m) => ({ default: m.PluginSettings })),
)
const RuntimeSettings = lazy(() =>
  import('./pages/RuntimeSettings').then((m) => ({ default: m.RuntimeSettings })),
)
const ToolMatchingSettings = lazy(() =>
  import('./pages/ToolMatchingSettings').then((m) => ({ default: m.ToolMatchingSettings })),
)
const InboxSettings = lazy(() =>
  import('./pages/InboxSettings').then((m) => ({ default: m.InboxSettings })),
)
const WebhookSettings = lazy(() =>
  import('./pages/WebhookSettings').then((m) => ({ default: m.WebhookSettings })),
)
const IdentitiesSettings = lazy(() =>
  import('./pages/IdentitiesSettings').then((m) => ({ default: m.IdentitiesSettings })),
)
const MemorySettings = lazy(() =>
  import('./pages/MemorySettings').then((m) => ({ default: m.MemorySettings })),
)
const ScheduleSettings = lazy(() =>
  import('./pages/ScheduleSettings').then((m) => ({ default: m.ScheduleSettings })),
)
const SettingsHome = lazy(() =>
  import('./pages/SettingsHome').then((m) => ({ default: m.SettingsHome })),
)
const SkillsSettings = lazy(() =>
  import('./pages/SkillsSettings').then((m) => ({ default: m.SkillsSettings })),
)
const StorageSettings = lazy(() =>
  import('./pages/StorageSettings').then((m) => ({ default: m.StorageSettings })),
)
const MetricsSettings = lazy(() =>
  import('./pages/MetricsSettings').then((m) => ({ default: m.MetricsSettings })),
)
const EvalSettings = lazy(() =>
  import('./pages/EvalSettings').then((m) => ({ default: m.EvalSettings })),
)
const ToolsSettings = lazy(() =>
  import('./pages/ToolsSettings').then((m) => ({ default: m.ToolsSettings })),
)
const WeixinChannelSettings = lazy(() =>
  import('./pages/WeixinChannelSettings').then((m) => ({ default: m.WeixinChannelSettings })),
)
const ExternalLoginSettings = lazy(() =>
  import('./pages/ExternalLoginSettings').then((m) => ({ default: m.ExternalLoginSettings })),
)

installApiOrigin()
initSidebarWidth()

function AdminOnly({ children }: { children: ReactNode }) {
  const { role } = useGate()
  if (role !== 'admin') return <Navigate to="/settings/identities" replace />
  return children
}

createRoot(document.getElementById('app')!).render(
  <StrictMode>
    <BrowserRouter basename={routerBasename() || undefined}>
      <LocaleProvider>
      <GateRoot>
        <FeatureSearchProvider>
          <Suspense
            fallback={
              <div className="route-loading">
                <Spinner />
              </div>
            }
          >
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
                <Route path="memory" element={<MemorySettings />} />
                <Route path="schedules" element={<ScheduleSettings />} />
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
                  path="external-login"
                  element={
                    <AdminOnly>
                      <ExternalLoginSettings />
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
                <Route
                  path="metrics"
                  element={
                    <AdminOnly>
                      <MetricsSettings />
                    </AdminOnly>
                  }
                />
                <Route
                  path="eval"
                  element={
                    <AdminOnly>
                      <EvalSettings />
                    </AdminOnly>
                  }
                />
                <Route path="runtime" element={<RuntimeSettings />} />
                <Route path="tool-matching" element={<ToolMatchingSettings />} />
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        </FeatureSearchProvider>
      </GateRoot>
      </LocaleProvider>
    </BrowserRouter>
  </StrictMode>,
)
