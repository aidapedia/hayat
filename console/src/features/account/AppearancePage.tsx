import { Layers } from 'lucide-react'
import { AccountService } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import type { Look } from '@/shared/lib/appearance'
import { DEFAULTS, useTheme } from '@/shared/lib/theme'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import AppearanceEditor from './AppearanceEditor'

const CRUMBS = [{ label: 'Appearance' }]

/**
 * Account → Appearance: the one place a person's theme is edited. It is the
 * account's, so every app that signs them in with Hayat reads the same one
 * (DirectoryService.GetUserAppearance) — tabletsgo included.
 */
export default function AppearancePage() {
  const theme = useTheme()
  const account = useClient(AccountService)

  const save = async (look: Look | null) => {
    try {
      const res = await account.setMyAppearance({ appearance: look ?? undefined })
      theme.receive(res.appearance ?? null)
    } catch (err) {
      throw new Error(errorMessage(err))
    }
  }

  return (
    <>
      <PageHeader title="Appearance" desc="Colors, sizes and type — the same in every app you sign in to with Hayat." crumbs={CRUMBS} />
      <div className="mt-7">
        <AppearanceEditor
          saved={theme.saved}
          defaults={DEFAULTS}
          systemDark={theme.systemDark}
          onPreview={theme.preview}
          onSave={save}
          savedNote="Saved to your account. Every app you sign in to uses it."
          intro={
            <section className="flex items-start gap-4 rounded-xl border border-border bg-card p-5">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/15 text-primary">
                <Layers className="size-4" />
              </span>
              <div className="min-w-0">
                <h3 className="text-[14px] font-semibold">One theme, every app</h3>
                <p className="mt-0.5 text-[12px] text-muted-foreground">
                  Saved with your Hayat account. This console and every app you sign in to with Hayat — tabletsgo included —
                  use it, on every device. There is nothing to set per app; a color you leave on Default is each app’s own.
                </p>
              </div>
            </section>
          }
        />
      </div>
    </>
  )
}
