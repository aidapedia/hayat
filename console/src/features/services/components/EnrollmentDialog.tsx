import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { KeyRound, TriangleAlert } from 'lucide-react'
import { RegistryService } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { formatDateTime } from '@/shared/lib/datetime'
import { CopyField, Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Combobox } from '@/shared/ui/combobox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'

const LIFETIMES = [
  { value: String(60 * 60), label: '1 hour' },
  { value: String(24 * 60 * 60), label: '1 day' },
  { value: String(7 * 24 * 60 * 60), label: '7 days' },
]

type Made = { serviceId: string; code: string; expiresAt: string; registers: boolean }

/**
 * Makes an enrollment code: the one-time code a service's deployment pastes
 * into its setup page (or redeems on its own) to get its credentials. With a
 * `service`, it re-enrolls that service, which rotates its secret; without,
 * an instance admin names a new service, which redeeming registers.
 */
export function EnrollmentDialog({ service, onClose, onMade }: { service?: { id: string; displayName: string }; onClose: () => void; onMade?: () => unknown }) {
  const registry = useClient(RegistryService)
  const [id, setId] = useState(service?.id ?? '')
  const [name, setName] = useState('')
  const [ttl, setTtl] = useState(LIFETIMES[1].value)
  const [busy, setBusy] = useState(false)
  const [made, setMade] = useState<Made | null>(null)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await registry.createEnrollment({ serviceId: id, displayName: name, ttlSeconds: Number(ttl) })
      setMade({ serviceId: id, code: res.code, expiresAt: formatDateTime(res.enrollment?.expiresAt), registers: !service })
      await onMade?.()
    } catch (err) {
      toast.error(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  if (made) {
    return (
      <Dialog open onOpenChange={(o) => !o && onClose()}>
        <DialogContent className="sm:max-w-lg" onInteractOutside={(e) => e.preventDefault()}>
          <DialogHeader>
            <DialogTitle className="text-[15px]">Enrollment code for {made.serviceId}</DialogTitle>
            <DialogDescription className="text-[13px]">
              Paste it into the service's setup page. It works once, until {made.expiresAt}, and Hayat can't show it again.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <Field label="Code">
              <CopyField value={made.code} />
            </Field>
            <ol className="flex list-decimal flex-col gap-1 pl-5 text-[12px] text-muted-foreground">
              <li>Open the service. A new one shows its setup page; an existing one has it under its admin settings.</li>
              <li>Paste the code. The service registers {made.registers ? 'itself' : 'again'} and stores its credentials.</li>
              <li>{made.registers ? 'You become its owner and, unless the setup page names someone else, its admin.' : 'Its sign-in keeps working; its old secret stops.'}</li>
            </ol>
            {!made.registers && (
              <p className="flex items-start gap-2 rounded-[10px] border border-warning/30 bg-warning/10 px-3 py-2 text-[12px] text-warning">
                <TriangleAlert className="mt-0.5 size-3.5 shrink-0" /> Redeeming it rotates the service's API secret. Anything else still using the old
                one stops working at once.
              </p>
            )}
          </div>
          <DialogFooter>
            <Button onClick={onClose}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    )
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle className="text-[15px]">{service ? `Re-enroll ${service.displayName}` : 'Enroll a service'}</DialogTitle>
            <DialogDescription className="text-[13px]">
              {service
                ? 'A one-time code the service redeems for a new API secret. Making it revokes any other open code for this service.'
                : 'A one-time code the service redeems on its setup page: it registers itself, publishes its contract and stores its credentials. Nobody copies a secret.'}
            </DialogDescription>
          </DialogHeader>
          {!service && (
            <>
              <Field label="Namespace" htmlFor="enr-id" hint="What the service calls itself, e.g. tabletsgo. Its setup page tells you.">
                <Input id="enr-id" value={id} onChange={(e) => setId(e.target.value.toLowerCase())} className="font-mono" required autoFocus />
              </Field>
              <Field label="Display name" htmlFor="enr-name" hint="Optional; the service can send its own.">
                <Input id="enr-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Tabletsgo" />
              </Field>
            </>
          )}
          <Field label="Valid for" htmlFor="enr-ttl">
            <Combobox id="enr-ttl" value={ttl} onChange={(v) => v && setTtl(v)} options={LIFETIMES} />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !id}>
              <KeyRound /> {busy ? 'Making…' : 'Make code'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
