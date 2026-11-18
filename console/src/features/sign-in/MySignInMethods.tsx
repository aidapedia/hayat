import { useEffect, useState } from 'react'
import { KeyRound } from 'lucide-react'
import { toast } from 'sonner'
import { SignInService, type LinkedAccount } from '@/shared/api/gen/hayat/admin/v1/sign_in_pb'
import { useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate } from '@/shared/lib/datetime'
import { ErrorState, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { ProviderIcon } from './ProviderIcon'

/**
 * The caller's ways in: their password, and the Google, GitHub or company
 * accounts linked to them. Linking opens the provider in this tab and comes
 * back here (?linked=<name>).
 */
export function MySignInMethods() {
  const signIn = useClient(SignInService)
  const { data, error, loading, reload } = useQuery(() => signIn.listMySignInMethods({}), [signIn])
  const [unlinking, setUnlinking] = useState<LinkedAccount | null>(null)

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const linked = params.get('linked')
    if (linked) {
      toast.success(`${linked} is linked. You can sign in with it now.`)
      params.delete('linked')
      const rest = params.toString()
      window.history.replaceState(null, '', window.location.pathname + (rest ? `?${rest}` : ''))
    }
  }, [])

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data) return null
  const onlyWay = !data.hasPassword && data.linked.length === 1

  return (
    <div className="overflow-hidden rounded-xl border">
      <div className="flex items-center gap-3 px-4 py-3">
        <span className="flex size-8 items-center justify-center rounded-lg border bg-card">
          <KeyRound className="size-4 text-muted-foreground" />
        </span>
        <span className="min-w-0 flex-1 text-[13px] font-semibold">Password</span>
        {data.hasPassword ? <Pill className="border-success/30 text-success">Set</Pill> : <Pill>Not set</Pill>}
      </div>
      {data.linked.map((l) => (
        <div key={l.id} className="flex items-center gap-3 border-t px-4 py-3">
          <span className="flex size-8 items-center justify-center rounded-lg border bg-card">
            <ProviderIcon kind={l.providerKind} />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-[13px] font-semibold">{l.providerName}</span>
            <span className="block truncate text-[12px] text-muted-foreground">
              {l.email || 'Linked'} · since {formatDate(l.createdAt)}
            </span>
          </span>
          <Button
            variant="ghost"
            size="sm"
            disabled={onlyWay}
            title={onlyWay ? 'Your only way to sign in: set a password or link another account first' : undefined}
            onClick={() => setUnlinking(l)}
          >
            Unlink
          </Button>
        </div>
      ))}
      {data.available.length > 0 && (
        <div className="flex flex-wrap gap-2 border-t bg-muted/30 px-4 py-3">
          {data.available.map((p) => (
            <Button key={p.id} variant="outline" size="sm" asChild>
              <a href={p.linkUrl}>
                <ProviderIcon kind={p.kind} /> Link {p.name}
              </a>
            </Button>
          ))}
        </div>
      )}
      {unlinking && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setUnlinking(null)}
          title={`Unlink ${unlinking.providerName}?`}
          body="You won't be able to sign in with it until you link it again."
          confirm="Unlink"
          onConfirm={async () => {
            await signIn.unlinkMyAccount({ id: unlinking.id })
            toast.success(`${unlinking.providerName} unlinked`)
            setUnlinking(null)
            reload()
          }}
        />
      )}
    </div>
  )
}
