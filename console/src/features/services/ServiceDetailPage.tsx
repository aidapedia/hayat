import { useState } from 'react'
import { useParams } from 'react-router'
import { toast } from 'sonner'
import { Check, FileCode2, KeyRound, UserMinus, UserPlus, X } from 'lucide-react'
import { RegistryService, type Contract } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { formatDateTime } from '@/shared/lib/datetime'
import { CopyField, DetailList, EmptyState, ErrorState, Field, LoadingState, PersonRow, Pill, StatusBadge } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { ConfirmDialog } from '@/shared/ui/confirm-dialog'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { PageHeader, Section } from '@/shared/ui/page/PageHeader'
import { Textarea } from '@/shared/ui/textarea'
import { ContractEditor } from './contract/ContractEditor'
import { manifestYaml, type Manifest } from './contract/manifest'
import { SummaryJson } from './contract/SummaryList'
import { CredentialsDialog } from './CredentialsDialog'

/** A stored manifest as YAML, the way it was written; the raw JSON if it can't be. */
function manifestText(json: string): string {
  try {
    return manifestYaml(JSON.parse(json) as Manifest)
  } catch {
    return json
  }
}

function ContractCard({ c, onApprove, onReject, onView }: { c: Contract; onApprove: () => void; onReject: () => void; onView: () => void }) {
  return (
    <div className="flex flex-col gap-3 px-4 py-3.5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <span className="text-[13px] font-semibold">
            {c.serviceId} · version {c.version}
          </span>
          <StatusBadge status={c.status} />
        </div>
        <div className="flex gap-2">
          <Button variant="ghost" size="sm" onClick={onView}>
            <FileCode2 /> Manifest
          </Button>
          {c.canApprove.length > 0 && (
            <>
              <Button variant="outline" size="sm" onClick={onReject}>
                <X /> Reject
              </Button>
              <Button size="sm" onClick={onApprove}>
                <Check /> Approve
              </Button>
            </>
          )}
        </div>
      </div>
      <SummaryJson json={c.summaryJson} />
      <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-faint">
        Needs approval from
        {c.namespaces.map((ns) => (
          <Pill key={ns} className={c.approved.includes(ns) ? 'border-success/30 text-success' : ''}>
            {c.approved.includes(ns) ? '✓ ' : ''}
            {ns}
          </Pill>
        ))}
        <span>· submitted by {c.submittedBy} {formatDateTime(c.submittedAt)}</span>
      </div>
      {c.reason && <p className="text-[12px] text-destructive">Rejected: {c.reason}</p>}
    </div>
  )
}

