import { lazy, Suspense, useEffect, useState, type ReactNode } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useParams } from 'react-router'
import { AuthProvider, CallbackPage, SessionGate, useMe } from '@/features/auth'
import { loadConfig, type AppConfig } from '@/shared/lib/config'
import { ThemeProvider } from '@/shared/lib/theme'
import { LoadingState } from '@/shared/ui/bits'
import { Toaster } from '@/shared/ui/sonner'
import { TooltipProvider } from '@/shared/ui/tooltip'
import HomeLayout from './layouts/HomeLayout'

const OverviewPage = lazy(() => import('@/features/overview/OverviewPage'))
const UsersPage = lazy(() => import('@/features/users/UsersPage'))
const UserDetailPage = lazy(() => import('@/features/users/UserDetailPage'))
const TenantsPage = lazy(() => import('@/features/tenants/TenantsPage'))
const TenantDetailPage = lazy(() => import('@/features/tenants/TenantDetailPage'))
const ClientsPage = lazy(() => import('@/features/clients/ClientsPage'))
const ClientDetailPage = lazy(() => import('@/features/clients/ClientDetailPage'))
const KeysPage = lazy(() => import('@/features/keys/KeysPage'))
const AuditPage = lazy(() => import('@/features/audit/AuditPage'))
const AccountPage = lazy(() => import('@/features/account/AccountPage'))
const RolesPage = lazy(() => import('@/features/roles/RolesPage'))
const ServicesPage = lazy(() => import('@/features/services/ServicesPage'))
const ServiceDetailPage = lazy(() => import('@/features/services/ServiceDetailPage'))
const ResourceTreePage = lazy(() => import('@/features/resource-tree/ResourceTreePage'))

/** The tree used to live on the tenant page; its old addresses still land on the same node. */
function ToResourceTree() {
  const { id = '', nodeId } = useParams()
  return <Navigate to={nodeId ? `/resource-tree/${id}/${nodeId}` : `/resource-tree/${id}`} replace />
}

/** Admin pages; anyone else is sent to their account. The server enforces it too. */
function AdminOnly({ children }: { children: ReactNode }) {
  const { isAdmin } = useMe()
  return isAdmin ? children : <Navigate to="/account" replace />
}

function NotFound() {
  return <p className="text-[13px] text-muted-foreground">There's nothing at this address.</p>
}

export default function App() {
  const [config, setConfig] = useState<AppConfig | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    loadConfig().then(setConfig, (err: Error) => setError(err.message))
  }, [])

  return (
    <ThemeProvider>
      <TooltipProvider delayDuration={300}>
        {error ? (
          <div className="flex h-full items-center justify-center px-4 text-center text-[13px] text-muted-foreground">{error}</div>
        ) : !config ? (
          <div className="flex h-full items-center justify-center">
            <LoadingState />
          </div>
        ) : (
          <AuthProvider config={config}>
            <BrowserRouter>
              <Routes>
                <Route path="/callback" element={<CallbackPage />} />
                <Route
                  element={
                    <SessionGate>
                      <HomeLayout />
                    </SessionGate>
                  }
                >
                  {[
                    ['/', <OverviewPage key="o" />],
                    ['/users', <UsersPage key="u" />],
                    ['/users/:id', <UserDetailPage key="ud" />],
                    ['/tenants', <TenantsPage key="t" />],
                    ['/tenants/:id', <TenantDetailPage key="td" />],
                    ['/tenants/:id/resources', <ToResourceTree key="tr" />],
                    ['/tenants/:id/resources/:nodeId', <ToResourceTree key="trn" />],
                    // One route, so moving between nodes keeps the page (and the open branches) mounted.
                    ['/resource-tree/:tenantId?/:nodeId?', <ResourceTreePage key="rt" />],
                    ['/roles', <RolesPage key="r" />],
                    ['/services', <ServicesPage key="s" />],
                    ['/services/:id', <ServiceDetailPage key="sd" />],
                    ['/clients', <ClientsPage key="c" />],
                    ['/clients/:id', <ClientDetailPage key="cd" />],
                    ['/keys', <KeysPage key="k" />],
                    ['/audit', <AuditPage key="a" />],
                  ].map(([path, el]) => (
                    <Route
                      key={path as string}
                      path={path as string}
                      element={
                        <AdminOnly>
                          <Suspense fallback={<LoadingState />}>{el}</Suspense>
                        </AdminOnly>
                      }
                    />
                  ))}
                  <Route
                    path="/account"
                    element={
                      <Suspense fallback={<LoadingState />}>
                        <AccountPage />
                      </Suspense>
                    }
                  />
                  <Route path="*" element={<NotFound />} />
                </Route>
              </Routes>
            </BrowserRouter>
          </AuthProvider>
        )}
        <Toaster position="bottom-right" />
      </TooltipProvider>
    </ThemeProvider>
  )
}
