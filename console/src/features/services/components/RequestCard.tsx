import { useState } from 'react'
import { toast } from 'sonner'
import { Check, Clock, FileCode2 } from 'lucide-react'
import { RegistryService, type Contract } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { formatRelative } from '@/shared/lib/datetime'
import { Field, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Textarea } from '@/shared/ui/textarea'
import { SummaryJson } from '../contract/SummaryList'
import { readManifest } from '../lib'
import { ManifestYaml } from './ManifestView'

/**
 * A contract version waiting on an approval. `incoming` is another service's
 * version that files its types under this service's types; otherwise it is
 * this service's own version, waiting on another service.
 */
export function RequestCard({ c, incoming, onChanged }: { c: Contract; incoming: boolean; onChanged: () => unknown }) {
  const registry = useClient(RegistryService)
  const [viewing, setViewing] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const canDecide = c.canApprove.length > 0
  const manifest = readManifest(c.manifestJson)

  const act = async (fn: () => Promise<unknown>, done: string) => {
    setBusy(true)
    try {
      await fn()
      toast.success(done)
      await onChanged()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="overflow-hidden rounded-xl border border-warning/40">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-warning/40 bg-warning/10 px-4 py-3">
        <div className="flex items-center gap-2.5">
          <Clock className="size-4 text-warning" />
          <h2 className="text-[13px] font-semibold">
            {incoming ? `${c.serviceId} version ${c.version} wants to file its types under yours` : `Version ${c.version} waits for another service`}
          </h2>
        </div>
        <span className="text-[12px] text-muted-foreground">
          Published by {c.submittedBy} · {formatRelative(c.submittedAt)}
        </span>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-4 bg-card px-4 py-3.5">
        <div className="flex min-w-0 flex-col gap-2">
          <SummaryJson json={c.summaryJson} />
          <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-faint">
            Needs approval from
            {c.namespaces.map((ns) => (
              <Pill key={ns} className={c.approved.includes(ns) ? 'border-success/30 text-success' : ''}>
                {c.approved.includes(ns) ? '✓ ' : ''}
                {ns}
              </Pill>
            ))}
          </div>
        </div>
        <div className="flex gap-2">
          <Button variant="ghost" size="sm" onClick={() => setViewing(true)}>
            <FileCode2 /> Manifest
          </Button>
          {canDecide && (
            <>
              <Button variant="outline" size="sm" disabled={busy} onClick={() => setRejecting(true)}>
                Reject…
              </Button>
              <Button size="sm" disabled={busy} onClick={() => act(() => registry.approveContract({ serviceId: c.serviceId, version: c.version }), 'Approved')}>
                <Check /> Approve
              </Button>
            </>
          )}
        </div>
      </div>

      <Dialog open={viewing} onOpenChange={setViewing}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="text-[15px]">
              {c.serviceId} · version {c.version}
            </DialogTitle>
          </DialogHeader>
          {manifest ? <ManifestYaml manifest={manifest} /> : <pre className="font-mono text-[11px]">{c.manifestJson}</pre>}
        </DialogContent>
      </Dialog>

      <Dialog open={rejecting} onOpenChange={setRejecting}>
        <DialogContent className="sm:max-w-md">
          <form
            className="flex flex-col gap-4"
            onSubmit={async (e) => {
              e.preventDefault()
              await act(() => registry.rejectContract({ serviceId: c.serviceId, version: c.version, reason }), 'Rejected')
              setRejecting(false)
              setReason('')
            }}
          >
            <DialogHeader>
              <DialogTitle className="text-[15px]">
                Reject {c.serviceId} version {c.version}?
              </DialogTitle>
              <DialogDescription className="text-[13px]">Its owners see your reason and can publish a version without it.</DialogDescription>
            </DialogHeader>
            <Field label="Reason" htmlFor="reject-reason">
              <Textarea id="reject-reason" value={reason} onChange={(e) => setReason(e.target.value)} rows={3} required />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setRejecting(false)}>
                Cancel
              </Button>
              <Button type="submit" variant="destructive" disabled={busy || !reason.trim()}>
                Reject version
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  )
}
