import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Layers } from 'lucide-react'
import { NavLink, useParams } from 'react-router'
import { AccountService, type AppAppearance, type GetMyAppearanceResponse } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { normalizeLook, type Look } from '@/shared/lib/appearance'
import { CONSOLE_APP, DEFAULTS, consoleLook, useTheme } from '@/shared/lib/theme'
import { cn } from '@/shared/lib/utils'
import { ErrorState, LoadingState, LogoTile } from '@/shared/ui/bits'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import AppearanceEditor from './AppearanceEditor'

const CRUMBS = [{ label: 'Appearance' }]

/**
 * Account → Appearance: the one place a person's themes are edited. The
 * account's theme is the one every app follows (`/account/appearance`); an
 * app may be given a theme of its own instead (`/account/appearance/<app>`,
 * "hayat" being this console). A service reads the one that applies to it
 * through DirectoryService.GetUserAppearance — tabletsgo included.
 */
export default function AppearancePage() {
  const { app: appId } = useParams()
  const theme = useTheme()
  const account = useClient(AccountService)
  const [data, setData] = useState<GetMyAppearanceResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setData(await account.getMyAppearance({}))
      setError(null)
    } catch (err) {
      setError(errorMessage(err))
    }
  }, [account])

  useEffect(() => {
    void load()
  }, [load])

  // Keep what this console shows in step with what was just saved.
  const settle = (next: GetMyAppearanceResponse) => {
    setData(next)
    theme.receive(consoleLook(next))
  }

  // One object per answer, not per render: the editor previews it, and a
  // preview re-renders this page.
  const accountLook = useMemo(() => normalizeLook(data?.appearance ?? null), [data?.appearance])

  if (error) return <ErrorState message={error} onRetry={() => void load()} />
  if (!data) return <LoadingState />

  const app = appId ? data.apps.find((a) => a.app === appId) : undefined
  const consoleFollows = !data.apps.find((a) => a.app === CONSOLE_APP)?.own
  // The whole console previews a theme only when it is the one it would show.
  const previewsHere = app ? app.app === CONSOLE_APP : consoleFollows

  const saveAccount = async (look: Look | null) => {
    try {
      const res = await account.setMyAppearance({ appearance: look ?? undefined })
      settle({ ...data, appearance: res.appearance })
    } catch (err) {
      throw new Error(errorMessage(err))
    }
  }

  const saveApp = async (look: Look | null, own: boolean) => {
    if (!app) return
    try {
      const res = await account.setMyAppAppearance({ app: app.app, own, appearance: look ?? undefined })
      settle({ ...data, apps: data.apps.map((a) => (a.app === app.app && res.app ? res.app : a)) })
    } catch (err) {
      throw new Error(errorMessage(err))
    }
  }

  return (
    <>
      <PageHeader title="Appearance" desc="Colors, sizes and type: one theme every app follows, or a theme of its own for an app." crumbs={CRUMBS} />
      <AppPicker apps={data.apps} active={app?.app} />

      <div className="mt-6">
        {appId && !app ? (
          <ErrorState message="There’s no app by that name." />
        ) : app ? (
          <AppearanceEditor
            key={app.app}
            saved={app.own ? normalizeLook(app.appearance ?? null) : null}
            app={{ name: app.name, own: app.own, account: accountLook }}
            defaults={DEFAULTS}
            systemDark={theme.systemDark}
            onPreview={previewsHere ? theme.preview : undefined}
            onSave={saveApp}
            savedNote={app.own ? `Saved to your account. Only ${app.name} uses this theme.` : `${app.name} follows your account’s theme.`}
          />
        ) : (
          <AppearanceEditor
            key="account"
            saved={accountLook}
            defaults={DEFAULTS}
            systemDark={theme.systemDark}
            onPreview={previewsHere ? theme.preview : undefined}
            onSave={(look) => saveAccount(look)}
            savedNote="Saved to your account. Every app that follows it uses it."
            intro={
              <section className="flex items-start gap-4 rounded-xl border border-border bg-card p-5">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                  <Layers className="size-4" />
                </span>
                <div className="min-w-0">
                  <h3 className="text-[14px] font-semibold">One theme, every app</h3>
                  <p className="mt-0.5 text-[12px] text-muted-foreground">
                    Saved with your Hayat account and used on every device by every app you sign in to with Hayat — this console
                    and tabletsgo included — unless you give an app a theme of its own above. A color you leave on Default is
                    each app’s own.
                  </p>
                </div>
              </section>
            }
          />
        )}
      </div>
    </>
  )
}

/** Which theme is being edited: the account's, or one app's. Each is an address. */
function AppPicker({ apps, active }: { apps: AppAppearance[]; active?: string }) {
  const item = (to: string, on: boolean, mark: ReactNode, name: string, status: string) => (
    <NavLink
      key={to}
      to={to}
      end
      aria-current={on ? 'page' : undefined}
      className={cn(
        'flex min-w-0 items-center gap-2.5 rounded-xl border px-3 py-2.5 transition-colors',
        on ? 'border-primary bg-primary/5 ring-1 ring-primary' : 'border-border bg-card hover:border-input',
      )}
    >
      {mark}
      <span className="min-w-0">
        <span className="block truncate text-[13px] font-semibold">{name}</span>
        <span className="block truncate text-[11px] text-muted-foreground">{status}</span>
      </span>
    </NavLink>
  )
  return (
    <nav aria-label="Theme for" className="mt-6 grid grid-cols-[repeat(auto-fill,minmax(12rem,1fr))] gap-2">
      {item(
        '/account/appearance',
        !active,
        <span className="flex size-8 shrink-0 items-center justify-center rounded-[10px] bg-primary/10 text-primary">
          <Layers className="size-4" />
        </span>,
        'Every app',
        'Your account’s theme',
      )}
      {apps.map((a) =>
        item(
          `/account/appearance/${a.app}`,
          active === a.app,
          <LogoTile name={a.name} src={a.logoUrl || undefined} size={32} inverted={a.app === CONSOLE_APP} />,
          a.name,
          a.own ? 'Its own theme' : 'Follows your account',
        ),
      )}
    </nav>
  )
}