export default function ServiceDetailPage() {
  const { id = '' } = useParams()
  const registry = useClient(RegistryService)
  const { data, error, loading, reload } = useQuery(() => registry.getService({ id }), [registry, id])
  const [viewing, setViewing] = useState<Contract | null>(null)
  const [rejecting, setRejecting] = useState<Contract | null>(null)
  const [reason, setReason] = useState('')
  const [ownerEmail, setOwnerEmail] = useState('')
  const [removing, setRemoving] = useState<{ id: string; label: string } | null>(null)
  const [rotating, setRotating] = useState(false)
  const [secret, setSecret] = useState('')

  if (error) return <ErrorState message={error} onRetry={reload} />
  if (loading && !data) return <LoadingState />
  if (!data?.service) return null
  const s = data.service

  const act = async (fn: () => Promise<unknown>, done: string) => {
    try {
      await fn()
      toast.success(done)
      await reload()
    } catch (err) {
      toast.error(errorMessage(err))
    }
  }

  return (
    <>
      <PageHeader
        title={s.displayName}
        crumbs={[{ label: s.displayName }]}
        meta={
          <>
            <Pill className="font-mono normal-case">{s.id}</Pill>
            {s.activeVersion > 0 && <Pill className="border-success/30 text-success">v{s.activeVersion} active</Pill>}
          </>
        }
        desc={s.description || undefined}
      />

      <div className="mt-7 grid grid-cols-[minmax(0,1fr)_minmax(280px,360px)] gap-x-10 max-[1000px]:grid-cols-1">
        <div className="min-w-0">
          {data.incoming.length > 0 && (
            <Section title="Waiting on this namespace" desc="Other services want to file their types under this service's types.">
              <div className="divide-y rounded-xl border border-warning/40">
                {data.incoming.map((c) => (
                  <ContractCard
                    key={`${c.serviceId}-${c.version}`}
                    c={c}
                    onView={() => setViewing(c)}
                    onApprove={() => act(() => registry.approveContract({ serviceId: c.serviceId, version: c.version }), 'Approved')}
                    onReject={() => setRejecting(c)}
                  />
                ))}
              </div>
            </Section>
          )}
          <Section title="Contract versions" desc="What this service declares: its resource types, permissions and suggested roles.">
            {data.contracts.length === 0 ? (
              <EmptyState icon={FileCode2} title="No contract yet">
                Fill in the form below, or publish one from CI through the API's SchemaService.
              </EmptyState>
            ) : (
              <div className="divide-y rounded-xl border">
                {data.contracts.map((c) => (
                  <ContractCard
                    key={c.version}
                    c={c}
                    onView={() => setViewing(c)}
                    onApprove={() => act(() => registry.approveContract({ serviceId: c.serviceId, version: c.version }), 'Approved')}
                    onReject={() => setRejecting(c)}
                  />
                ))}
              </div>
            )}
          </Section>
          {data.canManage && <ContractEditor key={`${data.contracts[0]?.version ?? 0}-${data.contracts[0]?.status ?? ""}`} serviceId={s.id} contracts={data.contracts} onPublished={reload} />}
        </div>

        <div className="min-w-0 max-[1000px]:mt-9">
          <Section title="Owners" desc="They approve contracts and manage this service.">
            <div className="divide-y rounded-xl border">
              {s.owners.map((o) => (
                <div key={o.id} className="flex items-center justify-between gap-2 px-3 py-2.5">
                  <PersonRow name={o.name} email={o.email} />
                  {data.canManage && s.owners.length > 1 && (
                    <Button variant="ghost" size="icon-sm" aria-label={`Remove ${o.email}`} onClick={() => setRemoving({ id: o.id, label: o.name || o.email })}>
                      <UserMinus />
                    </Button>
                  )}
                </div>
              ))}
            </div>
            {data.canManage && (
              <form
                className="mt-3 flex gap-2"
                onSubmit={async (e) => {
                  e.preventDefault()
                  await act(() => registry.addServiceOwner({ serviceId: id, email: ownerEmail }), 'Owner added')
                  setOwnerEmail('')
                }}
              >
                <Input type="email" value={ownerEmail} onChange={(e) => setOwnerEmail(e.target.value)} placeholder="email@example.com" className="h-8 text-[13px]" />
                <Button type="submit" variant="outline" disabled={!ownerEmail}>
                  <UserPlus /> Add
                </Button>
              </form>
            )}
          </Section>
          <Section title="API credentials">
            <div className="rounded-xl border p-4">
              <DetailList stacked items={[['Client id', <CopyField key="c" value={s.clientId} />], ['Registered', formatDateTime(s.createdAt)]]} />
              {data.canManage && (
                <Button variant="outline" className="mt-4 w-full" onClick={() => setRotating(true)}>
                  <KeyRound /> Rotate secret
                </Button>
              )}
            </div>
          </Section>
        </div>
      </div>

      <Dialog open={!!viewing} onOpenChange={(o) => !o && setViewing(null)}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="text-[15px]">
              {viewing?.serviceId} · version {viewing?.version}
            </DialogTitle>
          </DialogHeader>
          <pre className="overflow-x-auto rounded-[10px] border bg-muted/50 p-3 font-mono text-[11px]">{viewing && manifestText(viewing.manifestJson)}</pre>
        </DialogContent>
      </Dialog>

      <Dialog open={!!rejecting} onOpenChange={(o) => !o && setRejecting(null)}>
        <DialogContent className="sm:max-w-md">
          <form
            className="flex flex-col gap-4"
            onSubmit={async (e) => {
              e.preventDefault()
              if (!rejecting) return
              await act(() => registry.rejectContract({ serviceId: rejecting.serviceId, version: rejecting.version, reason }), 'Rejected')
              setRejecting(null)
              setReason('')
            }}
          >
            <DialogHeader>
              <DialogTitle className="text-[15px]">Reject version {rejecting?.version}?</DialogTitle>
              <DialogDescription className="text-[13px]">The service's owners see your reason and can publish a corrected version.</DialogDescription>
            </DialogHeader>
            <Field label="Reason" htmlFor="reject-reason">
              <Textarea id="reject-reason" value={reason} onChange={(e) => setReason(e.target.value)} rows={3} required />
            </Field>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setRejecting(null)}>
                Cancel
              </Button>
              <Button type="submit" variant="destructive" disabled={!reason.trim()}>
                Reject version
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={`Remove ${removing?.label} as an owner?`}
        body="They stop approving this service's contracts."
        confirm="Remove owner"
        destructive
        onConfirm={async () => {
          if (!removing) return
          await registry.removeServiceOwner({ serviceId: id, userId: removing.id })
          toast.success('Owner removed')
          await reload()
        }}
      />
      <ConfirmDialog
        open={rotating}
        onOpenChange={setRotating}
        title="Rotate the API secret?"
        body="The current secret stops working at once. Update the service's configuration right away."
        confirm="Rotate secret"
        destructive
        onConfirm={async () => {
          const res = await registry.rotateServiceSecret({ serviceId: id })
          setSecret(res.clientSecret)
        }}
      />
      {secret && <CredentialsDialog clientId={s.clientId} secret={secret} onClose={() => setSecret('')} />}
    </>
  )
}
