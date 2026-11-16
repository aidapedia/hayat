import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { AccountService, type GetMeResponse } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { setDisplayZone } from '@/shared/lib/datetime'
import { useTheme } from '@/shared/lib/theme'
import { Button } from '@/shared/ui/button'
import { LoadingState } from '@/shared/ui/bits'
import { useAuth } from './AuthContext'

type Me = { me: GetMeResponse; isAdmin: boolean; reload: () => Promise<void> }

const MeContext = createContext<Me | null>(null)

export function useMe() {
  const ctx = useContext(MeContext)
  if (!ctx) throw new Error('useMe outside SessionGate')
  return ctx
}

/** Renders its children only for a signed-in user, and tells them who that is. */
export function SessionGate({ children }: { children: ReactNode }) {
  const { manager, signIn, signOut } = useAuth()
  const account = useClient(AccountService)
  const { receive } = useTheme()
  const [me, setMe] = useState<GetMeResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    const user = await manager.getUser()
    if (!user || (user.expired && !user.refresh_token)) {
      await signIn()
      return
    }
    try {
      // The theme is the account's, so it arrives with the account. A failure
      // to read it only costs the defaults.
      const [res, look] = await Promise.all([
        account.getMe({}),
        account.getMyAppearance({}).then((r) => r.appearance ?? null, () => null),
      ])
      setDisplayZone(res.user?.zoneinfo)
      receive(look)
      setMe(res)
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
    }
  }, [manager, signIn, account, receive])

  useEffect(() => {
    void load()
  }, [load])

  if (error) {
    return (
      <FullScreen>
        <p className="text-[13px] text-muted-foreground">{error}</p>
        <div className="flex gap-2">
          <Button onClick={() => void load()}>Try again</Button>
          <Button variant="outline" onClick={() => void signOut()}>
            Sign out
          </Button>
        </div>
      </FullScreen>
    )
  }
  if (!me) {
    return (
      <FullScreen>
        <LoadingState label="Signing you in…" />
      </FullScreen>
    )
  }
  return <MeContext.Provider value={{ me, isAdmin: !!me.user?.isAdmin, reload: load }}>{children}</MeContext.Provider>
}

/** Where Hayat sends the browser back after sign-in. */
export function CallbackPage() {
  const { manager } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    manager
      .signinRedirectCallback()
      .then((user) => {
        const returnTo = (user.state as { returnTo?: string } | undefined)?.returnTo
        navigate(returnTo && returnTo !== '/callback' ? returnTo : '/', { replace: true })
      })
      .catch((err) => setError(errorMessage(err)))
  }, [manager, navigate, location.key])

  return (
    <FullScreen>
      {error ? (
        <>
          <p className="text-[13px] text-muted-foreground">Sign-in didn't finish: {error}</p>
          <Button onClick={() => navigate('/', { replace: true })}>Start again</Button>
        </>
      ) : (
        <LoadingState label="Finishing sign-in…" />
      )}
    </FullScreen>
  )
}

function FullScreen({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-4 px-4 text-center">
      <img src="/hayat.png" alt="" className="size-10 rounded-[10px]" />
      {children}
    </div>
  )
}
