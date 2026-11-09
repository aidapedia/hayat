import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'
import { Boxes, Plus } from 'lucide-react'
import { RegistryService } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useQuery } from '@/shared/hooks/use-query'
import { useMe } from '@/features/auth'
import { EmptyState, ErrorState, Field, LoadingState, Pill } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { PageHeader } from '@/shared/ui/page/PageHeader'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/shared/ui/table'
import { CredentialsDialog } from './CredentialsDialog'

function RegisterDialog({ onClose, onDone }: { onClose: () => void; onDone: (r: { id: string; clientId: string; secret: string }) => void }) {
  const registry = useClient(RegistryService)
  const { me } = useMe()
  const [id, setId] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [owner, setOwner] = useState(me.user?.email ?? '')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await registry.registerService({ id, displayName: name, description, ownerEmail: owner })
      toast.success(`${res.service?.displayName} registered`)
      onDone({ id: res.service!.id, clientId: res.clientId, secret: res.clientSecret })
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">Register a service</DialogTitle>
            <DialogDescription className="text-[13px]">
              Gives it a namespace in the catalog, API credentials, and an owner who approves its contracts.
            </DialogDescription>
          </DialogHeader>
          <Field label="Namespace" htmlFor="svc-id" hint="Lowercase, e.g. billing. Its types become billing/…, its permissions billing.…">
            <Input id="svc-id" value={id} onChange={(e) => setId(e.target.value.toLowerCase())} className="font-mono" required autoFocus />
          </Field>
          <Field label="Display name" htmlFor="svc-name">
            <Input id="svc-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Billing" />
          </Field>
          <Field label="Description" htmlFor="svc-desc">
            <Input id="svc-desc" value={description} onChange={(e) => setDescription(e.target.value)} />
          </Field>
          <Field label="Owner's email" htmlFor="svc-owner" hint="They approve its contracts and can add more owners.">
            <Input id="svc-owner" type="email" value={owner} onChange={(e) => setOwner(e.target.value)} required />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !id || !owner}>
              {busy ? 'Registering…' : 'Register service'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

export default function ServicesPage() {
  const registry = useClient(RegistryService)
  const navigate = useNavigate()
  const { isAdmin } = useMe()
  const { data, error, loading, reload } = useQuery(() => registry.listServices({}).then((r) => r.services), [registry])
  const [registering, setRegistering] = useState(false)
  const [creds, setCreds] = useState<{ id: string; clientId: string; secret: string } | null>(null)

  return (
    <>
      <PageHeader
        title="Services"
        desc="Applications that file their resources in Hayat's tree and ask it for decisions. Each publishes a contract its owners approve."
        action={
          isAdmin && (
            <Button size="lg" onClick={() => setRegistering(true)}>
              <Plus /> Register service
            </Button>
          )
        }
      />
      <div className="mt-7">
        {error ? (
          <ErrorState message={error} onRetry={reload} />
        ) : loading && !data ? (
          <LoadingState />
        ) : !data || data.length === 0 ? (
          <EmptyState icon={Boxes} title="No services yet">
            Register one to give it API credentials and a namespace for its contract.
          </EmptyState>
        ) : (
          <div className="overflow-x-auto rounded-xl border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="text-[11px]">Service</TableHead>
                  <TableHead className="text-[11px]">Contract</TableHead>
                  <TableHead className="text-[11px]">Owners</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((s) => (
                  <TableRow key={s.id} className="cursor-pointer" onClick={() => navigate(`/services/${s.id}`)}>
                    <TableCell>
                      <div className="flex items-center gap-2 text-[13px] font-semibold">
                        {s.displayName}
                        {s.awaitingYou && <Pill className="border-warning/30 bg-warning/10 text-warning">Needs your approval</Pill>}
                      </div>
                      <div className="font-mono text-[12px] text-faint">{s.id}</div>
                    </TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{s.activeVersion ? `version ${s.activeVersion}` : 'none active yet'}</TableCell>
                    <TableCell className="text-[12px] text-muted-foreground">{s.owners.map((o) => o.name || o.email).join(', ')}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
      {registering && (
        <RegisterDialog
          onClose={() => setRegistering(false)}
          onDone={(r) => {
            setRegistering(false)
            setCreds(r)
            void reload()
          }}
        />
      )}
      {creds && (
        <CredentialsDialog
          clientId={creds.clientId}
          secret={creds.secret}
          onClose={() => {
            const id = creds.id
            setCreds(null)
            navigate(`/services/${id}`)
          }}
        />
      )}
    </>
  )
}
