import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from 'react'
import { UserManager, type User } from 'oidc-client-ts'
import type { Transport } from '@connectrpc/connect'
import { createTransport, TransportContext } from '@/shared/api/client'
import type { AppConfig } from '@/shared/lib/config'
import { StepUpDialog, reauthURL } from './StepUpDialog'

/**
 * The console signs in like any other Hayat client: authorization code + PKCE
 * against the Hayat issuer, as the built-in public client. Tokens live in
 * sessionStorage (per tab); a tab without one is sent through Hayat, which
 * signs it in without a prompt while the Hayat session lasts.
 */
type Auth = {
  config: AppConfig
  manager: UserManager
  signIn: (returnTo?: string) => Promise<void>
  signOut: () => Promise<void>
  /** Sends the person to confirm it's them on Hayat, then back to this page. */
  confirmIdentity: () => void
}

const AuthContext = createContext<Auth | null>(null)

export function AuthProvider({ config, children }: { config: AppConfig; children: ReactNode }) {
  const manager = useMemo(
    () =>
      new UserManager({
        authority: config.issuer,
        client_id: config.clientId,
        redirect_uri: `${window.location.origin}/callback`,
        post_logout_redirect_uri: `${window.location.origin}/`,
        response_type: 'code',
        scope: 'openid profile email offline_access',
        // Renewal is done on demand below. Letting the library renew on its own
        // timer as well could present one refresh token twice, and Hayat
        // revokes a refresh token family the moment it sees a reuse.
        automaticSilentRenew: false,
        loadUserInfo: false,
      }),
    [config],
  )

  const refreshing = useRef<Promise<User | null> | null>(null)
  const redirecting = useRef(false)

  const signIn = useCallback(
    async (returnTo?: string) => {
      if (redirecting.current) return
      redirecting.current = true
      await manager.signinRedirect({ state: { returnTo: returnTo ?? window.location.pathname + window.location.search } })
    },
    [manager],
  )

  const signOut = useCallback(async () => {
    const user = await manager.getUser()
    redirecting.current = true
    await manager.signoutRedirect({ id_token_hint: user?.id_token })
  }, [manager])

  const getToken = useCallback(async () => {
    const user = await manager.getUser()
    if (!user) return null
    if ((user.expires_in ?? 0) > 30) return user.access_token
    if (!user.refresh_token) return null
    refreshing.current ??= manager.signinSilent().finally(() => {
      refreshing.current = null
    })
    try {
      return (await refreshing.current)?.access_token ?? null
    } catch {
      return null
    }
  }, [manager])

  const [stepUp, setStepUp] = useState(false)
  const transport: Transport = useMemo(
    () =>
      createTransport(
        getToken,
        () => {
          void manager.removeUser().then(() => signIn())
        },
        () => setStepUp(true),
      ),
    [getToken, manager, signIn],
  )

  const confirmIdentity = useCallback(() => window.location.assign(reauthURL(config.issuer)), [config.issuer])
  const value = useMemo(() => ({ config, manager, signIn, signOut, confirmIdentity }), [config, manager, signIn, signOut, confirmIdentity])
  return (
    <AuthContext.Provider value={value}>
      <TransportContext.Provider value={transport}>
        {children}
        <StepUpDialog open={stepUp} onOpenChange={setStepUp} onConfirm={confirmIdentity} />
      </TransportContext.Provider>
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth outside AuthProvider')
  return ctx
}
