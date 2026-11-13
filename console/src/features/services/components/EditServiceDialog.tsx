import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { RegistryService, type ServiceInfo } from '@/shared/api/gen/hayat/admin/v1/policy_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'
import { Textarea } from '@/shared/ui/textarea'

/** A service's display name and description. Its namespace never changes. */
export function EditServiceDialog({ service, onClose, onSaved }: { service: ServiceInfo; onClose: () => void; onSaved: () => unknown }) {
  const registry = useClient(RegistryService)
  const [name, setName] = useState(service.displayName)
  const [description, setDescription] = useState(service.description)
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      await registry.updateService({ id: service.id, displayName: name, description })
      toast.success('Service saved')
      await onSaved()
      onClose()
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
            <DialogTitle className="text-[15px]">Edit {service.displayName}</DialogTitle>
            <DialogDescription className="text-[13px]">The namespace can't change: its types and permissions are named after it.</DialogDescription>
          </DialogHeader>
          <Field label="Namespace" htmlFor="svc-ns">
            <Input id="svc-ns" value={service.id} readOnly className="bg-muted font-mono text-muted-foreground" />
          </Field>
          <Field label="Display name" htmlFor="svc-name" hint="On the sign-in page and in this console.">
            <Input id="svc-name" value={name} onChange={(e) => setName(e.target.value)} required maxLength={100} autoFocus />
          </Field>
          <Field label="Description" htmlFor="svc-desc">
            <Textarea id="svc-desc" value={description} onChange={(e) => setDescription(e.target.value)} rows={3} />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name.trim()}>
              {busy ? 'Saving…' : 'Save'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
