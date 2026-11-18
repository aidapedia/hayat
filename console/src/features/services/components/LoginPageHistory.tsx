import { useState } from 'react'
import { History } from 'lucide-react'
import { toast } from 'sonner'
import { LoginPageService, type LoginPage } from '@/shared/api/gen/hayat/admin/v1/login_page_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { EmptyState, ErrorState, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/shared/ui/sheet'
import { DEFAULT_ACCENT, LAYOUTS } from '../login-page'

/** Every published version, newest first; any of them can come back as the draft. */
export function LoginPageHistory({
  serviceId,
  live,
  open,
  onOpenChange,
  onRestored,
}: {
  serviceId: string
  live: number
  open: boolean
  onOpenChange: (open: boolean) => void
  onRestored: (page: LoginPage) => void
}) {
  const pages = useClient(LoginPageService)
  const { data, error, loading, reload } = useQuery(
    () => (open ? pages.listLoginPageVersions({ serviceId }) : Promise.resolve(undefined)),
    [pages, serviceId, open],
  )
  const [busy, setBusy] = useState(0)

  const restore = async (version: number) => {
    setBusy(version)
    try {
      const res = await pages.restoreLoginPageVersion({ serviceId, version })
      if (res.page) onRestored(res.page)
      toast.success(`Version ${version} is the draft now. Publish it to make it live.`)
      onOpenChange(false)
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(0)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-[440px]">
        <SheetHeader className="border-b">
          <SheetTitle className="text-[15px]">Published versions</SheetTitle>
          <SheetDescription className="text-[12px]">Restoring one copies it into the draft. Nothing changes for people until you publish.</SheetDescription>
        </SheetHeader>
        <div className="flex-1 overflow-y-auto p-4">
          {error ? (
            <ErrorState message={error} onRetry={reload} />
          ) : loading && !data ? (
            <LoadingState />
          ) : !data?.versions.length ? (
            <EmptyState icon={History} title="Nothing published yet">
              People see Hayat's own page until the first publish.
            </EmptyState>
          ) : (
            <ul className="flex flex-col gap-2">
              {data.versions.map((v) => {
                const d = v.design
                const layout = LAYOUTS.find((l) => l.id === (d?.layout || 'centered'))?.label
                return (
                  <li key={v.version} className="flex items-center gap-3 rounded-xl border border-border p-3">
                    <span className="size-8 shrink-0 rounded-md ring-1 ring-border" style={{ background: d?.accent || DEFAULT_ACCENT }} aria-hidden />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2 text-[13px] font-semibold">
                        Version {v.version}
                        {v.version === live && <Pill className="border-success/30 text-success">Live</Pill>}
                      </div>
                      <div className="truncate text-[12px] text-muted-foreground">
                        {layout} · {formatDateTime(v.publishedAt)}
                        {v.publishedBy && ` · ${v.publishedBy.name || v.publishedBy.email}`}
                      </div>
                    </div>
                    <Button variant="outline" size="sm" disabled={busy !== 0} onClick={() => restore(v.version)}>
                      Restore
                    </Button>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
