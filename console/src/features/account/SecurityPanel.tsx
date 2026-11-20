import { useState } from 'react'
import { Fingerprint, LifeBuoy, ShieldCheck, Smartphone } from 'lucide-react'
import { toast } from 'sonner'
import { AccountService, type Factor } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, isStepUp, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDate, formatRelative } from '@/shared/lib/datetime'
import { ErrorState, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'

function FactorIcon({ kind }: { kind: string }) {
  const Icon = kind === 'passkey' ? Fingerprint : Smartphone
  return (
    <span className="flex size-8 flex-none items-center justify-center rounded-lg border bg-card">
      <Icon className="size-4 text-muted-foreground" />
    </span>
  )
}

/** One row per authenticator app or passkey. `onRemove` adds a Remove button. */
export function FactorList({ factors, onRemove }: { factors: Factor[]; onRemove?: (f: Factor) => void }) {
  return (
    <>
      {factors.map((f, i) => (
        <div key={f.id} className={`flex items-center gap-3 px-4 py-3 ${i > 0 ? 'border-t' : ''}`}>
          <FactorIcon kind={f.kind} />
          <span className="min-w-0 flex-1">
            <span className="block text-[13px] font-semibold">{f.name || (f.kind === 'passkey' ? 'Passkey' : 'Authenticator app')}</span>
            <span className="block truncate text-[12px] text-muted-foreground">
              {f.kind === 'passkey' ? 'Passkey' : 'Authenticator app'} · added {formatDate(f.createdAt)}
              {f.lastUsedAt ? ` · last used ${formatRelative(f.lastUsedAt)}` : ''}
            </span>
          </span>
          {onRemove && (
            <Button variant="ghost" size="sm" onClick={() => onRemove(f)}>
              Remove
            </Button>
          )}
        </div>
      ))}
    </>
  )
}

/**
 * Two-step verification on the caller's account. Adding an authenticator app
 * or a passkey happens on Hayat's own pages (a passkey belongs to Hayat's
 * address, not the console's), which bring the person back here.
 */
export function SecurityPanel() {
  const account = useClient(AccountService)
  const { data, error, loading, reload } = useQuery(() => account.getMySecurity({}), [account])
  const [removing, setRemoving] = useState<Factor | null>(null)
  const [codes, setCodes] = useState<string[] | null>(null)
  const [busy, setBusy] = useState(false)

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data) return null

  const here = encodeURIComponent(window.location.href)
  const setup = `${data.setupUrl}?return_to=${here}`
  const on = data.factors.length > 0
  const lastRequired = data.mfaRequired && data.factors.length === 1

  const regenerate = async () => {
    setBusy(true)
    try {
      const res = await account.regenerateRecoveryCodes({})
      setCodes(res.codes)
      await reload()
    } catch (err) {
      if (!isStepUp(err)) toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <div className="overflow-hidden rounded-xl border">
        <div className="flex items-center gap-3 px-4 py-3">
          <span className="flex size-8 flex-none items-center justify-center rounded-lg border bg-card">
            <ShieldCheck className={`size-4 ${on ? 'text-success' : 'text-muted-foreground'}`} />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-[13px] font-semibold">{on ? 'On' : 'Off'}</span>
            <span className="block text-[12px] text-muted-foreground">
              {on
                ? 'Signing in with a password also asks for a code or a passkey.'
                : data.mfaRequired
                  ? `${data.mfaRequiredBy} requires it: you'll be asked to set it up at your next sign-in.`
                  : 'Add an authenticator app or a passkey so a stolen password isn’t enough.'}
            </span>
          </span>
          {data.mfaRequired && <Pill>Required by {data.mfaRequiredBy}</Pill>}
        </div>
        {on && (
          <div className="border-t">
            <FactorList factors={data.factors} onRemove={setRemoving} />
          </div>
        )}
        {on && (
          <div className="flex items-center gap-3 border-t px-4 py-3">
            <span className="flex size-8 flex-none items-center justify-center rounded-lg border bg-card">
              <LifeBuoy className="size-4 text-muted-foreground" />
            </span>
            <span className="min-w-0 flex-1">
              <span className="block text-[13px] font-semibold">Recovery codes</span>
              <span className="block text-[12px] text-muted-foreground">
                {data.recoveryCodesLeft} of 10 left. Each one signs you in once if you lose your device.
              </span>
            </span>
            <Button variant="ghost" size="sm" disabled={busy} onClick={() => void regenerate()}>
              {busy ? 'Making…' : 'New codes'}
            </Button>
          </div>
        )}
        <div className="flex flex-wrap gap-2 border-t bg-muted/30 px-4 py-3">
          <Button variant="outline" size="sm" asChild>
            <a href={setup}>
              <Smartphone /> Add authenticator app
            </a>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <a href={setup}>
              <Fingerprint /> Add passkey
            </a>
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={`Remove ${removing?.name || 'this method'}?`}
        body={
          lastRequired
            ? `${data.mfaRequiredBy} requires two-step verification: add another method before removing this one.`
            : data.factors.length === 1
              ? 'It is your only second step: two-step verification turns off and your recovery codes stop working.'
              : 'You won’t be able to sign in or confirm it’s you with it any more.'
        }
        confirm="Remove"
        destructive
        onConfirm={async () => {
          if (!removing) return
          await account.removeMyFactor({ factorId: removing.id })
          toast.success('Removed')
          await reload()
        }}
      />

      <Dialog open={!!codes} onOpenChange={(o) => !o && setCodes(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="text-[15px]">Your new recovery codes</DialogTitle>
            <DialogDescription className="text-[13px]">
              Each one signs you in once. Keep them somewhere safe: they won&apos;t be shown again, and the old ones no longer work.
            </DialogDescription>
          </DialogHeader>
          <ol className="grid list-decimal grid-cols-2 gap-x-6 gap-y-1.5 rounded-xl border px-8 py-3 font-mono text-[13px]">
            {codes?.map((c) => (
              <li key={c}>{c}</li>
            ))}
          </ol>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                void navigator.clipboard.writeText((codes ?? []).join('\n')).then(() => toast.success('Copied'))
              }}
            >
              Copy
            </Button>
            <Button onClick={() => setCodes(null)}>I&apos;ve saved them</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
