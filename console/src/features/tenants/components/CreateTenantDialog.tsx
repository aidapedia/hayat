import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { TenantService, type Tenant } from '@/shared/api/gen/hayat/admin/v1/admin_pb'
import { errorMessage, useClient } from '@/shared/api/client'
import { useMe } from '@/features/auth'
import { UserPicker, type UserChoice } from '@/features/users'
import { Field } from '@/shared/ui/bits'
import { Button } from '@/shared/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/shared/ui/dialog'
import { Input } from '@/shared/ui/input'

const slugify = (name: string) =>
  name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)

/**
 * Create a tenant: name, slug, first owner. Mounted while asking; the caller
 * decides where to go next (the Tenants page opens the tenant, the Resource
 * Tree selects its branch).
 */
export function CreateTenantDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (tenant: Tenant) => void }) {
  const tenants = useClient(TenantService)
  const { me } = useMe()
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [slugTouched, setSlugTouched] = useState(false)
  const [owner, setOwner] = useState<UserChoice | null>(me.user ? { kind: 'user', user: me.user } : null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    try {
      const res = await tenants.createTenant({
        name,
        slug: slugTouched ? slug : '',
        ...(owner?.kind === 'user' ? { ownerUserId: owner.user.id } : { ownerEmail: owner?.email ?? '' }),
      })
      toast.success(`${res.tenant!.name} created`)
      onCreated(res.tenant!)
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
            <DialogTitle className="text-[15px]">Create a tenant</DialogTitle>
            <DialogDescription className="text-[13px]">An organisation every service shares. The owner becomes its first member.</DialogDescription>
          </DialogHeader>
          <Field label="Name" htmlFor="t-name">
            <Input
              id="t-name"
              value={name}
              onChange={(e) => {
                setName(e.target.value)
                if (!slugTouched) setSlug(slugify(e.target.value))
              }}
              required
              autoFocus
            />
          </Field>
          <Field label="Slug" htmlFor="t-slug" hint="Lowercase letters, digits and dashes. Used in addresses; can't be changed later.">
            <Input
              id="t-slug"
              value={slug}
              onChange={(e) => {
                setSlug(e.target.value)
                setSlugTouched(true)
              }}
              className="font-mono"
            />
          </Field>
          <Field label="Owner" htmlFor="t-owner" hint="Search by name or email. An address without an account gets an invitation.">
            <UserPicker id="t-owner" value={owner} onChange={setOwner} />
          </Field>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name || !owner}>
              {busy ? 'Creating…' : 'Create tenant'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
